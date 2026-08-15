package localapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"devin-byok/internal/logx"
	"devin-byok/internal/pbwire"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/openai"
)

// CodeMap 相关 RPC（多数由 Language Server 本机处理；若打到 api_server 则在此兜底）。

func isCodeMapRPC(method string) bool {
	m := strings.ToLower(method)
	keys := []string{
		"generatecodemap",
		"branchcascadeandgeneratecodemap",
		"getcodemapsforrepos",
		"getcodemapsforfile",
		"getcodemapsuggestions",
		"sharecodemap",
		"getsharedcodemap",
		"updatecodemapmetadata",
		"savecodemapfromjson",
		"dismisscodemapsuggestion",
		"loadcodemap",
	}
	for _, k := range keys {
		if strings.Contains(m, k) {
			return true
		}
	}
	return false
}

func (s *Server) handleCodeMapRPC(w http.ResponseWriter, r *http.Request, method string, body, raw []byte) {
	cfg := s.GetConfig()
	if !cfg.Features.EnableCodeMap {
		// 关闭时返回空成功，避免 UI 报错
		s.writeProtoRPC(w, method, []byte{})
		return
	}
	ml := strings.ToLower(method)
	plain := body
	if len(plain) == 0 {
		if p := decodeConnectPayloads(raw); len(p) > 0 {
			plain = p
		}
	}

	switch {
	case strings.Contains(ml, "generatecodemap") && !strings.Contains(ml, "branch"):
		s.handleGenerateCodeMap(w, r, method, plain, false)
	case strings.Contains(ml, "branchcascadeandgeneratecodemap"):
		s.handleGenerateCodeMap(w, r, method, plain, true)
	case strings.Contains(ml, "getcodemapsforrepos"),
		strings.Contains(ml, "getcodemapsforfile"):
		// code_maps repeated string = empty
		s.writeProtoRPC(w, method, []byte{})
	case strings.Contains(ml, "getcodemapsuggestions"):
		s.writeProtoRPC(w, method, []byte{})
	case strings.Contains(ml, "sharecodemap"):
		// ShareCodeMapResponse { share_url=1 }
		id := fmt.Sprintf("local-%d", time.Now().UnixNano())
		url := strings.TrimRight(cfg.Server.PublicBase, "/") + "/codemap/" + id
		var b []byte
		b = pbwire.AppendString(b, 1, url)
		s.writeProtoRPC(w, method, b)
	case strings.Contains(ml, "getsharedcodemap"):
		// GetSharedCodeMapResponse { code_map_data=1 }
		var b []byte
		b = pbwire.AppendString(b, 1, `{"title":"BYOK Shared CodeMap","items":[]}`)
		s.writeProtoRPC(w, method, b)
	case strings.Contains(ml, "savecodemapfromjson"):
		// 回显请求中的 code_map_json
		jsonText := extractProtoStringField(plain, 2)
		if jsonText == "" {
			jsonText = `{"title":"saved","items":[]}`
		}
		var b []byte
		b = pbwire.AppendString(b, 1, jsonText)
		s.writeProtoRPC(w, method, b)
	case strings.Contains(ml, "updatecodemapmetadata"),
		strings.Contains(ml, "dismisscodemapsuggestion"),
		strings.Contains(ml, "loadcodemap"):
		s.writeProtoRPC(w, method, []byte{})
	default:
		s.writeProtoRPC(w, method, []byte{})
	}
}

func extractProtoStringField(plain []byte, want int) string {
	var out string
	walkProto(plain, func(fn int, wt int, v []byte, num uint64) {
		if fn == want && wt == 2 && out == "" {
			out = string(v)
		}
	})
	return out
}

func parseGenerateCodeMapRequest(plain []byte) (prompt, mode, source, cascadeID, editingID string) {
	// GenerateCodeMapRequest: 1 prompt, 2 mode, 3 source
	// Branch: 1 cascade_id, 2 prompt, 3 source, 4 editing_codemap_id, 5 mode
	walkProto(plain, func(fn int, wt int, v []byte, num uint64) {
		if wt != 2 {
			return
		}
		s := string(v)
		switch fn {
		case 1:
			// 可能是 prompt 或 cascade_id
			if strings.Contains(strings.ToLower(s), "cascade") || len(s) == 36 {
				cascadeID = s
			} else if prompt == "" {
				prompt = s
			} else {
				cascadeID = s
			}
		case 2:
			// prompt or mode
			if prompt == "" && (strings.Contains(s, " ") || len(s) > 24) {
				prompt = s
			} else if mode == "" {
				mode = s
			}
		case 3:
			source = s
		case 4:
			editingID = s
		case 5:
			mode = s
		}
	})
	// 若 field1 被当成 cascade 且 prompt 空，再扫字符串
	if prompt == "" {
		for _, s := range extractStrings(plain, 8) {
			if strings.Contains(s, " ") || len(s) > 16 {
				// 跳过元数据噪声
				if strings.Contains(s, "windsurf") || strings.Contains(s, "Windows") {
					continue
				}
				prompt = s
				break
			}
		}
	}
	// 规范化 mode：优先识别 fast/smart/agent
	mode = normalizeCodeMapMode(mode, plain)
	return
}

func normalizeCodeMapMode(mode string, plain []byte) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "fast" || m == "smart" || m == "agent" {
		return m
	}
	// 从原文扫描短字段
	for _, s := range extractStrings(plain, 3) {
		ls := strings.ToLower(strings.TrimSpace(s))
		if ls == "fast" || ls == "smart" || ls == "agent" {
			return ls
		}
	}
	if strings.Contains(m, "fast") {
		return "fast"
	}
	if strings.Contains(m, "smart") {
		return "smart"
	}
	if m == "" {
		return "smart"
	}
	return m
}

func (s *Server) handleGenerateCodeMap(w http.ResponseWriter, r *http.Request, method string, plain []byte, branch bool) {
	cfg := s.GetConfig()
	prompt, mode, source, cascadeID, editingID := parseGenerateCodeMapRequest(plain)
	if strings.TrimSpace(prompt) == "" {
		prompt = "Summarize this repository into a code map of key modules and entry points."
	}

	// 按 Fast/Smart 模式选择不同模型绑定
	uiModel := cfg.FeatureModelIDForCodeMapMode(mode)
	prov, okProv := cfg.ResolveProvider(uiModel)
	logx.Infof("codemap generate branch=%v mode=%q source=%q cascade=%q edit=%q model=%s prompt=%q",
		branch, mode, source, cascadeID, editingID, uiModel, truncate(prompt, 80))
	metricsAddLog("info", "codemap generate: "+truncate(prompt, 60))

	// Connect streaming response
	w.Header().Set("Content-Type", "application/connect+proto")
	w.Header().Set("Connect-Protocol-Version", "1")
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

	// status 帧
	_ = write(buildCodeMapStatus("generating"))

	jsonText := ""
	if okProv {
		upModel := prov.UpstreamModel
		sys := "You generate a CodeMap JSON document for an IDE. " +
			"Return ONLY valid JSON (no markdown fences) with shape: " +
			`{"title":string,"summary":string,"items":[{"title":string,"description":string,"content":string,"location":string}]}` +
			". Keep items under 20. location should be path or path:line when known."
		msgs := []openai.ChatMessage{
			{Role: "system", Content: sys},
			{Role: "user", Content: prompt},
		}
		composed := promptstore.ComposeMessages(msgs, promptstore.ComposeContext{
			Route: "codemap", ModelID: uiModel, Family: prov.FamilyUID,
			UserText: prompt, QualityMode: "balanced", QualityEnabled: boolPtr(cfg.Quality.Enabled),
		})
		msgs = composed.Messages
		metricsPromptContext("codemap", promptstore.DetectTask(prompt), "balanced", cfg.ResolveThinking(uiModel), composed.ProfileIDs, composed.Hash)
		maxTok := 2048
		if m, ok := cfg.FindModel(uiModel); ok && m.MaxTokens > 0 && m.MaxTokens < maxTok {
			maxTok = m.MaxTokens
		}
		opt := openai.ChatOptions{
			Thinking:             cfg.ResolveThinking(uiModel),
			ThinkingParam:        firstNonEmptyStr(prov.ThinkingParam, cfg.Upstream.Thinking.Param),
			ThinkingType:         prov.ThinkingType,
			ThinkingBudgetTokens: prov.ThinkingBudgetTokens,
			MaxTokens:            maxTok,
			BaseURL:              prov.BaseURL,
			APIKey:               prov.APIKey,
		}
		timeoutSec := cfg.ResolveChatTimeoutSec(false)
		if timeoutSec <= 0 {
			timeoutSec = 120
		}
		upCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		defer cancel()
		res, err := s.chatOnce(upCtx, prov, upModel, msgs, opt)
		if err != nil {
			logx.Errorf("codemap upstream: %v", err)
			metricsReqFail(uiModel)
			metricsFeatureFail("codemap", uiModel)
			// 回落本地模板
			jsonText = fallbackCodeMapJSON(prompt, humanizeChatError(err))
		} else {
			jsonText = sanitizeCodeMapJSON(res.Content, prompt)
			if res.Usage.PromptTokens > 0 || res.Usage.EffectiveCached() > 0 {
				metricsAddPromptUsage(res.Usage.PromptTokens, res.Usage.EffectiveCached(), res.Usage.CacheWriteTokens, res.Usage.CompletionTokens)
			}
			metricsReqOK(uiModel, estimateTokens(prompt), estimateTokens(jsonText), 0)
			metricsFeatureOK("codemap", uiModel, strings.ToLower(strings.TrimSpace(mode)))
		}
	} else {
		jsonText = fallbackCodeMapJSON(prompt, "no provider configured")
		metricsFeatureFail("codemap", uiModel)
	}

	_ = write(buildCodeMapSuccess(jsonText, cascadeID, branch))
	mu.Lock()
	_, _ = w.Write(pbwire.ConnectEndStream())
	mu.Unlock()
	if flusher != nil {
		flusher.Flush()
	}
}

// buildCodeMapStatus GenerateCodeMapResponse.result = status(field 3)
func buildCodeMapStatus(status string) []byte {
	var b []byte
	b = pbwire.AppendString(b, 3, status)
	return b
}

// buildCodeMapSuccess result = success(field 2) { code_map_json=1, new_cascade_id?=2 }
func buildCodeMapSuccess(codeMapJSON, cascadeID string, branch bool) []byte {
	var suc []byte
	suc = pbwire.AppendString(suc, 1, codeMapJSON)
	if branch {
		if cascadeID == "" {
			cascadeID = fmt.Sprintf("byok-cascade-%d", time.Now().UnixNano())
		}
		suc = pbwire.AppendString(suc, 2, cascadeID)
	}
	var b []byte
	b = pbwire.AppendMessage(b, 2, suc)
	return b
}

func sanitizeCodeMapJSON(raw, prompt string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fallbackCodeMapJSON(prompt, "empty model output")
	}
	// 去掉 ```json 围栏
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```JSON")
		s = strings.TrimPrefix(s, "```")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	// 截取第一个 { ... } 对象
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	var tmp any
	if err := json.Unmarshal([]byte(s), &tmp); err != nil {
		return fallbackCodeMapJSON(prompt, "invalid json from model")
	}
	return normalizeCodeMapDoc(s, prompt)
}

func fallbackCodeMapJSON(prompt, note string) string {
	type node struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Location    string `json:"location"`
	}
	type edge struct {
		From  string `json:"from"`
		To    string `json:"to"`
		Label string `json:"label,omitempty"`
	}
	type item struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Location    string `json:"location"`
	}
	type doc struct {
		Version     int    `json:"version"`
		ID          string `json:"id"`
		Title       string `json:"title"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Nodes       []node `json:"nodes"`
		Edges       []edge `json:"edges"`
		Items       []item `json:"items"`
		Note        string `json:"note,omitempty"`
	}
	id := fmt.Sprintf("byok-%d", time.Now().UnixNano())
	n0 := node{ID: "n1", Title: "Overview", Description: "Local BYOK fallback map", Content: prompt, Location: ""}
	d := doc{
		Version: 1, ID: id, Title: "BYOK CodeMap", Summary: truncate(prompt, 200), Description: truncate(prompt, 200),
		Nodes: []node{n0},
		Edges: []edge{},
		Items: []item{{Title: n0.Title, Description: n0.Description, Content: n0.Content, Location: n0.Location}},
		Note:  note,
	}
	b, _ := json.Marshal(d)
	return string(b)
}

func normalizeCodeMapDoc(raw, prompt string) string {
	// 将模型输出规整为含 nodes/items 的结构，兼容 CodeMapScopeItem 风格
	s := strings.TrimSpace(raw)
	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return fallbackCodeMapJSON(prompt, "invalid json")
	}
	// ensure title
	if _, ok := doc["title"]; !ok {
		doc["title"] = "BYOK CodeMap"
	}
	if _, ok := doc["version"]; !ok {
		doc["version"] = 1
	}
	if _, ok := doc["id"]; !ok {
		doc["id"] = fmt.Sprintf("byok-%d", time.Now().UnixNano())
	}
	// items -> nodes if nodes missing
	if _, ok := doc["nodes"]; !ok {
		nodes := []any{}
		if items, ok := doc["items"].([]any); ok {
			for i, it := range items {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				nodes = append(nodes, map[string]any{
					"id":          fmt.Sprintf("n%d", i+1),
					"title":       firstStr(m, "title", "name"),
					"description": firstStr(m, "description", "summary"),
					"content":     firstStr(m, "content", "text"),
					"location":    firstStr(m, "location", "path"),
				})
			}
		}
		if len(nodes) == 0 {
			nodes = []any{map[string]any{"id": "n1", "title": "Overview", "description": "", "content": prompt, "location": ""}}
		}
		doc["nodes"] = nodes
	}
	if _, ok := doc["edges"]; !ok {
		doc["edges"] = []any{}
	}
	// mirror nodes to items for older consumers
	if _, ok := doc["items"]; !ok {
		items := []any{}
		if nodes, ok := doc["nodes"].([]any); ok {
			for _, n := range nodes {
				m, _ := n.(map[string]any)
				if m == nil {
					continue
				}
				items = append(items, map[string]any{
					"title": m["title"], "description": m["description"], "content": m["content"], "location": m["location"],
				})
			}
		}
		doc["items"] = items
	}
	if _, ok := doc["summary"]; !ok {
		doc["summary"] = truncate(prompt, 200)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return fallbackCodeMapJSON(prompt, "normalize failed")
	}
	return string(b)
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
