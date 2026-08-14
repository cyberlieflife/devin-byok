package localapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"devin-byok/internal/config"
	"devin-byok/internal/logx"
	"devin-byok/internal/pbwire"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/openai"
)

// Server 本地 Codeium API 兼容层。
type Server struct {
	mu              sync.RWMutex
	cfg             *config.File
	cfgPath         string
	upstream        *openai.Client
	rpcMu           sync.Mutex
	rpcLog          string
	bodyDir         string
	stopWatch       chan struct{}
	rpcRotator      *logx.RotatingWriter
	restartRequired atomic.Bool
}

const officialAPIBase = "https://server.codeium.com"

func boolPtr(v bool) *bool { return &v }

func New(cfg *config.File, captureDir string) *Server {
	if captureDir == "" {
		captureDir = "work/capture"
	}
	_ = os.MkdirAll(captureDir, 0o755)
	bodyDir := filepath.Join(captureDir, "bodies")
	_ = os.MkdirAll(bodyDir, 0o755)
	return &Server{
		cfg:        cfg,
		upstream:   openai.New(cfg.Upstream),
		rpcLog:     filepath.Join(captureDir, "localapi-rpc.jsonl"),
		bodyDir:    bodyDir,
		stopWatch:  make(chan struct{}),
		rpcRotator: logx.NewRotatingWriter(filepath.Join(captureDir, "localapi-rpc.jsonl"), 32<<20, 3),
	}
}

// Close 释放 Server 占用的底侧资源与句柄。
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopWatch:
	default:
		close(s.stopWatch)
	}
	if s.rpcRotator != nil {
		return s.rpcRotator.Close()
	}
	return nil
}

// SetConfigPath 设置配置文件路径以启用热重载。
func (s *Server) SetConfigPath(path string) {
	s.mu.Lock()
	s.cfgPath = path
	s.mu.Unlock()
}

// GetConfig 返回当前配置快照。
func (s *Server) GetConfig() *config.File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// ReloadConfig 从磁盘重载配置。
func (s *Server) ReloadConfig() error {
	s.mu.RLock()
	path := s.cfgPath
	s.mu.RUnlock()
	if path == "" {
		return fmt.Errorf("未设置 config path")
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.upstream.Update(cfg.Upstream)
	s.mu.Unlock()
	logx.Infof("config reloaded: pure_local=%v stream=%v model=%s models=%d",
		cfg.Features.PureLocal, cfg.Features.EnableStream, cfg.DefaultModelID(), len(cfg.ModelList()))
	return nil
}

// StartConfigWatch 轮询配置文件变更（热重载）。
func (s *Server) StartConfigWatch(interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		var lastMod time.Time
		var lastSize int64
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-s.stopWatch:
				return
			case <-t.C:
				s.mu.RLock()
				path := s.cfgPath
				s.mu.RUnlock()
				if path == "" {
					continue
				}
				st, err := os.Stat(path)
				if err != nil {
					continue
				}
				if st.ModTime().Equal(lastMod) && st.Size() == lastSize {
					continue
				}
				// 首次只记录
				if lastMod.IsZero() {
					lastMod, lastSize = st.ModTime(), st.Size()
					continue
				}
				lastMod, lastSize = st.ModTime(), st.Size()
				if err := s.ReloadConfig(); err != nil {
					logx.Warnf("config reload failed: %v", err)
				}
			}
		}
	}()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/chat/completions", s.handleOpenAIProxy)
	mux.HandleFunc("/_route/api_server/", s.handleConnectRPC)
	mux.HandleFunc("/_route/api_server", s.handleConnectRPC)
	s.registerUIRoutes(mux)
	mux.HandleFunc("/", s.handleCatchAll)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	cfg := s.GetConfig()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":            true,
		"service":       "devin-byok-local-api",
		"time":          time.Now().Format(time.RFC3339),
		"portal":        cfg.Server.PublicBase,
		"api":           cfg.Server.PublicBase + cfg.APIBasePath(),
		"model":         cfg.DefaultModelID(),
		"models":        cfg.ModelList(),
		"pure_local":    cfg.Features.PureLocal,
		"stream":        cfg.Features.EnableStream,
		"cascade_tools": cfg.Features.EnableCascadeTools,
		"deepwiki":      cfg.Features.EnableDeepWiki,
		"codemap":       cfg.Features.EnableCodeMap,
		"tools_mode":    cfg.ToolsMode(),
		"tools_timeout": cfg.ResolveChatTimeoutSec(true),
	})
}

func (s *Server) handleOpenAIProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var envelope struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &envelope)
	cfg := s.GetConfig()
	modelID := strings.TrimSpace(envelope.Model)
	if modelID == "" {
		modelID = cfg.DefaultModelID()
	}
	model, ok := cfg.ResolveModelUID(modelID)
	if !ok {
		model = config.ModelEntry{ID: modelID, UpstreamModel: modelID}
	}
	prov, ok := cfg.ResolveProvider(model.ID)
	if !ok {
		// Keep legacy single-provider configs working when the request uses the
		// configured upstream model name rather than a Devin model UID.
		prov = config.ProviderResolved{
			BaseURL: cfg.Upstream.BaseURL, APIKey: cfg.Upstream.APIKey,
			UpstreamModel: model.ResolveUpstream(), Headers: cfg.Upstream.Headers,
		}
	}
	if prov.UpstreamModel != "" && modelID != prov.UpstreamModel {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err == nil {
			encodedModel, _ := json.Marshal(prov.UpstreamModel)
			payload["model"] = encodedModel
			if remapped, err := json.Marshal(payload); err == nil {
				body = remapped
			}
		}
	}
	endpoint := config.NormalizeChatCompletionsURL(prov.BaseURL)
	if endpoint == "" {
		http.Error(w, "model provider base_url is not configured", http.StatusBadGateway)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+prov.APIKey)
	for k, v := range prov.Headers {
		req.Header.Set(k, v)
	}
	logx.Infof("openai proxy model=%s upstream_model=%s base=%s", model.ID, prov.UpstreamModel, truncate(prov.BaseURL, 80))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		s.handleHealthz(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/ui") || strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/_route/api_server") {
		s.handleConnectRPC(w, r)
		return
	}
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/ui/", http.StatusFound)
		return
	}
	s.serveRPC(w, r)
}

func (s *Server) handleConnectRPC(w http.ResponseWriter, r *http.Request) {
	s.serveRPC(w, r)
}

func (s *Server) serveRPC(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	path := r.URL.Path
	method := methodName(path)
	ctype := r.Header.Get("Content-Type")
	body := maybeGunzip(raw)

	// 落盘完整请求体，便于继续逆向
	s.dumpBody(method, raw)

	rec := map[string]any{
		"ts":        time.Now().Format(time.RFC3339Nano),
		"method":    r.Method,
		"path":      path,
		"rpc":       method,
		"ctype":     ctype,
		"connect":   r.Header.Get("Connect-Protocol-Version"),
		"encoding":  r.Header.Get("Content-Encoding"),
		"body_len":  len(raw),
		"plain_len": len(body),
		"body_b64":  truncateB64(raw, 2048),
		"strings":   extractStrings(body, 12),
	}
	s.appendRPC(rec)
	logx.Infof("RPC %s plain=%d ctype=%s", method, len(body), ctype)
	cfg := s.GetConfig()

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST,GET,OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 混合模式：非聊天关键 RPC 优先透传官方（用户仍有有效 session），保证能发出消息
	// 聊天 RPC 才走本地 OpenAI 兼容上游
	if s.shouldProxyOfficial(method) {
		if err := s.proxyOfficial(w, r, raw); err != nil {
			logx.Warnf("proxy official %s failed: %v ; fallback local stub", method, err)
			// fallthrough to local stubs
		} else {
			return
		}
	}

	// ---- P0 handlers ----
	switch {
	case strings.HasSuffix(method, "Ping"):
		s.writeProtoRPC(w, method, buildPingResponse())
		return
	case strings.HasSuffix(method, "RegisterUser"):
		s.writeProtoRPC(w, method, buildRegisterUserResponse(cfg))
		return
	case strings.HasSuffix(method, "GetUserStatus"):
		s.writeProtoRPC(w, method, buildGetUserStatusResponse(cfg))
		return
	case strings.HasSuffix(method, "GetUserJwt"):
		s.writeProtoRPC(w, method, buildGetUserJwtResponse(cfg))
		return
	case strings.HasSuffix(method, "CheckChatCapacity"):
		s.writeProtoRPC(w, method, buildCheckChatCapacityResponse())
		return
	case strings.HasSuffix(method, "CheckUserMessageRateLimit"):
		s.writeProtoRPC(w, method, buildCheckUserMessageRateLimitResponse())
		return
	case strings.HasSuffix(method, "GetModelStatuses"):
		s.writeProtoRPC(w, method, buildGetModelStatusesResponse(cfg))
		return
	case strings.HasSuffix(method, "GetCommandModelConfigs"),
		strings.HasSuffix(method, "GetCommandModelConfigsForSite"),
		strings.HasSuffix(method, "GetCliModelConfigs"),
		strings.HasSuffix(method, "GetCliModelConfigsForSite"):
		s.writeProtoRPC(w, method, buildGetCommandModelConfigsResponse(cfg))
		return
	case strings.HasSuffix(method, "GetCascadeModelConfigs"),
		strings.HasSuffix(method, "GetCascadeModelConfigsForSite"):
		s.writeProtoRPC(w, method, buildGetCascadeModelConfigsResponse(cfg))
		return
	case strings.HasSuffix(method, "GetDefaultWorkflowTemplates"):
		s.writeProtoRPC(w, method, buildGetDefaultWorkflowTemplatesResponse())
		return
	case strings.HasSuffix(method, "GetAllAcpRegistries"):
		s.writeProtoRPC(w, method, buildGetAllAcpRegistriesResponse())
		return
	case strings.HasSuffix(method, "GetProfileData"):
		s.writeProtoRPC(w, method, buildGetProfileDataResponse())
		return
	case strings.HasSuffix(method, "GetStatus"):
		s.writeProtoRPC(w, method, buildGetStatusResponse())
		return
	case strings.HasSuffix(method, "GetUnleashData"):
		s.writeProtoRPC(w, method, buildGetUnleashDataResponse())
		return
	case strings.HasSuffix(method, "ShouldEnableUnleash"):
		s.writeProtoRPC(w, method, buildShouldEnableUnleashResponse())
		return
	case strings.HasSuffix(method, "RecordCommitMessageGeneration"):
		markCommitGenerationPending()
		s.writeProto(w, []byte{})
		return
	case strings.Contains(method, "Record"),
		strings.Contains(method, "Log"):
		// 轨迹步骤含 FIND_CODE_CONTEXT 时，开启 Fast Context 模型绑定窗口
		blob := string(body)
		if strings.Contains(blob, "FIND_CODE_CONTEXT") || strings.Contains(strings.ToLower(blob), "find_code_context") {
			markFastContextPending()
			metricsAddLog("info", "fast-context pending from "+method)
		}
		s.writeProto(w, []byte{})
		return
	}

	// ---- DeepWiki / CodeMap ----
	if isDeepWikiRPC(method) {
		s.handleGetDeepWiki(w, r, method, body, raw)
		return
	}
	if isCodeMapRPC(method) {
		s.handleCodeMapRPC(w, r, method, body, raw)
		return
	}

	// ---- P1 chat-ish streaming / unary ----
	if isDevstralRPC(method) {
		s.handleGetDevstralStream(w, r, method, body, raw)
		return
	}
	if isChatRPC(method) {
		// 仅单机：Cascade 主对话走本地 BYOK
		s.handleChatLike(w, r, method, body, raw)
		return
	}

	// 默认空成功
	s.writeProto(w, []byte{})
}

func isDeepWikiRPC(method string) bool {
	m := strings.ToLower(method)
	return strings.Contains(m, "getdeepwiki")
}

// commitGenPendingUntil 在 RecordCommitMessageGeneration 后短窗口内把聊天计为 Commit 生成。
var commitGenPendingUntil atomic.Int64

func markCommitGenerationPending() {
	commitGenPendingUntil.Store(time.Now().Add(2 * time.Minute).UnixNano())
}

func isCommitGenerationPending() bool {
	until := commitGenPendingUntil.Load()
	if until == 0 {
		return false
	}
	return time.Now().UnixNano() <= until
}

func clearCommitGenerationPending() {
	commitGenPendingUntil.Store(0)
}

// fastContextPendingUntil：find_code_context 工具调用后，短窗口内的聊天视为 Fast Context 子代理。
var fastContextPendingUntil atomic.Int64

func markFastContextPending() {
	fastContextPendingUntil.Store(time.Now().Add(3 * time.Minute).UnixNano())
}

func isFastContextPending() bool {
	until := fastContextPendingUntil.Load()
	if until == 0 {
		return false
	}
	return time.Now().UnixNano() <= until
}

func clearFastContextPending() {
	fastContextPendingUntil.Store(0)
}

// isTitleGenerationChat 识别会话标题生成请求。
func isTitleGenerationChat(parsed parsedChatRequest, userText string, plain []byte) bool {
	blob := strings.ToLower(parsed.SystemPrompt + "\n" + userText)
	keys := []string{
		"conversation title", "title generator", "generate a title",
		"concise title", "summarize the conversation", "generate title",
		"output only the title", "title of the conversation",
	}
	for _, k := range keys {
		if strings.Contains(blob, k) {
			return true
		}
	}
	low := strings.ToLower(string(plain))
	for _, k := range []string{"conversation title", "title generator", "generate a title", "concise title"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// isFastContextChat 识别 Fast Context / Instant Context / find_code_context 相关请求。
func isFastContextChat(parsed parsedChatRequest, userText string, plain []byte) bool {
	if isFastContextPending() {
		return true
	}
	for _, t := range parsed.Tools {
		n := strings.ToLower(strings.TrimSpace(t.Function.Name))
		if n == "" {
			continue
		}
		if strings.Contains(n, "find_code_context") || n == "fast_context" || n == "instant_context" {
			return true
		}
	}
	blob := strings.ToLower(parsed.SystemPrompt + "\n" + userText)
	keys := []string{
		"fast context", "find code context", "find_code_context",
		"instant context", "instantcontext", "code context agent",
		"cortex_step_type_find_code_context",
	}
	for _, k := range keys {
		if strings.Contains(blob, k) {
			return true
		}
	}
	// proto 原始字节中的枚举/工具名
	low := strings.ToLower(string(plain))
	for _, k := range []string{"find_code_context", "findcodecontext", "fast_context", "instantcontext", "cortex_step_type_find_code_context"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

func toolCallsIncludeFastContext(calls []openai.ToolCall) bool {
	for _, c := range calls {
		n := strings.ToLower(c.Function.Name)
		if strings.Contains(n, "find_code_context") || n == "fast_context" || n == "instant_context" {
			return true
		}
	}
	return false
}

func isChatRPC(method string) bool {
	m := strings.ToLower(method)
	keys := []string{
		"getchatmessage", "rawgetchatmessage", "getchatcompletions",
		"getstreaming", "externalchat", "modelapitext",
	}
	for _, k := range keys {
		if strings.Contains(m, k) {
			return true
		}
	}
	return false
}

func (s *Server) handleChatLike(w http.ResponseWriter, r *http.Request, method string, body, raw []byte) {
	plain := body
	if len(plain) == 0 || !bytes.Contains(plain, []byte("<user_request>")) {
		if p := decodeConnectPayloads(raw); len(p) > 0 {
			plain = p
		}
	}
	// B5：完整解析 system / 历史消息 / tools
	parsed := parseGetChatMessageRequest(plain)
	if n := len(parsed.Images); n > 0 {
		metricsAddLog("info", fmt.Sprintf("multimodal images=%d", n))
		logx.Infof("multimodal images=%d", n)
	}
	userText := parsed.UserText
	if userText == "" {
		userText = pickUserText(plain)
	}
	if userText == "" {
		userText = "hello"
	}
	msgID := pickMessageID(plain)
	if msgID == "" {
		msgID = fmt.Sprintf("byok-%d", time.Now().UnixNano())
	}
	msgs := buildOpenAIMessages(parsed)
	// …user ? tool …
	if len(msgs) == 0 {
		msgs = append(msgs, openai.ChatMessage{Role: "user", Content: userText})
	} else {
		switch msgs[len(msgs)-1].Role {
		case "user", "tool":
			// ok
		default:
			msgs = append(msgs, openai.ChatMessage{Role: "user", Content: userText})
		}
	}

	cfg := s.GetConfig()
	allowed := make([]string, 0)
	for _, m := range cfg.ModelList() {
		allowed = append(allowed, m.ID)
	}
	// 记录请求是否在 payload (field 21) 中显式指定了有效的 model_uid
	hasExplicitModel := false
	uiModel := strings.TrimSpace(parsed.ModelUID)
	if uiModel != "" {
		if m, ok := cfg.ResolveModelUID(uiModel); ok {
			uiModel = m.ID
			hasExplicitModel = true
		} else if _, ok := cfg.FindModel(uiModel); ok {
			hasExplicitModel = true
		}
	}

	if !hasExplicitModel {
		uiModel = pickModel(plain, cfg.DefaultModelID(), allowed)
		if m2, ok2 := cfg.ResolveModelUID(uiModel); ok2 {
			uiModel = m2.ID
		}
	}

	// Title 模型绑定与语言提示词处理
	isTitleReq := isTitleGenerationChat(parsed, userText, plain)
	if isTitleReq && !hasExplicitModel {
		if tm := cfg.FeatureModelID("title"); tm != "" {
			if m, ok := cfg.ResolveModelUID(tm); ok {
				uiModel = m.ID
			} else if m, ok := cfg.FindModel(tm); ok {
				uiModel = m.ID
			} else {
				uiModel = tm
			}
		}
		metricsAddLog("info", "title-generation model="+uiModel)
		logx.Infof("title-generation model=%s", uiModel)
	}

	// Commit 模型绑定：仅在生成 Commit 消息窗口且未显式强行指定其他有效模型时使用 command_model
	isCommitPending := isCommitGenerationPending()
	if isCommitPending {
		// 消费掉 Commit 生成 pending 状态，防止残留在后续对话中
		clearCommitGenerationPending()
		if cmd := cfg.FeatureModelID("command"); cmd != "" && !hasExplicitModel {
			if m, ok := cfg.ResolveModelUID(cmd); ok {
				uiModel = m.ID
			} else {
				uiModel = cmd
			}
		}
	}

	// Fast Context（纯本地）：仅在非用户显式选模型的前提下，由 FC 标记或官方 mini 枚举判定覆盖
	fastCtx := isFastContextChat(parsed, userText, plain) || (!hasExplicitModel && looksLikeOfficialModelEnum(plain))
	if fastCtx && !hasExplicitModel {
		if fc := cfg.FeatureModelID("fast_context"); fc != "" {
			if m, ok := cfg.ResolveModelUID(fc); ok {
				uiModel = m.ID
			} else if m, ok := cfg.FindModel(fc); ok {
				uiModel = m.ID
			} else {
				uiModel = fc
			}
		} else {
			uiModel = cfg.DefaultModelID()
		}
		metricsAddLog("info", "fast-context local model="+uiModel)
		logx.Infof("fast-context pure-local model=%s", uiModel)
	}
	prov, okProv := cfg.ResolveProvider(uiModel)
	model := prov.UpstreamModel
	effort := cfg.ResolveThinking(uiModel)
	if !okProv || model == "" {
		msg := "未找到模型供应商配置: " + uiModel + "。请在「模型」页为 Family 配置 base_url / api_key / upstream_model。"
		metricsReqFail(uiModel)
		if isCommitGenerationPending() {
			metricsFeatureFail("commit", uiModel)
			clearCommitGenerationPending()
		}
		if fastCtx {
			metricsFeatureFail("fast_context", uiModel)
			clearFastContextPending()
		}
		metricsAddLog("error", msg)
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		_ = func() bool {
			flusher, _ := w.(http.Flusher)
			payload := buildGetChatMessageErrorDelta(pickMessageID(plain), msg)
			if _, err := w.Write(pbwire.ConnectFrame(0, payload)); err != nil {
				return false
			}
			_, _ = w.Write(pbwire.ConnectEndStream())
			if flusher != nil {
				flusher.Flush()
			}
			return true
		}()
		return
	}
	if !isTitleReq && !fastCtx && isModelIdentityQuestion(userText) {
		answer := modelIdentityAnswer(modelDisplayName(cfg, uiModel), userText)
		w.Header().Set("Content-Type", "application/connect+proto")
		w.Header().Set("Connect-Protocol-Version", "1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pbwire.ConnectFrame(0, buildGetChatMessageDelta(msgID, answer, "", false)))
		_, _ = w.Write(pbwire.ConnectEndStream())
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		metricsReqOK(uiModel, estimateTokens(userText), estimateTokens(answer), 0)
		metricsAddLog("info", "model identity answered locally model="+uiModel)
		return
	}
	// …/family … sampling
	maxTok := cfg.Upstream.Sampling.MaxTokens
	if me, ok := cfg.FindModel(uiModel); ok && me.MaxTokens > 0 {
		maxTok = me.MaxTokens
	}
	chatOpt := openai.ChatOptions{
		Thinking:             effort,
		ThinkingParam:        firstNonEmptyStr(prov.ThinkingParam, cfg.Upstream.Thinking.Param),
		ThinkingType:         prov.ThinkingType,
		ThinkingBudgetTokens: prov.ThinkingBudgetTokens,
		Temperature:          cfg.Upstream.Sampling.Temperature,
		MaxTokens:            maxTok,
		TopP:                 cfg.Upstream.Sampling.TopP,
		BaseURL:              prov.BaseURL,
		APIKey:               prov.APIKey,
	}
	// Cascade … tools.mode … function calling
	toolsMode := cfg.ToolsMode()
	filteredTools, toolsMode := filterToolsByPolicy(parsed.Tools, cfg)
	if cfg.Features.EnableCascadeTools && len(filteredTools) > 0 {
		chatOpt.Tools = filteredTools
	}
	// 工具场景使用 tools.timeout_sec（否则 upstream.timeout_sec），并下发到 HTTP client
	chatTimeoutSec := cfg.ResolveChatTimeoutSec(len(chatOpt.Tools) > 0)
	chatOpt.HTTPTimeout = time.Duration(chatTimeoutSec) * time.Second
	// cursor-byok 同款：按会话绑定 prompt_cache_key，提升上游 prompt 缓存命中
	if cfg.PromptCacheEnabled() {
		key := strings.TrimSpace(cfg.Cache.PromptCacheKey)
		if key == "" {
			cid := parsed.ConvID
			if cid == "" {
				cid = "default"
			}
			key = "devin-byok|" + cid + "|" + prov.BaseURL + "|" + model
		}
		chatOpt.PromptCacheKey = key
	}
	// …
	if cfg.ToolsWorkspaceHint() && needsWorkspaceHint(plain, userText, msgs) {
		msgs = injectWorkspaceHint(msgs)
	}
	// …
	if len(chatOpt.Tools) > 0 {
		msgs = injectCascadeToolsHint(msgs)
	}
	if fastCtx {
		msgs = injectFastContextAgentHint(msgs)
	}
	if isTitleReq {
		msgs = injectSystemNote(msgs, "按照用户的语言生成对应语言的标题", "对应语言的标题")
	}
	workspaceRoots := extractWorkspaceRoots(plain)
	route := "chat"
	if fastCtx {
		route = "fast_context"
	}
	composed := promptstore.ComposeMessages(msgs, promptstore.ComposeContext{
		Route: route, ModelID: uiModel, ModelName: modelDisplayName(cfg, uiModel), Family: prov.FamilyUID,
		UserText: userText, HasTools: len(chatOpt.Tools) > 0,
		HasWorkspace: len(workspaceRoots) > 0, QualityMode: cfg.QualityMode(), QualityEnabled: boolPtr(cfg.Quality.Enabled),
	})
	msgs = composed.Messages
	task := promptstore.DetectTask(userText)
	logx.Infof("chat-like %s msg=%s uiModel=%s upstreamModel=%s provider=%s base=%s thinking=%s route=%s task=%s quality=%s profiles=%v prompt_hash=%s stream=%v sysPrompt=%d toolsIn=%d toolsOut=%d mode=%s hist=%d plain=%d cascadeTools=%v workspace=%v",
		method, msgID, uiModel, model, prov.Provider, truncate(prov.BaseURL, 60), effort, route, task, cfg.QualityMode(), composed.ProfileIDs, composed.Hash, cfg.Features.EnableStream, len(parsed.SystemPrompt), len(parsed.Tools), len(filteredTools), toolsMode, len(parsed.Messages), len(plain), cfg.Features.EnableCascadeTools, workspaceRoots)
	metricsAddLog("info", fmt.Sprintf("chat model=%s tools=%d %q", uiModel, len(filteredTools), truncate(userText, 60)))
	metricsPromptContext(route, task, cfg.QualityMode(), effort, composed.ProfileIDs, composed.Hash)
	metricsAddLog("info", fmt.Sprintf("upstream start model=%s base=%s stream=%v tools=%d timeout=%ds", model, truncate(prov.BaseURL, 80), cfg.Features.EnableStream, len(chatOpt.Tools), cfg.ResolveChatTimeoutSec(len(chatOpt.Tools) > 0)))

	// 响应缓存：无工具时按 model+消息指纹命中
	cacheKey := ""
	if len(chatOpt.Tools) == 0 {
		cacheKey = respCacheKey(model, effort, composed.Hash, msgs, nil)
		if ent, ok := respCacheGet(cfg, cacheKey); ok {
			metricsAddLog("info", "cache hit model="+model)
			// 直接回放缓存（流式也一次性吐出，保证正确）
			msgID2 := msgID
			w.Header().Set("Content-Type", "application/connect+proto")
			w.Header().Set("Connect-Protocol-Version", "1")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			writeFrame := func(payload []byte) bool {
				if _, err := w.Write(pbwire.ConnectFrame(0, payload)); err != nil {
					return false
				}
				if flusher != nil {
					flusher.Flush()
				}
				return true
			}
			if ent.Thinking != "" {
				_ = writeFrame(buildGetChatMessageDelta(msgID2, "", ent.Thinking, true))
			}
			if ent.Text != "" {
				_ = writeFrame(buildGetChatMessageDelta(msgID2, ent.Text, "", false))
			} else {
				_ = writeFrame(buildGetChatMessageDelta(msgID2, "", "", false))
			}
			_, _ = w.Write(pbwire.ConnectEndStream())
			if flusher != nil {
				flusher.Flush()
			}
			metricsReqOK(uiModel, estimateTokens(userText), estimateTokens(ent.Text+ent.Thinking), 0)
			return
		}
	}

	timeoutSec := int(chatOpt.HTTPTimeout / time.Second)
	if timeoutSec <= 0 {
		timeoutSec = cfg.ResolveChatTimeoutSec(len(chatOpt.Tools) > 0)
		chatOpt.HTTPTimeout = time.Duration(timeoutSec) * time.Second
	}

	w.Header().Set("Content-Type", "application/connect+proto")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("Connect-Accept-Encoding", "gzip")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	// … goroutine ? keepAlive …
	var writeMu sync.Mutex
	writeFrame := func(payload []byte) bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err := w.Write(pbwire.ConnectFrame(0, payload)); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	writeDelta := func(text, thinking string, inProgress bool) bool {
		return writeFrame(buildGetChatMessageDelta(msgID, text, thinking, inProgress))
	}
	keepAlive := func() bool {
		return writeFrame(buildGetChatMessageResponse(msgID, "", "", true))
	}

	// 首帧 incomplete，防止客户端过早取消
	if !keepAlive() {
		logx.Warnf("chat-like client gone on first keepalive")
		return
	}

	type chatResult struct {
		text      string
		thinking  string
		toolCalls []openai.ToolCall
		usage     openai.Usage
		err       error
	}

	// 自动重试循环（处理区外路径工具调用报错，最多重试 5 次）
	var valid []openai.ToolCall
	var warn string
	var result chatResult
	const maxWorkspacePathRetries = 5
	const maxStreamInterruptedRetries = 2
	streamRetries := 0

	for retry := 0; retry <= maxWorkspacePathRetries; retry++ {
		done := make(chan chatResult, 1)
		upCtx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)

		if cfg.Features.EnableStream {
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						done <- chatResult{err: fmt.Errorf("stream panic: %v", rec)}
					}
				}()
				metricsAddLog("info", fmt.Sprintf("upstream streaming (attempt %d)...", retry+1))
				var content strings.Builder
				var thinking strings.Builder
				var tools []openai.ToolCall
				first := true
				usage, err := s.chatStream(upCtx, prov, model, msgs, chatOpt, func(d openai.StreamDelta) error {
					if first {
						first = false
						metricsAddLog("info", "upstream first token")
					}
					if d.Thinking != "" {
						thinking.WriteString(d.Thinking)
						if !writeDelta("", d.Thinking, true) {
							return context.Canceled
						}
					}
					if d.Content != "" {
						content.WriteString(d.Content)
						if !writeDelta(d.Content, "", true) {
							return context.Canceled
						}
					}
					// 只合并上游 tool delta，不在生成中途向 Devin 推工具参数。
					// 中途推累计/分片 JSON 会导致客户端追加错乱或过早执行，出现 TargetFile 丢失与截停。
					if len(d.ToolCalls) > 0 && cfg.Features.EnableCascadeTools {
						tools = openai.MergeToolCallDeltas(tools, d.ToolCalls)
					}
					return nil
				})
				done <- chatResult{text: content.String(), thinking: thinking.String(), toolCalls: tools, usage: usage, err: err}
			}()
		} else {
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						done <- chatResult{err: fmt.Errorf("chat panic: %v", rec)}
					}
				}()
				metricsAddLog("info", fmt.Sprintf("upstream non-stream request (attempt %d)...", retry+1))
				res, err := s.chatOnce(upCtx, prov, model, msgs, chatOpt)
				done <- chatResult{text: res.Content, thinking: res.Thinking, toolCalls: res.ToolCalls, usage: res.Usage, err: err}
			}()
		}

		keepEvery := 700 * time.Millisecond
		if len(chatOpt.Tools) > 0 {
			keepEvery = 300 * time.Millisecond // 工具场景更勤保活，降低 Devin 端超时
		}
		ticker := time.NewTicker(keepEvery)
		keepCount := 0
		waiting := true
		for waiting {
			select {
			case result = <-done:
				waiting = false
			case <-ticker.C:
				keepCount++
				// 工具等待时定期发 incomplete 心跳（假流式保活）
				ok := keepAlive()
				if ok && len(chatOpt.Tools) > 0 && keepCount%4 == 0 {
					// 轻量 thinking 心跳，避免客户端以为连接僵死
					ok = writeDelta("", "​", true)
				}
				if !ok {
					ticker.Stop()
					cancel()
					logx.Warnf("chat-like client disconnected during wait")
					return
				}
			case <-r.Context().Done():
				ticker.Stop()
				logx.Warnf("chat-like client context done while waiting")
				select {
				case result = <-done:
					waiting = false
				default:
					cancel()
					_ = writeDelta("BYOK: client canceled before upstream finished", "", false)
					_, _ = w.Write(pbwire.ConnectEndStream())
					if flusher != nil {
						flusher.Flush()
					}
					return
				}
			}
		}
		ticker.Stop()
		cancel()

		if result.err != nil {
			if errors.Is(result.err, openai.ErrStreamInterrupted) && streamRetries < maxStreamInterruptedRetries {
				streamRetries++
				// 把第一遍已收到的文本回填为 assistant 消息，让重试
				// 从中断处继续，避免向 Devin 重复输出。
				if strings.TrimSpace(result.text) != "" {
					msgs = append(msgs, openai.ChatMessage{Role: "assistant", Content: result.text})
				}
				metricsAddLog("warn", fmt.Sprintf("upstream stream interrupted, resuming (%d/%d) partial=%d chars", streamRetries, maxStreamInterruptedRetries, len(result.text)))
				continue
			}
			logx.Errorf("upstream chat: %v", result.err)
			metricsReqFail(uiModel)
			if isCommitGenerationPending() {
				metricsFeatureFail("commit", uiModel)
				clearCommitGenerationPending()
				metricsAddLog("error", "commit fail model="+uiModel+" err="+result.err.Error())
			}
			if fastCtx {
				metricsFeatureFail("fast_context", uiModel)
				clearFastContextPending()
				metricsAddLog("error", "fast-context fail model="+uiModel+" err="+result.err.Error())
			}
			metricsAddLog("error", "upstream fail: "+result.err.Error())
			_ = writeFrame(buildGetChatMessageErrorDelta(msgID, humanizeChatError(result.err)))
			_, _ = w.Write(pbwire.ConnectEndStream())
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		if len(result.toolCalls) > 0 && cfg.Features.EnableCascadeTools {
			valid, warn = validateToolCallsEx(result.toolCalls, workspaceRoots)
			if warn != "" {
				logx.Warnf("tool validate (attempt %d): %s", retry+1, warn)
			}

			// 如果所有工具调用均因区外路径被拦截，且重试次数未用尽，注入反思 Prompt 重新向模型发起请求
			if len(valid) == 0 && isWorkspacePathError(warn) && retry < maxWorkspacePathRetries {
				reflection := buildWorkspacePathRetryPrompt(warn, retry+1)
				logx.Warnf("workspace path error detected, retrying (%d/%d) with reflection prompt", retry+1, maxWorkspacePathRetries)
				metricsAddLog("warn", fmt.Sprintf("workspace path retry %d/%d: %s", retry+1, maxWorkspacePathRetries, warn))

				msgs = append(msgs, openai.ChatMessage{
					Role:      "assistant",
					Content:   result.text,
					ToolCalls: result.toolCalls,
				}, openai.ChatMessage{
					Role:    "user",
					Content: reflection,
				})
				continue
			}
		}
		break
	}

	text := strings.TrimSpace(result.text)
	thinking := strings.TrimSpace(result.thinking)
	if text == "" && thinking == "" && len(result.toolCalls) == 0 {
		text = "(empty upstream response)"
	}

	if !cfg.Features.EnableStream {
		if thinking != "" {
			_ = writeDelta("", thinking, true)
		}
		if len(result.toolCalls) > 0 && cfg.Features.EnableCascadeTools {
			views := toolCallViews(valid)
			if len(views) > 0 {
				if text != "" {
					_ = writeDelta(text, "", true)
				}
				emitToolCallsSmart(writeFrame, writeDelta, msgID, views)
				logx.Infof("chat-like tool_calls=%d names=%v", len(views), toolNames(views))
			} else if warn != "" {
				_ = writeFrame(buildGetChatMessageErrorDelta(msgID, warn))
			} else {
				_ = writeDelta(text, thinking, false)
			}
		} else {
			_ = writeDelta(text, thinking, false)
		}
	} else {
		// …/… FUNCTION_CALL
		if len(result.toolCalls) > 0 && cfg.Features.EnableCascadeTools {
			views := toolCallViews(valid)
			if len(views) > 0 {
				if text != "" {
					_ = writeDelta(text, "", true)
				}
				emitToolCallsSmart(writeFrame, writeDelta, msgID, views)
				logx.Infof("chat-like stream tool_calls=%d names=%v", len(views), toolNames(views))
			} else if warn != "" {
				_ = writeFrame(buildGetChatMessageErrorDelta(msgID, warn))
			} else {
				_ = writeDelta("", "", false)
			}
		} else {
			_ = writeDelta("", "", false)
		}
	}
	_, _ = w.Write(pbwire.ConnectEndStream())
	if flusher != nil {
		flusher.Flush()
	}
	// 写入响应缓存（无 tool_calls）
	if cacheKey != "" && len(result.toolCalls) == 0 {
		respCachePut(cfg, cacheKey, text, thinking, result.toolCalls)
	}
	tin, tout := estimateTokens(userText+parsed.SystemPrompt), estimateTokens(text+thinking)
	if result.usage.PromptTokens > 0 {
		tin = result.usage.PromptTokens
	}
	if result.usage.CompletionTokens > 0 {
		tout = result.usage.CompletionTokens
	}
	if result.usage.PromptTokens > 0 || result.usage.EffectiveCached() > 0 {
		metricsAddPromptUsage(result.usage.PromptTokens, result.usage.EffectiveCached(), result.usage.CacheWriteTokens, result.usage.CompletionTokens)
		metricsAddLog("info", fmt.Sprintf("prompt-cache model=%s prompt=%d cached=%d write=%d", uiModel, result.usage.PromptTokens, result.usage.EffectiveCached(), result.usage.CacheWriteTokens))
	}
	toolN := len(result.toolCalls)
	metricsReqOK(uiModel, tin, tout, toolN)
	if isCommitGenerationPending() {
		metricsFeatureOK("commit", uiModel, "")
		clearCommitGenerationPending()
		metricsAddLog("info", fmt.Sprintf("commit ok model=%s out_tokens~%d", uiModel, tout))
	}
	if fastCtx {
		metricsFeatureOK("fast_context", uiModel, "")
		clearFastContextPending()
		metricsAddLog("info", fmt.Sprintf("fast-context ok model=%s out_tokens~%d", uiModel, tout))
	}
	// 主 Cascade 若发起 find_code_context 工具，后续窗口内聊天按 Fast Context 计
	if toolCallsIncludeFastContext(result.toolCalls) {
		markFastContextPending()
		metricsAddLog("info", "fast-context pending after tool_calls")
	}
	metricsAddLog("info", fmt.Sprintf("done model=%s out_tokens~%d tools=%d cacheKey=%v", uiModel, tout, toolN, chatOpt.PromptCacheKey != ""))
	logx.Infof("chat-like done text=%d thinkingText=%d effort=%s stream=%v model=%s", len(text), len(thinking), effort, cfg.Features.EnableStream, model)
}

// isWriteEditCascadeTool 新建/写入/编辑类工具。
func isWriteEditCascadeTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	keys := []string{
		"write_to_file", "write_file", "edit_file", "edit_",
		"replace", "search_replace", "create_file", "apply_patch",
		"modify_file", "append_file",
	}
	for _, k := range keys {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// isHeavyCascadeTool 命令/写入/编辑等重工具：结束时用 incomplete 心跳保活，再一次下发完整 FUNCTION_CALL。
func isHeavyCascadeTool(name string) bool {
	if isWriteEditCascadeTool(name) {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(name))
	keys := []string{"run_command", "command", "delete_file"}
	for _, k := range keys {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

func hasHeavyCascadeTools(views []openaiToolCallView) bool {
	for _, v := range views {
		if isHeavyCascadeTool(v.Name) {
			return true
		}
	}
	return false
}

// isRunCommandTool 判断是否为 run_command
func isRunCommandTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(n, "run_command")
}

func hasRunCommandTool(views []openaiToolCallView) bool {
	for _, v := range views {
		if isRunCommandTool(v.Name) {
			return true
		}
	}
	return false
}

// emitToolCallsSmart 仅在参数已完整校验后调用。
// 禁止分片/累计中间帧：Devin 会对 delta_tool_calls 的 arguments 做追加或过早执行，导致 TargetFile 丢失与截停。
// 策略：轻量 incomplete 保活 →（可选）一帧完整 incomplete tool delta → FUNCTION_CALL 终帧。
// 对 run_command 特殊处理：跳过 incomplete tool delta 预览帧，直接下发带有 stopReasonFunctionCall 的终帧，防止 UI 卡片先空展开。
func emitToolCallsSmart(writeFrame func([]byte) bool, writeDelta func(text, thinking string, inProgress bool) bool, msgID string, views []openaiToolCallView) {
	if len(views) == 0 {
		return
	}
	if hasHeavyCascadeTools(views) {
		metricsAddLog("info", "tool-final complete names="+strings.Join(toolNames(views), ","))
		for i := 0; i < 3; i++ {
			_ = writeFrame(buildGetChatMessageDelta(msgID, "", "", true))
			time.Sleep(40 * time.Millisecond)
		}
		// 对于 run_command，直接发送 final 帧，避免下发 incomplete tool delta
		if !hasRunCommandTool(views) {
			_ = writeFrame(buildGetChatMessageToolDelta(msgID, views))
			time.Sleep(40 * time.Millisecond)
		}
	}
	_ = writeFrame(buildGetChatMessageToolFinal(msgID, views))
}

// emitToolCallsWithFakeStream 兼容旧调用名。
func emitToolCallsWithFakeStream(writeFrame func([]byte) bool, writeDelta func(text, thinking string, inProgress bool) bool, msgID string, views []openaiToolCallView) {
	emitToolCallsSmart(writeFrame, writeDelta, msgID, views)
}

func toolNames(views []openaiToolCallView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.Name)
	}
	return out
}

func toolCallViews(calls []openai.ToolCall) []openaiToolCallView {
	views := make([]openaiToolCallView, 0, len(calls))
	for _, tc := range calls {
		if strings.TrimSpace(tc.Function.Name) == "" {
			continue
		}
		id := tc.ID
		if id == "" {
			id = "call_" + tc.Function.Name
		}
		views = append(views, openaiToolCallView{ID: id, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return views
}

func pickCascadeSystemPrompt(body []byte) string {
	// field 2 在 wire 上是 length-delimited 大字符串，直接扫 "You are Cascade"
	s := string(body)
	markers := []string{
		"You are Cascade, a powerful agentic AI coding assistant.",
		"You are Cascade, a powerful agentic AI",
		"You are Cascade",
	}
	for _, m := range markers {
		i := strings.Index(s, m)
		if i < 0 {
			continue
		}
		// 取到 user_request 或合理上限
		rest := s[i:]
		if j := strings.Index(rest, "<user_request>"); j > 0 {
			rest = rest[:j]
		}
		if len(rest) > 120000 {
			rest = rest[:120000]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// pickConversationID 从 GetChatMessage 请求中提取会话 ID（字段 16/22 等 UUID）。
func pickConversationID(body []byte) string {
	uuids := extractUUIDs(body)
	if len(uuids) > 0 {
		return uuids[len(uuids)-1]
	}
	return ""
}

func pickMessageID(body []byte) string {
	uuids := extractUUIDs(body)
	if len(uuids) > 0 {
		return uuids[0]
	}
	return ""
}

func extractUUIDs(body []byte) []string {
	s := strings.ToLower(string(body))
	var out []string
	for i := 0; i+36 <= len(s); i++ {
		seg := s[i : i+36]
		if seg[8] != '-' || seg[13] != '-' || seg[18] != '-' || seg[23] != '-' {
			continue
		}
		ok := true
		for j, ch := range seg {
			if j == 8 || j == 13 || j == 18 || j == 23 {
				continue
			}
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, seg)
			i += 35
		}
	}
	seen := map[string]bool{}
	var uniq []string
	for _, u := range out {
		if !seen[u] {
			seen[u] = true
			uniq = append(uniq, u)
		}
	}
	return uniq
}

// decodeConnectPayloads 解析 connect 请求帧并解压。
func decodeConnectPayloads(raw []byte) []byte {
	if len(raw) < 5 {
		return maybeGunzip(raw)
	}
	var out []byte
	i := 0
	for i+5 <= len(raw) {
		flags := raw[i]
		n := int(raw[i+1])<<24 | int(raw[i+2])<<16 | int(raw[i+3])<<8 | int(raw[i+4])
		if n < 0 || i+5+n > len(raw) {
			break
		}
		payload := raw[i+5 : i+5+n]
		if flags&1 == 1 {
			payload = maybeGunzip(payload)
		}
		out = append(out, payload...)
		i += 5 + n
	}
	if len(out) == 0 {
		return maybeGunzip(raw)
	}
	return out
}

func (s *Server) writeProto(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/proto")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// writeProtoRPC 写本地 stub 并打日志，便于确认模型/额度 RPC 未再被官方空响应覆盖
func (s *Server) writeProtoRPC(w http.ResponseWriter, method string, payload []byte) {
	logx.Infof("local stub %s bytes=%d", method, len(payload))
	s.writeProto(w, payload)
}

// writeChatProxyError 混合模式官方聊天失败时，返回可被 Cascade 显示的错误 delta，避免空响应变 Internal error。
func (s *Server) writeChatProxyError(w http.ResponseWriter, method string, body, raw []byte, msg string) {
	plain := body
	if len(plain) == 0 {
		if p := decodeConnectPayloads(raw); len(p) > 0 {
			plain = p
		}
	}
	msgID := pickMessageID(plain)
	if msgID == "" {
		msgID = "byok-official-proxy-error"
	}
	w.Header().Set("Content-Type", "application/connect+proto")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.WriteHeader(http.StatusOK)
	payload := buildGetChatMessageErrorDelta(msgID, msg)
	_, _ = w.Write(pbwire.ConnectFrame(0, payload))
	_, _ = w.Write(pbwire.ConnectEndStream())
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	logx.Warnf("wrote chat proxy error method=%s msg=%s", method, msg)
}

func (s *Server) writeConnectError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    "internal",
		"message": msg,
	})
}

func (s *Server) dumpBody(method string, raw []byte) {
	if len(raw) == 0 {
		return
	}
	name := fmt.Sprintf("%s_%d.bin", sanitize(method), time.Now().UnixNano())
	_ = os.WriteFile(filepath.Join(s.bodyDir, name), raw, 0o644)
}

func (s *Server) appendRPC(rec map[string]any) {
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	b = append(b, '\n')
	if s.rpcRotator != nil {
		_, _ = s.rpcRotator.Write(b)
		return
	}
	// fallback
	f, err := os.OpenFile(s.rpcLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(b)
}

func methodName(path string) string {
	// /_route/api_server/exa.xxx.Service/Method
	path = strings.TrimPrefix(path, "/_route/api_server/")
	path = strings.TrimPrefix(path, "/")
	return path
}

func maybeGunzip(b []byte) []byte {
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return b
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return b
		}
		return out
	}
	return b
}

func extractStrings(b []byte, min int) []string {
	var out []string
	var cur []byte
	flush := func() {
		if len(cur) >= min && utf8.Valid(cur) {
			s := string(cur)
			if isMostlyPrintable(s) {
				out = append(out, s)
			}
		}
		cur = cur[:0]
	}
	for _, c := range b {
		if c >= 32 && c < 127 {
			cur = append(cur, c)
		} else {
			flush()
		}
	}
	flush()
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func isMostlyPrintable(s string) bool {
	if len(s) == 0 {
		return false
	}
	return true
}

func pickUserText(body []byte) string {
	// 1) Cascade 标准标签
	s := string(body)
	if i := strings.Index(s, "<user_request>"); i >= 0 {
		rest := s[i+len("<user_request>"):]
		if j := strings.Index(rest, "</user_request>"); j >= 0 {
			req := strings.TrimSpace(rest[:j])
			if req != "" {
				return req
			}
		}
	}
	// 2) 启发式
	cands := extractStrings(body, 2)
	best := ""
	for _, s := range cands {
		ls := strings.ToLower(s)
		if strings.Contains(ls, "http://") || strings.Contains(ls, "https://") {
			continue
		}
		if strings.Contains(ls, "exa.") || strings.Contains(ls, "protobuf") {
			continue
		}
		if strings.HasPrefix(s, "windsurf") || strings.HasPrefix(s, "devin") {
			continue
		}
		if strings.Contains(ls, "cascade") || strings.Contains(ls, "you are") {
			continue
		}
		if len(s) > len(best) && len(s) < 500 {
			best = s
		}
	}
	for _, s := range cands {
		if strings.Contains(s, "你好") || strings.EqualFold(s, "hello") || strings.EqualFold(s, "hi") {
			return s
		}
	}
	return best
}

func pickModel(body []byte, fallback string, allowed []string) string {
	raw := string(body)
	// 只允许返回配置内模型 ID，禁止把请求体里的 grok4.5 之类脏串当模型
	ids := append([]string(nil), allowed...)
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if len(ids[j]) > len(ids[i]) {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		if id != "" && strings.Contains(raw, id) {
			return id
		}
	}
	// 去分隔符后再匹配 allowed（body 含 grok4.5 时对齐 grok-4.5-byok-*）
	cands := extractStrings(body, 3)
	for _, str := range cands {
		for _, id := range ids {
			if id == "" {
				continue
			}
			if strings.EqualFold(str, id) {
				return id
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func truncateB64(b []byte, maxRaw int) string {
	if len(b) > maxRaw {
		b = b[:maxRaw]
	}
	return base64.StdEncoding.EncodeToString(b)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// shouldProxyOfficial 混合模式（pure_local=false）策略，对齐 dao 类「登录官方 + 能力分流」：
//   - 官方：GetUserJwt / GetProfileData（真实账号 session）、以及未列入本地的 RPC
//   - 本地：GetUserStatus（伪装 Pro）、模型列表、容量/限流 stub、Ping 等
//   - 聊天默认本地；Fast Context 在 serveRPC 里单独透传官方
//
// pure_local=true 时全部不透传。
func (s *Server) shouldProxyOfficial(method string) bool {
	// 仅单机：全部 RPC 本地处理，不透传 server.codeium.com
	_ = s
	_ = method
	return false
}

// shouldProxyChatOfficial 混合模式聊天分流（保守，避免把主 Cascade 误送官方导致 Internal error）：
//
//	默认本地 BYOK；仅在明确 Fast Context 或明确官方模型枚举时透传官方。
func (s *Server) shouldProxyChatOfficial(method string, plain []byte) bool {
	// 仅单机：聊天（含 Fast Context）全部本地，永不透传官方
	_, _, _ = s, method, plain
	return false
}

func (s *Server) proxyOfficial(w http.ResponseWriter, r *http.Request, raw []byte) error {
	// 映射到官方路径：去掉 /_route/api_server 前缀
	rel := strings.TrimPrefix(r.URL.Path, "/_route/api_server")
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	url := officialAPIBase + rel
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	for _, h := range []string{"Content-Type", "Content-Encoding", "Accept", "Accept-Encoding", "Connect-Protocol-Version", "Connect-Timeout-Ms", "User-Agent", "Authorization", "X-Api-Key", "X-Auth-Token", "Cookie"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if req.Header.Get("Connect-Protocol-Version") == "" {
		req.Header.Set("Connect-Protocol-Version", "1")
	}
	// 从 body / 缓存官方身份 注入鉴权，保证 Fast Context 官方调用带真账号
	if req.Header.Get("Authorization") == "" && req.Header.Get("X-Api-Key") == "" {
		if tok := extractSessionToken(raw); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("X-Api-Key", tok)
		} else if oid := getOfficialIdentity(); oid.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+oid.APIKey)
			req.Header.Set("X-Api-Key", oid.APIKey)
			logx.Infof("proxy official using cached api_key for %s", rel)
		} else if oid := getOfficialIdentity(); oid.RawJWT != "" {
			req.Header.Set("Authorization", "Bearer "+oid.RawJWT)
			logx.Infof("proxy official using cached JWT for %s", rel)
		}
	}
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logx.Infof("proxy official %s -> %d (%d bytes)", rel, resp.StatusCode, len(body))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500 {
		return fmt.Errorf("official status %d", resp.StatusCode)
	}
	// 混合登录：缓存官方 JWT 身份，供 GetUserStatus 展示真名而非 byok-local
	if strings.HasSuffix(methodName(rel), "GetUserJwt") || strings.HasSuffix(methodName(r.URL.Path), "GetUserJwt") {
		rememberOfficialJWTFromProxyBody(body)
	}
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "transfer-encoding" || lk == "connection" || lk == "content-length" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	return err
}

func extractSessionToken(raw []byte) string {
	// 在原始/解压后 body 中找 devin-session-token$...
	data := maybeGunzip(raw)
	s := string(data)
	const p = "devin-session-token$"
	i := strings.Index(s, p)
	if i < 0 {
		return ""
	}
	j := i
	for j < len(s) {
		c := s[j]
		if c == '"' || c == '\'' || c < 33 {
			break
		}
		j++
	}
	tok := s[i:j]
	// 去掉可能的尾部反斜杠引号残留
	tok = strings.TrimSuffix(tok, "\\")
	tok = strings.Trim(tok, "\"")
	return tok
}
