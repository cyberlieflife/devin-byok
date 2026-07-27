package localapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"devin-byok/internal/pbwire"
	"devin-byok/internal/upstream/openai"
)

// ChatMessageSource（exa.codeium_common_pb）
// 注意：SOURCE_SYSTEM(2) 在 Cascade 历史里实际表示“模型回复”，不是 OpenAI 的 system。
const (
	chatSrcUser         = 1
	chatSrcModel        = 2 // CHAT_MESSAGE_SOURCE_SYSTEM → 映射为 assistant
	chatSrcUnknown      = 3
	chatSrcTool         = 4
	chatSrcSystemPrompt = 5
)

// parsedChatRequest 解析后的 api_server GetChatMessage 请求。
type parsedChatRequest struct {
	SystemPrompt string
	Messages     []openai.ChatMessage
	Tools        []openai.Tool
	UserText     string
	ModelUID     string
	ConvID       string
	RequestID    string
	// Images 从请求中提取的 ImageData（多模态）
	Images []chatImage
}

// sessionCache 缓存同会话的 system/tools，供短请求复用。
type sessionCache struct {
	mu   sync.RWMutex
	data map[string]sessionEntry
}

type sessionEntry struct {
	System string
	Tools  []openai.Tool
}

var chatSessions = &sessionCache{data: map[string]sessionEntry{}}

func (c *sessionCache) get(id string) (sessionEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.data[id]
	return e, ok
}

func (c *sessionCache) put(id string, e sessionEntry) {
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// 简单上限，防止无限增长
	if len(c.data) > 64 {
		c.data = map[string]sessionEntry{}
	}
	c.data[id] = e
}

// parseGetChatMessageRequest 从 Connect/proto 请求体提取 system/messages/tools。
func parseGetChatMessageRequest(plain []byte) parsedChatRequest {
	out := parsedChatRequest{}
	fields := pbwire.ParseFields(plain)

	// field2: system prompt string
	// field16: cascade_id；field22: execution_id；field21: chat_model_uid
	for _, f := range fields {
		if f.Number == 2 && f.Wire == 2 {
			if s := string(f.Bytes); looksLikeText(s) {
				out.SystemPrompt = s
			}
		}
		if f.Number == 21 && f.Wire == 2 {
			out.ModelUID = string(f.Bytes)
		}
		if f.Number == 16 && f.Wire == 2 {
			out.ConvID = string(f.Bytes)
		}
		if f.Number == 22 && f.Wire == 2 && out.ConvID == "" {
			out.ConvID = string(f.Bytes)
		}
		if f.Number == 17 && f.Wire == 2 {
			out.RequestID = string(f.Bytes)
		}
	}

	// field3: chat_message_prompts
	// 1=message_id 2=source 3=prompt 6=tool_calls 7=tool_call_id 9=tool_result_is_error 11=thinking
	for _, f := range fields {
		if f.Number != 3 || f.Wire != 2 {
			continue
		}
		src := 0
		content := ""
		toolCallID := ""
		var toolCalls []openai.ToolCall
		for _, sf := range pbwire.ParseFields(f.Bytes) {
			switch {
			case sf.Number == 2 && sf.Wire == 0:
				src = int(sf.Varint)
			case sf.Number == 3 && sf.Wire == 2:
				content = string(sf.Bytes)
			case sf.Number == 7 && sf.Wire == 2:
				toolCallID = string(sf.Bytes)
			case sf.Number == 6 && sf.Wire == 2:
				if tc, ok := parseChatToolCall(sf.Bytes); ok {
					toolCalls = append(toolCalls, tc)
				}
			}
		}

		// tool 结果：SOURCE_TOOL 或带 tool_call_id
		if src == chatSrcTool || toolCallID != "" {
			if strings.TrimSpace(content) == "" && toolCallID == "" {
				continue
			}
			out.Messages = append(out.Messages, openai.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: toolCallID,
			})
			continue
		}

		// 模型历史（含 tool_calls）
		if src == chatSrcModel || len(toolCalls) > 0 {
			if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
				continue
			}
			msg := openai.ChatMessage{Role: "assistant", Content: content}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			out.Messages = append(out.Messages, msg)
			continue
		}

		// 用户 / 其它：当 user 文本
		if strings.TrimSpace(content) == "" {
			continue
		}
		// 跳过纯 system_prompt 源
		if src == chatSrcSystemPrompt {
			continue
		}
		out.Messages = append(out.Messages, openai.ChatMessage{Role: "user", Content: content})
		if ut := extractUserRequest(content); ut != "" {
			out.UserText = ut
		} else if out.UserText == "" && src == chatSrcUser {
			// 避免把 MEMORIES 之类噪声当主用户句；优先带 user_request 的
			if !strings.HasPrefix(strings.TrimSpace(content), "No MEMORIES") &&
				!strings.HasPrefix(strings.TrimSpace(content), "ENTER ANALYSIS MODE") {
				out.UserText = strings.TrimSpace(content)
			}
		}
	}

	// field10: tools {name=1, desc=2, params_json=3}
	for _, f := range fields {
		if f.Number != 10 || f.Wire != 2 {
			continue
		}
		name, desc, paramsJSON := "", "", ""
		for _, sf := range pbwire.ParseFields(f.Bytes) {
			if sf.Wire != 2 {
				continue
			}
			switch sf.Number {
			case 1:
				name = string(sf.Bytes)
			case 2:
				desc = string(sf.Bytes)
			case 3:
				paramsJSON = string(sf.Bytes)
			}
		}
		if name == "" || name == "do_not_call" {
			continue
		}
		var params any
		if paramsJSON != "" {
			_ = json.Unmarshal([]byte(paramsJSON), &params)
		}
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out.Tools = append(out.Tools, openai.Tool{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	// 会话缓存：完整 planner（有真实 tools 或长 system）写入；短请求复用
	if out.ConvID != "" {
		if out.SystemPrompt != "" || len(out.Tools) > 0 {
			e := sessionEntry{
				System: out.SystemPrompt,
				Tools:  append([]openai.Tool(nil), out.Tools...),
			}
			if old, ok := chatSessions.get(out.ConvID); ok {
				if e.System == "" {
					e.System = old.System
				}
				// 短请求常只带 do_not_call（已过滤为空）；不要用空 tools 覆盖缓存
				if len(e.Tools) == 0 {
					e.Tools = old.Tools
				}
			}
			// 仅当确有内容时写入
			if e.System != "" || len(e.Tools) > 0 {
				chatSessions.put(out.ConvID, e)
			}
		} else if e, ok := chatSessions.get(out.ConvID); ok {
			metricsCache(true)
			if out.SystemPrompt == "" {
				out.SystemPrompt = e.System
			}
			if len(out.Tools) == 0 {
				out.Tools = e.Tools
			}
		} else if out.ConvID != "" {
			metricsCache(false)
		}
	}

	if out.UserText == "" {
		if ut := extractUserRequest(out.SystemPrompt); ut != "" {
			out.UserText = ut
		} else {
			out.UserText = pickUserText(plain)
		}
	}
	out.Images = extractImagesFromProto(plain)
	return out
}

func parseChatToolCall(raw []byte) (openai.ToolCall, bool) {
	// ChatToolCall: 1=id 2=name 3=arguments_json
	var tc openai.ToolCall
	tc.Type = "function"
	for _, f := range pbwire.ParseFields(raw) {
		if f.Wire != 2 {
			continue
		}
		switch f.Number {
		case 1:
			tc.ID = string(f.Bytes)
		case 2:
			tc.Function.Name = string(f.Bytes)
		case 3:
			tc.Function.Arguments = string(f.Bytes)
		}
	}
	if tc.Function.Name == "" && tc.ID == "" {
		return openai.ToolCall{}, false
	}
	if tc.ID == "" {
		tc.ID = fmt.Sprintf("call_%s", tc.Function.Name)
	}
	return tc, true
}

func extractUserRequest(content string) string {
	const a, b = "<user_request>", "</user_request>"
	i := strings.Index(content, a)
	if i < 0 {
		return ""
	}
	rest := content[i+len(a):]
	j := strings.Index(rest, b)
	if j < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func looksLikeText(s string) bool {
	if len(s) < 8 {
		return false
	}
	// 粗判：高比例可打印/空白
	ok := 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r >= 32 {
			ok++
		}
	}
	return ok*10 >= utf8.RuneCountInString(s)*8
}

// buildOpenAIMessages 组装发给上游的 messages。
func buildOpenAIMessages(p parsedChatRequest) []openai.ChatMessage {
	msgs := make([]openai.ChatMessage, 0, len(p.Messages)+2)
	sys := strings.TrimSpace(p.SystemPrompt)
	if sys != "" {
		msgs = append(msgs, openai.ChatMessage{Role: "system", Content: sys})
	} else {
		msgs = append(msgs, openai.ChatMessage{
			Role:    "system",
			Content: "You are Cascade, a helpful coding agent. Use tools when needed to inspect and modify the codebase.",
		})
	}

	for _, m := range p.Messages {
		// 历史里不应再塞 system 角色
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, m)
	}

	// 若历史没有任何 user/tool，至少放用户句
	hasUserish := false
	for _, m := range p.Messages {
		if m.Role == "user" || m.Role == "tool" {
			hasUserish = true
			break
		}
	}
	if !hasUserish && strings.TrimSpace(p.UserText) != "" {
		msgs = append(msgs, openai.ChatMessage{Role: "user", Content: p.UserText})
	}
	msgs = attachImagesToUserMessage(msgs, p.Images)
	return msgs
}


// chatImage 对应 exa.codeium_common_pb.ImageData。
type chatImage struct {
	Base64  string
	MIME    string
	Caption string
}

// extractImagesFromProto 递归扫描 protobuf，识别 ImageData{base64,mime,caption}。
func extractImagesFromProto(body []byte) []chatImage {
	var out []chatImage
	var walk func(data []byte)
	walk = func(data []byte) {
		i := 0
		fields := map[int][]byte{}
		order := []int{}
		for i < len(data) {
			key, n := readVarintLocal(data[i:])
			if n <= 0 {
				return
			}
			i += n
			fn := int(key >> 3)
			wt := int(key & 7)
			if fn == 0 {
				return
			}
			switch wt {
			case 0:
				_, n2 := readVarintLocal(data[i:])
				if n2 <= 0 {
					return
				}
				i += n2
			case 1:
				if i+8 > len(data) {
					return
				}
				i += 8
			case 2:
				ln, n2 := readVarintLocal(data[i:])
				if n2 <= 0 {
					return
				}
				i += n2
				if int(ln) > len(data)-i {
					return
				}
				blob := data[i : i+int(ln)]
				i += int(ln)
				// 记录本层 string 字段
				fields[fn] = blob
				order = append(order, fn)
				// 递归 message
				if len(blob) > 0 && looksLikeProtoMessage(blob) {
					walk(blob)
				}
			case 5:
				if i+4 > len(data) {
					return
				}
				i += 4
			default:
				return
			}
		}
		// ImageData: 1 base64_data, 2 mime_type, 3 caption
		b64 := string(fields[1])
		mime := strings.ToLower(string(fields[2]))
		cap := string(fields[3])
		if b64 != "" && strings.HasPrefix(mime, "image/") && len(b64) > 32 {
			out = append(out, chatImage{Base64: b64, MIME: mime, Caption: cap})
		}
	}
	walk(body)
	// 去重
	seen := map[string]bool{}
	uniq := make([]chatImage, 0, len(out))
	for _, im := range out {
		k := im.MIME + ":" + im.Base64[:min(64, len(im.Base64))]
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, im)
	}
	return uniq
}

func looksLikeProtoMessage(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	// 粗判：以合法 tag 开头
	key, n := readVarintLocal(b)
	if n <= 0 || key == 0 {
		return false
	}
	fn := key >> 3
	wt := key & 7
	return fn > 0 && fn < 500 && (wt == 0 || wt == 1 || wt == 2 || wt == 5)
}

func readVarintLocal(b []byte) (uint64, int) {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// attachImagesToUserMessage 将图片挂到最后一条 user 消息（OpenAI vision 格式）。
func attachImagesToUserMessage(msgs []openai.ChatMessage, images []chatImage) []openai.ChatMessage {
	if len(images) == 0 || len(msgs) == 0 {
		return msgs
	}
	// 找最后 user
	idx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return msgs
	}
	text := openai.TextContent(msgs[idx].Content)
	parts := make([]openai.ContentPart, 0, 1+len(images))
	if strings.TrimSpace(text) != "" {
		parts = append(parts, openai.ContentPart{Type: "text", Text: text})
	}
	for _, im := range images {
		url := im.Base64
		if !strings.HasPrefix(url, "data:") {
			mt := im.MIME
			if mt == "" {
				mt = "image/png"
			}
			url = "data:" + mt + ";base64," + im.Base64
		}
		if im.Caption != "" {
			parts = append(parts, openai.ContentPart{Type: "text", Text: "[image] " + im.Caption})
		}
		parts = append(parts, openai.ContentPart{Type: "image_url", ImageURL: &openai.ImageURL{URL: url}})
	}
	msgs[idx].Content = parts
	return msgs
}
