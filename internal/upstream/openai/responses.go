package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"devin-byok/internal/config"
)

// responsesRequest OpenAI Responses API body.
type responsesRequest struct {
	Model           string              `json:"model"`
	Input           any                 `json:"input"`
	Instructions    string              `json:"instructions,omitempty"`
	Stream          bool                `json:"stream,omitempty"`
	MaxOutputTokens int                 `json:"max_output_tokens,omitempty"`
	Temperature     *float64            `json:"temperature,omitempty"`
	TopP            *float64            `json:"top_p,omitempty"`
	Reasoning       *reasoningBody      `json:"reasoning,omitempty"`
	Tools           []responsesTool     `json:"tools,omitempty"`
	ToolChoice      any                 `json:"tool_choice,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"` // function
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// NormalizeResponsesURL 将 base 规范为 .../v1/responses。
func NormalizeResponsesURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		return ""
	}
	low := strings.ToLower(u)
	if strings.HasSuffix(low, "/responses") {
		return u
	}
	if strings.HasSuffix(low, "/chat/completions") {
		u = strings.TrimSuffix(u, "/chat/completions")
		u = strings.TrimSuffix(u, "/Chat/Completions")
		low = strings.ToLower(u)
	}
	if strings.HasSuffix(low, "/v1") {
		return u + "/responses"
	}
	return u + "/v1/responses"
}

func (c *Client) resolveResponsesEndpoint(opt ChatOptions) (endpoint, apiKey string, headers map[string]string, httpClient *http.Client) {
	cfg, _, hc := c.snapshot()
	apiKey, httpClient = cfg.APIKey, hc
	headers = map[string]string{}
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	base := firstNonEmpty(opt.BaseURL, cfg.BaseURL)
	endpoint = NormalizeResponsesURL(base)
	if strings.TrimSpace(opt.APIKey) != "" {
		apiKey = opt.APIKey
	}
	return endpoint, apiKey, headers, httpClient
}

func buildResponsesRequest(model string, messages []ChatMessage, stream bool, opt ChatOptions, cfg config.UpstreamConfig) responsesRequest {
	var instructions string
	var input []responsesMessage
	for _, m := range messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		text := TextContent(m.Content)
		if role == "system" || role == "developer" {
			if instructions != "" {
				instructions += "\n\n"
			}
			instructions += text
			continue
		}
		if role == "assistant" {
			role = "assistant"
		} else if role == "tool" {
			// Responses 工具回传较复杂，降级为 user 文本
			role = "user"
			if m.ToolCallID != "" {
				text = "[tool_result " + m.ToolCallID + "] " + text
			}
		} else {
			role = "user"
		}
		// multimodal: Content 可能是 string 或 []ContentPart
		var content any = m.Content
		if content == nil || TextContent(content) == "" && text != "" {
			content = text
		}
		input = append(input, responsesMessage{Role: role, Content: content})
	}
	if len(input) == 0 {
		input = append(input, responsesMessage{Role: "user", Content: "hello"})
	}

	req := responsesRequest{
		Model:        model,
		Input:        input,
		Instructions: instructions,
		Stream:       stream,
	}
	temp := opt.Temperature
	if temp == nil {
		temp = cfg.Sampling.Temperature
	}
	topP := opt.TopP
	if topP == nil {
		topP = cfg.Sampling.TopP
	}
	req.Temperature = temp
	req.TopP = topP
	maxTok := opt.MaxTokens
	if maxTok == 0 {
		maxTok = cfg.Sampling.MaxTokens
	}
	if maxTok > 0 {
		req.MaxOutputTokens = maxTok
	}
	level := config.NormalizeThinkingLevel(opt.Thinking)
	if level == "" {
		level = config.NormalizeThinkingLevel(cfg.Thinking.Default)
	}
	if level != "" && level != "none" {
		req.Reasoning = &reasoningBody{Effort: level}
	}
	// tools: map OpenAI chat tools -> responses function tools
	for _, tl := range opt.Tools {
		if strings.ToLower(tl.Type) != "function" && tl.Type != "" {
			continue
		}
		params := map[string]any{}
		if tl.Function.Parameters != nil {
			if m, ok := tl.Function.Parameters.(map[string]any); ok {
				params = m
			}
		}
		name := tl.Function.Name
		if name == "" {
			continue
		}
		req.Tools = append(req.Tools, responsesTool{
			Type:        "function",
			Name:        name,
			Description: tl.Function.Description,
			Parameters:  params,
		})
	}
	if len(req.Tools) > 0 {
		req.ToolChoice = "auto"
	}
	return req
}

type responsesAPIResult struct {
	Output []struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		// function call
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		CallID    string `json:"call_id"`
	} `json:"output"`
	// convenience
	OutputText string `json:"output_text"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		InputTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r responsesAPIResult) toChatResult() ChatResult {
	var content strings.Builder
	var thinking strings.Builder
	var tools []ToolCall
	if r.OutputText != "" {
		content.WriteString(r.OutputText)
	}
	for _, item := range r.Output {
		switch strings.ToLower(item.Type) {
		case "message", "":
			for _, c := range item.Content {
				ct := strings.ToLower(c.Type)
				if ct == "output_text" || ct == "text" || ct == "" {
					content.WriteString(c.Text)
				} else if strings.Contains(ct, "reasoning") || strings.Contains(ct, "thinking") {
					thinking.WriteString(c.Text)
				}
			}
		case "reasoning":
			for _, c := range item.Content {
				thinking.WriteString(c.Text)
			}
		case "function_call", "custom_tool_call":
			id := item.CallID
			if id == "" {
				id = "call_" + item.Name
			}
			tc := ToolCall{ID: id, Type: "function"}
			tc.Function.Name = item.Name
			tc.Function.Arguments = item.Arguments
			tools = append(tools, tc)
		}
	}
	var usage Usage
	if r.Usage != nil {
		usage.PromptTokens = r.Usage.InputTokens
		usage.CompletionTokens = r.Usage.OutputTokens
		if r.Usage.InputTokensDetails != nil {
			usage.CachedTokens = r.Usage.InputTokensDetails.CachedTokens
		}
	}
	return ChatResult{Content: content.String(), Thinking: thinking.String(), ToolCalls: tools, Usage: usage}
}

// ChatResponses 非流式 Responses API。
func (c *Client) ChatResponses(ctx context.Context, model string, messages []ChatMessage, opt ChatOptions) (ChatResult, error) {
	cfg, _, _ := c.snapshot()
	if model == "" {
		model = cfg.Model
	}
	body := buildResponsesRequest(model, messages, false, opt, cfg)
	endpoint, apiKey, headers, httpClient := c.resolveResponsesEndpoint(opt)
	if endpoint == "" {
		return ChatResult{}, fmt.Errorf("模型未配置 base_url（Responses）")
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return ChatResult{}, HumanizeError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return ChatResult{}, HumanizeHTTPError(resp.StatusCode, string(data))
	}
	var parsed responsesAPIResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ChatResult{}, fmt.Errorf("Responses 返回无法解析: %v ; body=%s", err, truncate(string(data), 300))
	}
	if parsed.Error != nil {
		return ChatResult{}, fmt.Errorf("上游错误: %s", parsed.Error.Message)
	}
	return parsed.toChatResult(), nil
}

// StreamChatResponses 流式 Responses API。
func (c *Client) StreamChatResponses(ctx context.Context, model string, messages []ChatMessage, opt ChatOptions, onDelta func(StreamDelta) error) (Usage, error) {
	cfg, _, _ := c.snapshot()
	if model == "" {
		model = cfg.Model
	}
	body := buildResponsesRequest(model, messages, true, opt, cfg)
	endpoint, apiKey, headers, httpClient := c.resolveResponsesEndpoint(opt)
	if endpoint == "" {
		return Usage{}, fmt.Errorf("模型未配置 base_url（Responses）")
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Usage{}, HumanizeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return Usage{}, HumanizeHTTPError(resp.StatusCode, string(data))
	}

	br := bufio.NewReaderSize(resp.Body, 64*1024)
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	prefix, _ := br.Peek(32)
	prefixTrim := bytes.TrimSpace(prefix)
	isSSE := strings.Contains(ct, "text/event-stream") || bytes.HasPrefix(prefixTrim, []byte("data:")) || bytes.HasPrefix(prefixTrim, []byte("event:"))
	if !isSSE {
		data, err := io.ReadAll(br)
		if err != nil {
			return Usage{}, HumanizeError(err)
		}
		var parsed responsesAPIResult
		if err := json.Unmarshal(data, &parsed); err != nil {
			return Usage{}, fmt.Errorf("Responses non-SSE parse failed: %v ; body=%s", err, truncate(string(data), 300))
		}
		if parsed.Error != nil {
			return Usage{}, fmt.Errorf("上游错误: %s", parsed.Error.Message)
		}
		res := parsed.toChatResult()
		if res.Content != "" || res.Thinking != "" || len(res.ToolCalls) > 0 {
			if err := onDelta(StreamDelta{Content: res.Content, Thinking: res.Thinking, ToolCalls: res.ToolCalls}); err != nil {
				return Usage{}, err
			}
		}
		return res.Usage, nil
	}

	sc := bufio.NewScanner(br)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	var finalUsage Usage
	gotAny := false
	var toolBuf []ToolCall
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "response.output_text.delta", "response.content_part.delta":
			delta, _ := ev["delta"].(string)
			if delta == "" {
				if d, ok := ev["delta"].(map[string]any); ok {
					if t, ok := d["text"].(string); ok {
						delta = t
					}
				}
			}
			if delta != "" {
				gotAny = true
				if err := onDelta(StreamDelta{Content: delta}); err != nil {
					return Usage{}, err
				}
			}
		case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			delta, _ := ev["delta"].(string)
			if delta != "" {
				gotAny = true
				if err := onDelta(StreamDelta{Thinking: delta}); err != nil {
					return Usage{}, err
				}
			}
		case "response.function_call_arguments.delta":
			// accumulate is complex; ignore partial, final comes in output_item.done
		case "response.output_item.done":
			// try parse function call
			if item, ok := ev["item"].(map[string]any); ok {
				it, _ := item["type"].(string)
				if it == "function_call" {
					name, _ := item["name"].(string)
					args, _ := item["arguments"].(string)
					id, _ := item["call_id"].(string)
					if id == "" {
						id, _ = item["id"].(string)
					}
					if name != "" {
						tc := ToolCall{ID: id, Type: "function"}
						tc.Function.Name = name
						tc.Function.Arguments = args
						toolBuf = append(toolBuf, tc)
						gotAny = true
						if err := onDelta(StreamDelta{ToolCalls: []ToolCall{tc}}); err != nil {
							return Usage{}, err
						}
					}
				}
			}
		case "response.completed":
			if respObj, ok := ev["response"].(map[string]any); ok {
				if u, ok := respObj["usage"].(map[string]any); ok {
					if v, ok := u["input_tokens"].(float64); ok {
						finalUsage.PromptTokens = int(v)
					}
					if v, ok := u["output_tokens"].(float64); ok {
						finalUsage.CompletionTokens = int(v)
					}
					if d, ok := u["input_tokens_details"].(map[string]any); ok {
						if v, ok := d["cached_tokens"].(float64); ok {
							finalUsage.CachedTokens = int(v)
						}
					}
				}
			}
		case "error", "response.failed":
			msg, _ := ev["message"].(string)
			if msg == "" {
				if e, ok := ev["error"].(map[string]any); ok {
					msg, _ = e["message"].(string)
				}
			}
			if msg == "" {
				msg = payload
			}
			return Usage{}, fmt.Errorf("上游错误: %s", truncate(msg, 200))
		}
	}
	if err := sc.Err(); err != nil {
		return Usage{}, HumanizeError(err)
	}
	if !gotAny && finalUsage.PromptTokens == 0 && finalUsage.CompletionTokens == 0 {
		return Usage{}, fmt.Errorf("Responses SSE empty stream")
	}
	_ = toolBuf
	return finalUsage, nil
}
