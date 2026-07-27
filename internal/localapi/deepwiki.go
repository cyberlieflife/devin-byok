package localapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"devin-byok/internal/pbwire"
	"devin-byok/internal/upstream/openai"
	"devin-byok/internal/logx"
)

// deepWikiReq 解析自 exa.chat_pb / api_server GetDeepWikiRequest。
type deepWikiReq struct {
	RequestType       int32
	SymbolName        string
	SymbolURI         string
	Context           string
	SymbolType        int32
	Language          string
	GenerateFollowups bool
	RequestID         string
}

const (
	deepWikiRequestSummary = 1
	deepWikiRequestArticle = 2

	deepWikiModelPremium    = 4
)

// parseGetDeepWikiRequest 从解压后的 proto 提取 DeepWiki 字段。
func parseGetDeepWikiRequest(plain []byte) deepWikiReq {
	var out deepWikiReq
	if len(plain) == 0 {
		return out
	}
	// 顶层：2 request_type, 3 symbol_name, 4 symbol_uri, 5 context, 6 symbol_type, 7 language, 8 generate_followups
	walkProto(plain, func(fn int, wt int, v []byte, num uint64) {
		switch fn {
		case 1: // metadata
			walkProto(v, func(mfn int, mwt int, mv []byte, mnum uint64) {
				if mfn == 10 && mwt == 2 && out.RequestID == "" {
					out.RequestID = string(mv)
				}
			})
		case 2:
			if wt == 0 {
				out.RequestType = int32(num)
			}
		case 3:
			if wt == 2 {
				out.SymbolName = string(v)
			}
		case 4:
			if wt == 2 {
				out.SymbolURI = string(v)
			}
		case 5:
			if wt == 2 {
				out.Context = string(v)
			}
		case 6:
			if wt == 0 {
				out.SymbolType = int32(num)
			}
		case 7:
			if wt == 2 {
				out.Language = string(v)
			}
		case 8:
			if wt == 0 {
				out.GenerateFollowups = num != 0
			}
		}
	})
	if out.RequestID == "" {
		if ids := extractUUIDs(plain); len(ids) > 0 {
			out.RequestID = ids[0]
		}
	}
	if out.RequestID == "" {
		out.RequestID = fmt.Sprintf("deepwiki-%d", time.Now().UnixNano())
	}
	return out
}

// walkProto 轻量 proto3 字段遍历（仅用于请求解析）。
func walkProto(data []byte, fn func(field int, wt int, bytesVal []byte, varint uint64)) {
	i := 0
	for i < len(data) {
		key, n := readVarint(data[i:])
		if n <= 0 {
			return
		}
		i += n
		field := int(key >> 3)
		wt := int(key & 7)
		if field == 0 {
			return
		}
		switch wt {
		case 0:
			v, n2 := readVarint(data[i:])
			if n2 <= 0 {
				return
			}
			i += n2
			fn(field, wt, nil, v)
		case 1:
			if i+8 > len(data) {
				return
			}
			fn(field, wt, data[i:i+8], 0)
			i += 8
		case 2:
			ln, n2 := readVarint(data[i:])
			if n2 <= 0 {
				return
			}
			i += n2
			if ln > uint64(len(data)-i) {
				return
			}
			blob := data[i : i+int(ln)]
			i += int(ln)
			fn(field, wt, blob, 0)
		case 5:
			if i+4 > len(data) {
				return
			}
			fn(field, wt, data[i:i+4], 0)
			i += 4
		default:
			return
		}
	}
}

func readVarint(b []byte) (uint64, int) {
	var x uint64
	var s uint
	for i := 0; i < len(b) && i < 10; i++ {
		c := b[i]
		x |= uint64(c&0x7f) << s
		if c&0x80 == 0 {
			return x, i + 1
		}
		s += 7
	}
	return 0, 0
}

func buildDeepWikiPrompt(req deepWikiReq) (system, user string) {
	mode := "concise summary"
	switch req.RequestType {
	case deepWikiRequestArticle:
		mode = "detailed technical article"
	case deepWikiRequestSummary:
		mode = "concise summary"
	}
	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = "English"
	}
	system = "You are DeepWiki, an in-editor code documentation assistant. " +
		"Write clear Markdown about the requested symbol. " +
		"Use headings, bullet lists, and short code snippets when helpful. " +
		"Do not invent APIs that are not supported by the provided context. " +
		"Respond in " + lang + "."
	if req.GenerateFollowups {
		system += " After the main article, append a final line exactly like: FOLLOWUPS: q1 | q2 | q3"
	}
	var b strings.Builder
	b.WriteString("Mode: ")
	b.WriteString(mode)
	b.WriteString("\nSymbol: ")
	b.WriteString(req.SymbolName)
	if req.SymbolURI != "" {
		b.WriteString("\nURI: ")
		b.WriteString(req.SymbolURI)
	}
	if req.SymbolType != 0 {
		b.WriteString(fmt.Sprintf("\nSymbolType: %d", req.SymbolType))
	}
	if strings.TrimSpace(req.Context) != "" {
		b.WriteString("\n\nContext:\n")
		ctx := req.Context
		if len(ctx) > 60000 {
			ctx = ctx[:60000]
		}
		b.WriteString(ctx)
	} else {
		b.WriteString("\n\n(No extra file context was provided.)")
	}
	return system, b.String()
}

func splitDeepWikiFollowups(text string) (body, followups string) {
	body = strings.TrimSpace(text)
	idx := strings.LastIndex(strings.ToUpper(body), "FOLLOWUPS:")
	if idx < 0 {
		return body, ""
	}
	followups = strings.TrimSpace(body[idx+len("FOLLOWUPS:"):])
	body = strings.TrimSpace(body[:idx])
	return body, followups
}

// buildGetDeepWikiDelta 构造 api_server_pb.GetDeepWikiResponse 帧。
// 抓包/客户端错误证明 LS 反序列化的是 *api_server_pb.GetDeepWikiResponse，不是 language_server 版本：
//   1 response   = api_server GetChatMessageResponse（delta_text / stop_reason）
//   2 model_type = DeepWikiModelType
//   3 is_followup = bool
// 误用 RawChatMessage 包装会导致 "string field contains invalid UTF-8"。
func buildGetDeepWikiDelta(messageID, text string, inProgress, isError, isFollowup bool) []byte {
	var chat []byte
	if isError {
		chat = buildGetChatMessageErrorDelta(messageID, text)
	} else {
		chat = buildGetChatMessageDelta(messageID, text, "", inProgress)
	}
	var b []byte
	b = pbwire.AppendMessage(b, 1, chat)
	b = pbwire.AppendEnum(b, 2, deepWikiModelPremium)
	if isFollowup {
		b = pbwire.AppendBool(b, 3, true)
	}
	return b
}

func (s *Server) handleGetDeepWiki(w http.ResponseWriter, r *http.Request, method string, body, raw []byte) {
	cfg := s.GetConfig()
	if !cfg.Features.EnableDeepWiki {
		s.writeDeepWikiStream(w, func(write func([]byte) bool) {
			_ = write(buildGetDeepWikiDelta("deepwiki-disabled", "DeepWiki is disabled in devin-byok config.", false, true, false))
		})
		return
	}

	plain := body
	if len(plain) == 0 || !looksLikeDeepWiki(plain) {
		if p := decodeConnectPayloads(raw); len(p) > 0 {
			plain = p
		}
	}
	req := parseGetDeepWikiRequest(plain)
	msgID := "deepwiki-" + req.RequestID
	if len(msgID) > 64 {
		msgID = msgID[:64]
	}

	uiModel := cfg.FeatureModelID("deepwiki")
	prov, okProv := cfg.ResolveProvider(uiModel)
	if !okProv {
		s.writeDeepWikiStream(w, func(write func([]byte) bool) {
			_ = write(buildGetDeepWikiDelta(msgID, "未配置可用模型供应商，请在 GUI「模型」页配置 base_url / api_key / upstream_model。", false, true, false))
		})
		return
	}
	model := prov.UpstreamModel
	system, user := buildDeepWikiPrompt(req)
	msgs := []openai.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	maxTok := cfg.Upstream.Sampling.MaxTokens
	if m, ok := cfg.FindModel(uiModel); ok && m.MaxTokens > 0 {
		maxTok = m.MaxTokens
	}
	if maxTok <= 0 {
		maxTok = 4096
	}
	chatOpt := openai.ChatOptions{
		Thinking:      cfg.ResolveThinking(uiModel),
		ThinkingParam: cfg.Upstream.Thinking.Param,
		Temperature:   cfg.Upstream.Sampling.Temperature,
		MaxTokens:     maxTok,
		TopP:          cfg.Upstream.Sampling.TopP,
		BaseURL:       prov.BaseURL,
		APIKey:        prov.APIKey,
	}
	if cfg.PromptCacheEnabled() {
		key := strings.TrimSpace(cfg.Cache.PromptCacheKey)
		if key == "" {
			key = "devin-byok:deepwiki:" + req.RequestID
		}
		chatOpt.PromptCacheKey = key
	}

	logx.Infof("deepwiki symbol=%q uri=%q type=%d lang=%q followups=%v model=%s upstream=%s base=%s plain=%d",
		req.SymbolName, truncate(req.SymbolURI, 80), req.RequestType, req.Language, req.GenerateFollowups, uiModel, model, truncate(prov.BaseURL, 60), len(plain))
	metricsAddLog("info", fmt.Sprintf("deepwiki %s model=%s", truncate(req.SymbolName, 40), uiModel))

	timeoutSec := cfg.ResolveChatTimeoutSec(false)
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	upCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	s.writeDeepWikiStream(w, func(write func([]byte) bool) {
		// 首帧 incomplete，防止客户端过早结束
		if !write(buildGetDeepWikiDelta(msgID, "", true, false, false)) {
			cancel()
			return
		}

		type result struct {
			text  string
			usage openai.Usage
			err   error
		}
		done := make(chan result, 1)

		if cfg.Features.EnableStream {
			go func() {
				var content strings.Builder
				usage, err := s.chatStream(upCtx, prov, model, msgs, chatOpt, func(d openai.StreamDelta) error {
					if d.Content == "" {
						return nil
					}
					content.WriteString(d.Content)
					if !write(buildGetDeepWikiDelta(msgID, d.Content, true, false, false)) {
						return context.Canceled
					}
					return nil
				})
				done <- result{text: content.String(), usage: usage, err: err}
			}()
		} else {
			go func() {
				res, err := s.chatOnce(upCtx, prov, model, msgs, chatOpt)
				done <- result{text: res.Content, usage: res.Usage, err: err}
			}()
		}

		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()
		var res result
		waiting := true
		for waiting {
			select {
			case res = <-done:
				waiting = false
			case <-ticker.C:
				if !write(buildGetDeepWikiDelta(msgID, "", true, false, false)) {
					cancel()
					return
				}
			case <-r.Context().Done():
				cancel()
				_ = write(buildGetDeepWikiDelta(msgID, "DeepWiki canceled", false, true, false))
				return
			}
		}

		if res.err != nil {
			logx.Errorf("deepwiki upstream: %v", res.err)
			metricsReqFail(uiModel)
			metricsFeatureFail("deepwiki", uiModel)
			metricsAddLog("error", "deepwiki fail: "+res.err.Error())
			_ = write(buildGetDeepWikiDelta(msgID, humanizeChatError(res.err), false, true, false))
			return
		}

		text := strings.TrimSpace(res.text)
		if text == "" {
			text = "(empty DeepWiki response)"
		}
		bodyText, followups := splitDeepWikiFollowups(text)
		if !cfg.Features.EnableStream {
			// 非流式：整段正文
			_ = write(buildGetDeepWikiDelta(msgID, bodyText, true, false, false))
		} else if followups != "" {
			// 流式过程中可能已把 FOLLOWUPS 行吐出；终帧不再重复正文
		}
		// 完成帧：stop_reason=完成
		_ = write(buildGetDeepWikiDelta(msgID, "", false, false, false))
		if followups != "" {
			// api_server 无 followup_questions 字段；用 is_followup + delta_text 附带建议问题
			_ = write(buildGetDeepWikiDelta(msgID, followups, false, false, true))
		}
		if res.usage.PromptTokens > 0 || res.usage.EffectiveCached() > 0 {
			metricsAddPromptUsage(res.usage.PromptTokens, res.usage.EffectiveCached(), res.usage.CacheWriteTokens, res.usage.CompletionTokens)
		}
		metricsReqOK(uiModel, estimateTokens(user), estimateTokens(bodyText), 0)
		metricsFeatureOK("deepwiki", uiModel, "")
	})
}

func (s *Server) writeDeepWikiStream(w http.ResponseWriter, body func(write func([]byte) bool)) {
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
	body(write)
	mu.Lock()
	_, _ = w.Write(pbwire.ConnectEndStream())
	mu.Unlock()
	if flusher != nil {
		flusher.Flush()
	}
}

func looksLikeDeepWiki(plain []byte) bool {
	s := string(plain)
	return strings.Contains(s, "=== File Context") || strings.Contains(s, "file://") || strings.Contains(s, "vscode-userdata:")
}
