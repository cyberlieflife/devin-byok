package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"devin-byok/internal/logx"
	"devin-byok/internal/pbwire"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/openai"
)

// isDevstralRPC Fast Context / Instant Context 子代理走 GetDevstralStream，不是 GetChatMessage。
func isDevstralRPC(method string) bool {
	m := strings.ToLower(method)
	return strings.Contains(m, "getdevstralstream") || strings.Contains(m, "devstral")
}

type devstralReq struct {
	Messages []openai.ChatMessage
	Tools    []openai.Tool
	UserText string
}

// parseDevstralRequest 解析 GetDevstralStreamRequest。
// 1 metadata, 2 chat_message_prompts repeated, 3 tools_json
func parseDevstralRequest(plain []byte) devstralReq {
	var out devstralReq
	for _, f := range pbwire.ParseFields(plain) {
		switch f.Number {
		case 2: // ChatMessagePrompt
			if f.Wire != 2 {
				continue
			}
			msg, ok := parseDevstralPrompt(f.Bytes)
			if ok {
				out.Messages = append(out.Messages, msg)
				if msg.Role == "user" && out.UserText == "" {
					out.UserText = openai.TextContent(msg.Content)
				}
			}
		case 3: // tools_json
			if f.Wire == 2 {
				out.Tools = parseToolsJSON(string(f.Bytes))
			}
		}
	}
	return out
}

func parseDevstralPrompt(raw []byte) (openai.ChatMessage, bool) {
	// ChatMessagePrompt: 2 source, 3 prompt, 6 tool_calls, 7 tool_call_id
	var src int
	var prompt, toolCallID string
	var toolCalls []openai.ToolCall
	for _, f := range pbwire.ParseFields(raw) {
		switch f.Number {
		case 2:
			if f.Wire == 0 {
				src = int(f.Varint)
			}
		case 3:
			if f.Wire == 2 {
				prompt = string(f.Bytes)
			}
		case 6:
			if f.Wire == 2 {
				if tc, ok := parseChatToolCall(f.Bytes); ok {
					toolCalls = append(toolCalls, tc)
				}
			}
		case 7:
			if f.Wire == 2 {
				toolCallID = string(f.Bytes)
			}
		}
	}
	role := "user"
	// tool_call_id 优先：多轮 tool 结果绝不能被 source 误判成 user
	if toolCallID != "" {
		role = "tool"
	} else if len(toolCalls) > 0 {
		role = "assistant"
	} else {
		// ChatMessageSource：USER/SYSTEM/TOOL/SYSTEM_PROMPT/UNKNOWN（无独立 ASSISTANT）
		switch src {
		case 1: // USER（常见）
			role = "user"
		case 2: // 历史兼容：部分包体用 2 表示非 user；有 tool_calls 已在上面处理
			role = "user"
		case 3, 5: // SYSTEM / SYSTEM_PROMPT 一类
			role = "system"
		case 4, 6, 7: // TOOL 及兼容取值
			role = "tool"
		default:
			if strings.HasPrefix(strings.ToLower(prompt), "you are") {
				role = "system"
			} else {
				role = "user"
			}
		}
	}
	msg := openai.ChatMessage{Role: role, Content: prompt}
	if role == "assistant" && len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	if role == "tool" && toolCallID != "" {
		msg.ToolCallID = toolCallID
	}
	if strings.TrimSpace(prompt) == "" && len(toolCalls) == 0 && toolCallID == "" {
		return msg, false
	}
	return msg, true
}

func parseToolsJSON(s string) []openai.Tool {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var arr []openai.Tool
	if err := json.Unmarshal([]byte(s), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	var wrap struct {
		Tools []openai.Tool `json:"tools"`
	}
	if err := json.Unmarshal([]byte(s), &wrap); err == nil {
		return wrap.Tools
	}
	return nil
}

// buildDevstralResponse GetDevstralStreamResponse（LS rawDesc）:
//
//	string output = 2;
//	repeated ChatToolCall tool_calls = 3;
//
// 关键点：LS handlers.parseMistralToolCalls 从 output 文本解析
//
//	[TOOL_CALLS]name[ARGS]{json}
//
// 仅写 proto tool_calls、output 为空 → “Bad response format. The model returned no tool calls.”
// ChatToolCall: id=1 name=2 arguments_json=3；官方要求每轮恰好 1 个 tool call。
func buildDevstralResponse(output string, calls []openaiToolCallView) []byte {
	if len(calls) > 1 {
		calls = calls[:1]
	}
	if len(calls) == 1 {
		c := calls[0]
		args := c.Arguments
		if args == "" {
			args = "{}"
		}
		mistral := "[TOOL_CALLS]" + c.Name + "[ARGS]" + args
		if strings.TrimSpace(output) == "" {
			output = mistral
		} else if !strings.Contains(output, "[TOOL_CALLS]") {
			output = strings.TrimSpace(output) + "\n" + mistral
		}
	}
	if output != "" && !utf8.ValidString(output) {
		output = strings.ToValidUTF8(output, "")
	}
	var b []byte
	if output != "" {
		b = pbwire.AppendString(b, 2, output)
	}
	for _, c := range calls {
		var tc []byte
		if c.ID != "" {
			tc = pbwire.AppendString(tc, 1, c.ID)
		}
		if c.Name != "" {
			tc = pbwire.AppendString(tc, 2, c.Name)
		}
		args := c.Arguments
		if args == "" {
			args = "{}"
		}
		if !utf8.ValidString(args) {
			args = strings.ToValidUTF8(args, "")
		}
		tc = pbwire.AppendString(tc, 3, args)
		b = pbwire.AppendMessage(b, 3, tc)
	}
	return b
}

func (s *Server) handleGetDevstralStream(w http.ResponseWriter, r *http.Request, method string, body, raw []byte) {
	cfg := s.GetConfig()
	// Connect 帧内 gzip：必须先解帧，否则 tools_json/prompts 全解析失败 -> 无 tool_calls -> Skipped
	plain := decodeConnectPayloads(raw)
	if len(plain) == 0 {
		plain = maybeGunzip(body)
	}
	if len(plain) == 0 {
		plain = body
	}
	req := parseDevstralRequest(plain)
	msgs := req.Messages
	tools := req.Tools
	if len(tools) == 0 {
		tools = extractToolsFromPlain(plain)
	}

	userText := req.UserText
	if userText == "" && len(msgs) > 0 {
		userText = openai.TextContent(msgs[len(msgs)-1].Content)
	}

	// 解析失败：仍必须立刻回 tool_calls，否则 LS Bad response format
	if len(msgs) == 0 {
		logx.Warnf("devstral empty parse raw=%d body=%d plain=%d; synthesize tool call", len(raw), len(body), len(plain))
		metricsAddLog("warn", fmt.Sprintf("devstral parse fail plain=%d; synth tool", len(plain)))
		metricsFeatureFail("fast_context", "parse")
		synth := synthesizeRestrictedExec(userText, "")
		s.writeDevstralStream(w, func(write func([]byte) bool) {
			_ = write(buildDevstralResponse("", toolCallViews(synth)))
		})
		return
	}

	// Fast Context 子代理固定用 fast_context_model
	uiModel := cfg.FeatureModelID("fast_context")
	if uiModel == "" {
		uiModel = cfg.DefaultModelID()
	}
	if m, ok := cfg.ResolveModelUID(uiModel); ok {
		uiModel = m.ID
	}

	// 关键点：LS 对 FIND_CODE_CONTEXT / GetDevstralStream 只有约 5~6 秒窗口。
	// 实测等上游 grok 出 tool_calls 必触发 context deadline exceeded -> UI Skipped。
	// 首轮（尚无 tool 结果）立即合成 restricted_exec，让 LS 先本地检索；后续轮再短超时调模型写 answer。
	if !devstralHasToolResults(msgs) {
		synth := synthesizeRestrictedExec(userText, "")
		logx.Infof("devstral/fast-context FIRST-TURN fast-synth model=%s toolsIn=%d user=%q plain=%d", uiModel, len(tools), truncate(userText, 80), len(plain))
		metricsAddLog("info", fmt.Sprintf("devstral first-turn synth restricted_exec %q", truncate(userText, 60)))
		s.writeDevstralStream(w, func(write func([]byte) bool) {
			_ = write(buildDevstralResponse("", toolCallViews(synth)))
		})
		metricsFeatureOK("fast_context", uiModel, "first_synth")
		return
	}

	prov, okProv := cfg.ResolveProvider(uiModel)
	model := prov.UpstreamModel
	if !okProv || model == "" {
		logx.Warnf("Fast Context 未配置可用模型: %s；使用 tool 结果合成 answer", uiModel)
		synth := synthesizeAnswerTool(collectDevstralToolResultText(msgs), userText)
		s.writeDevstralStream(w, func(write func([]byte) bool) {
			_ = write(buildDevstralResponse("", toolCallViews(synth)))
		})
		metricsFeatureFail("fast_context", uiModel)
		return
	}

	composed := promptstore.ComposeMessages(msgs, promptstore.ComposeContext{
		Route: "fast_context", ModelID: uiModel, Family: prov.FamilyUID,
		UserText: userText, HasTools: len(tools) > 0, QualityMode: "fast", QualityEnabled: boolPtr(cfg.Quality.Enabled),
	})
	msgs = composed.Messages
	metricsPromptContext("fast_context", promptstore.DetectTask(userText), "fast", cfg.ResolveThinking(uiModel), composed.ProfileIDs, composed.Hash)

	// Fast Context 后续轮：强制小输出 + 短超时，禁止沿用 family 的 16k max_tokens
	maxTok := 1024
	if me, ok := cfg.FindModel(uiModel); ok && me.MaxTokens > 0 && me.MaxTokens < maxTok {
		maxTok = me.MaxTokens
	}
	var toolChoice any
	if len(tools) > 0 {
		toolChoice = "required"
	}
	// 给上游最多约 3.5s；整段 RPC 必须在 LS 约 6s 窗口内结束
	const fcUpstreamBudget = 3500 * time.Millisecond
	chatOpt := openai.ChatOptions{
		Thinking:             cfg.ResolveThinking(uiModel),
		ThinkingParam:        firstNonEmptyStr(prov.ThinkingParam, cfg.Upstream.Thinking.Param),
		ThinkingType:         prov.ThinkingType,
		ThinkingBudgetTokens: prov.ThinkingBudgetTokens,
		Temperature:          cfg.Upstream.Sampling.Temperature,
		MaxTokens:            maxTok,
		TopP:                 cfg.Upstream.Sampling.TopP,
		BaseURL:              prov.BaseURL,
		APIKey:               prov.APIKey,
		Tools:                tools,
		ToolChoice:           toolChoice,
		HTTPTimeout:          fcUpstreamBudget + 500*time.Millisecond,
	}

	logx.Infof("devstral/fast-context LATER-TURN model=%s upstream=%s tools=%d user=%q plain=%d budget=%s", uiModel, model, len(tools), truncate(userText, 80), len(plain), fcUpstreamBudget)
	metricsAddLog("info", fmt.Sprintf("devstral later-turn model=%s tools=%d %q", uiModel, len(tools), truncate(userText, 60)))

	upCtx, cancel := context.WithTimeout(context.Background(), fcUpstreamBudget)
	defer cancel()

	s.writeDevstralStream(w, func(write func([]byte) bool) {
		var content strings.Builder
		var toolsAcc []openai.ToolCall

		// 后续轮不要边流边写无 tool 的 output 帧；只在终局写 tool_calls
		res, err := s.chatOnce(upCtx, prov, model, msgs, chatOpt)
		if err == nil {
			if res.Content != "" {
				content.WriteString(res.Content)
			}
			toolsAcc = res.ToolCalls
		} else {
			logx.Warnf("devstral later-turn upstream err: %v ; fallback synth answer", err)
			metricsAddLog("warn", "devstral later-turn upstream fail; synth answer")
		}

		if len(toolsAcc) == 0 {
			toolsAcc = parseTextToolCalls(content.String())
		}
		toolBlob := collectDevstralToolResultText(msgs)
		if len(toolsAcc) == 0 {
			blob := strings.TrimSpace(content.String())
			if blob == "" {
				blob = toolBlob
			}
			toolsAcc = synthesizeAnswerTool(blob+"\n"+toolBlob, userText)
			logx.Warnf("devstral synthesized answer tool (later turn)")
			metricsAddLog("warn", "devstral synthesized answer")
		} else {
			// 模型若回了 answer 但 XML 不合格 → 纠正，避免 Bad answer format
			toolsAcc = ensureAnswerToolXML(toolsAcc, toolBlob+"\n"+content.String(), userText)
		}
		// 后续轮优先保证最终能收口为 answer（若仍是 restricted_exec 也可，但无结果时改 answer）
		if len(toolsAcc) > 0 && toolsAcc[0].Function.Name != "answer" && strings.TrimSpace(toolBlob) != "" {
			// 已有检索结果时直接给 answer，节省回合、避免再拖过 deadline
			toolsAcc = synthesizeAnswerTool(toolBlob+"\n"+content.String(), userText)
			logx.Infof("devstral force answer after tool results")
			metricsAddLog("info", "devstral force answer after tool results")
		}

		views := toolCallViews(toolsAcc)
		if len(views) == 0 {
			views = toolCallViews(synthesizeAnswerTool(toolBlob, userText))
		}
		if len(views) > 0 {
			_ = write(buildDevstralResponse("", views))
			metricsFeatureOK("fast_context", uiModel, "later")
		} else {
			_ = write(buildDevstralResponse("(empty Fast Context agent response)", nil))
			metricsFeatureFail("fast_context", uiModel)
		}
		metricsAddLog("info", fmt.Sprintf("devstral later done model=%s out=%d tools=%d err=%v", uiModel, content.Len(), len(views), err != nil))
		logx.Infof("devstral later done model=%s out=%d tools=%d", uiModel, content.Len(), len(views))
	})
}

// collectDevstralToolResultText 汇总已执行的 tool 消息，供 answer 兜底。
func collectDevstralToolResultText(msgs []openai.ChatMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		t := strings.TrimSpace(openai.TextContent(m.Content))
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		if len(t) > 1500 {
			t = t[:1500] + "..."
		}
		b.WriteString(t)
		if b.Len() > 5000 {
			b.WriteString("\n...")
			break
		}
	}
	return b.String()
}

func (s *Server) writeDevstralStream(w http.ResponseWriter, fn func(write func([]byte) bool)) {
	w.Header().Set("Content-Type", "application/connect+proto")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("Connect-Accept-Encoding", "gzip")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex
	write := func(payload []byte) bool {
		mu.Lock()
		defer mu.Unlock()
		if _, err := w.Write(pbwire.ConnectFrame(0, payload)); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	fn(write)
	_, _ = w.Write(pbwire.ConnectEndStream())
	if flusher != nil {
		flusher.Flush()
	}
}

func devstralNeedsForcedTool(msgs []openai.ChatMessage) bool {
	// 已有 tool 结果的多轮：auto；否则首轮 required
	for _, m := range msgs {
		if m.Role == "tool" {
			return false
		}
	}
	return true
}

func extractToolsFromPlain(plain []byte) []openai.Tool {
	// 在整包中找 OpenAI tools JSON 数组
	s := string(plain)
	idx := strings.Index(s, `[{"type": "function"`)
	if idx < 0 {
		idx = strings.Index(s, `[{"type":"function"`)
	}
	if idx < 0 {
		return nil
	}
	// 粗平衡括号截取
	depth := 0
	for i := idx; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return parseToolsJSON(s[idx : i+1])
			}
		}
	}
	return nil
}

// parseTextToolCalls 解析 [TOOL_CALLS]name[ARGS]{json} 或多个连续调用。
func parseTextToolCalls(text string) []openai.ToolCall {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []openai.ToolCall
	// 模式1：OpenAI 风格已在 stream 处理
	// 模式2：Devstral 标签
	marker := "[TOOL_CALLS]"
	rest := text
	n := 0
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		// name until [ARGS]
		j := strings.Index(rest, "[ARGS]")
		if j < 0 {
			break
		}
		name := strings.TrimSpace(rest[:j])
		rest = rest[j+len("[ARGS]"):]
		// json object
		jb, jr, ok := extractJSONObject(rest)
		if !ok {
			break
		}
		rest = jr
		n++
		id := fmt.Sprintf("call_fc_%d", n)
		tc := openai.ToolCall{ID: id, Type: "function"}
		tc.Function.Name = name
		tc.Function.Arguments = jb
		out = append(out, tc)
		if n >= 8 {
			break
		}
	}
	return out
}

func extractJSONObject(s string) (jsonStr string, rest string, ok bool) {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	if start < 0 {
		return "", s, false
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1], s[i+1:], true
			}
		}
	}
	return "", s, false
}

func synthesizeRestrictedExec(userText, modelText string) []openai.ToolCall {
	q := strings.TrimSpace(userText)
	if q == "" {
		q = strings.TrimSpace(modelText)
	}
	pat := "README|package.json|main|src|app|index"
	for _, w := range strings.Fields(q) {
		w = strings.Trim(w, " \t?,.\"'`")
		if len(w) >= 4 && len(w) < 32 {
			// only simple ascii tokens as rg pattern
			ok := true
			for i := 0; i < len(w); i++ {
				c := w[i]
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
					ok = false
					break
				}
			}
			if ok {
				pat = regexp.QuoteMeta(w)
				break
			}
		}
	}
	// 与官方 tools_json 对齐：command1..command6，type 仅 rg|readfile|tree
	args := map[string]any{
		// 官方 oneOf：rg 必填 type/pattern/path；先只发 1 条，降低校验失败概率
		"command1": map[string]any{"type": "rg", "pattern": pat, "path": "."},
		"command2": map[string]any{"type": "tree", "path": ".", "levels": 2},
		"command3": map[string]any{"type": "readfile", "file": "README.md"},
	}
	raw, _ := json.Marshal(args)
	tc := openai.ToolCall{ID: "call_fc_synth_1", Type: "function"}
	tc.Function.Name = "restricted_exec"
	tc.Function.Arguments = string(raw)
	return []openai.ToolCall{tc}
}

func devstralHasToolResults(msgs []openai.ChatMessage) bool {
	for _, m := range msgs {
		if m.Role == "tool" || strings.TrimSpace(m.ToolCallID) != "" {
			return true
		}
		// 已有 assistant tool_calls 历史，说明至少完成过首轮调用
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func synthesizeAnswerTool(modelText, userText string) []openai.ToolCall {
	xml := buildFastContextAnswerXML(modelText, userText)
	tc := openai.ToolCall{ID: "call_fc_answer_1", Type: "function"}
	tc.Function.Name = "answer"
	tc.Function.Arguments = marshalFastContextJSON(map[string]any{"answer": xml})
	return []openai.ToolCall{tc}
}

// marshalFastContextJSON 关闭 HTML 转义，避免 <ANSWER> 变成 \u003c 导致 LS 文本/校验失败。
func marshalFastContextJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		b, _ := json.Marshal(v)
		return string(b)
	}
	return strings.TrimSpace(buf.String())
}

// buildFastContextAnswerXML 生成 LS 校验的严格 ANSWER XML。
// 官方要求（系统提示 ANSWER FORMAT）:
//
//	<ANSWER>
//	  <file path="/codebase/...">
//	    <range>10-60</range>
//	  </file>
//	</ANSWER>
func buildFastContextAnswerXML(toolResultText, userText string) string {
	paths := extractFastContextPaths(toolResultText + "\n" + userText)
	if len(paths) == 0 {
		paths = []string{"/codebase/README.md", "/codebase/package.json", "/codebase/src"}
	}
	if len(paths) > 8 {
		paths = paths[:8]
	}
	var b strings.Builder
	b.WriteString("<ANSWER>\n")
	for _, path := range paths {
		path = normalizeCodebasePath(path)
		b.WriteString("  <file path=\"")
		b.WriteString(path)
		b.WriteString("\">\n")
		b.WriteString("    <range>1-80</range>\n")
		b.WriteString("  </file>\n")
	}
	b.WriteString("</ANSWER>")
	return b.String()
}

func normalizeCodebasePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || p == "." || p == "./" {
		return "/codebase"
	}
	if strings.HasPrefix(p, "/codebase/") || p == "/codebase" {
		return p
	}
	if strings.HasPrefix(p, "codebase/") {
		return "/" + p
	}
	// Windows abs → 只取最后若干段意义不大，常见是仓库相对路径
	p = strings.TrimPrefix(p, "./")
	if len(p) >= 2 && p[1] == ':' { // C:\...
		// 取 base 名兜底
		if i := strings.LastIndex(p, "/"); i >= 0 {
			p = p[i+1:]
		}
	}
	if !strings.HasPrefix(p, "/") {
		p = "/codebase/" + p
	} else if !strings.HasPrefix(p, "/codebase") {
		p = "/codebase" + p
	}
	return p
}

func extractFastContextPaths(blob string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if strings.Contains(p, " ") && !strings.Contains(p, "/") && !strings.Contains(p, ".") {
			return // 跳过纯句子
		}
		p = normalizeCodebasePath(p)
		if p == "/codebase" || seen[p] {
			return
		}
		// 过滤明显非路径
		base := p
		if i := strings.LastIndex(p, "/"); i >= 0 {
			base = p[i+1:]
		}
		if base == "" {
			return
		}
		seen[p] = true
		out = append(out, p)
	}

	// 1) 显式 path="/..." 或 path='...'
	for _, m := range regexp.MustCompile(`path\s*=\s*"([^"]+)"`).FindAllStringSubmatch(blob, -1) {
		add(m[1])
	}
	// 2) tree/rg 结果里的文件名行：README.md、src/app.ts、./foo
	for _, line := range strings.Split(blob, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "├── ")
		line = strings.TrimPrefix(line, "└── ")
		line = strings.TrimPrefix(line, "│   ")
		line = strings.Trim(line, " \t`|")
		if line == "" || strings.HasPrefix(line, "<") || strings.HasPrefix(line, "{") {
			continue
		}
		// 单段带扩展名或常见目录
		if strings.Contains(line, "/") || strings.Contains(line, ".") {
			if len(line) < 120 && !strings.Contains(line, "://") {
				// 排除 JSON key 等
				if strings.Count(line, " ") == 0 || strings.HasPrefix(line, "/codebase") {
					add(line)
				}
			}
		}
		switch line {
		case "README.md", "package.json", "go.mod", "src", "app", "lib", "cmd", "internal":
			add(line)
		}
	}
	// 3) 常见锚点
	for _, k := range []string{"README.md", "package.json", "src"} {
		if strings.Contains(blob, k) {
			add(k)
		}
	}
	return out
}

// ensureAnswerToolXML 若模型返回了 answer 但 XML 不合格，则重写为合法 ANSWER。
func ensureAnswerToolXML(calls []openai.ToolCall, toolResultText, userText string) []openai.ToolCall {
	if len(calls) == 0 {
		return synthesizeAnswerTool(toolResultText, userText)
	}
	out := make([]openai.ToolCall, 0, len(calls))
	for _, c := range calls {
		name := c.Function.Name
		if name != "answer" {
			out = append(out, c)
			continue
		}
		args := strings.TrimSpace(c.Function.Arguments)
		ans := ""
		var wrap struct {
			Answer string `json:"answer"`
		}
		if json.Unmarshal([]byte(args), &wrap) == nil {
			ans = wrap.Answer
		}
		if !strings.Contains(ans, "<ANSWER>") || !strings.Contains(ans, "<file") || !strings.Contains(ans, "<range>") {
			// 不合格 → 用工具结果重生成
			fixed := synthesizeAnswerTool(toolResultText+"\n"+ans, userText)
			if len(fixed) > 0 {
				out = append(out, fixed[0])
				continue
			}
		}
		out = append(out, c)
	}
	// 若模型又回了 restricted_exec 且已是后续轮，可保留；若完全没有 answer 且结果已够，可追加 answer
	return out
}
