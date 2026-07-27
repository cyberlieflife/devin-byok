package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"devin-byok/internal/upstream/openai"
)

// Client Anthropic Messages API（/v1/messages）。
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 0}}
}

type messageReq struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Messages    []msg     `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Tools       []toolDef `json:"tools,omitempty"`
}

type msg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// Chat 非流式。
func (c *Client) Chat(ctx context.Context, baseURL, apiKey, model string, messages []openai.ChatMessage, opt openai.ChatOptions) (openai.ChatResult, error) {
	body, err := c.build(model, messages, false, opt)
	if err != nil {
		return openai.ChatResult{}, err
	}
	raw, code, err := c.do(ctx, baseURL, apiKey, body)
	if err != nil {
		return openai.ChatResult{}, err
	}
	if code >= 300 {
		return openai.ChatResult{}, openai.HumanizeHTTPError(code, string(raw))
	}
	var resp struct {
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Name  string `json:"name"`
			ID    string `json:"id"`
			Input any    `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return openai.ChatResult{}, err
	}
	if resp.Error != nil {
		return openai.ChatResult{}, fmt.Errorf("%s", resp.Error.Message)
	}
	var text strings.Builder
	var tools []openai.ToolCall
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			tools = append(tools, openai.ToolCall{
				ID:   b.ID,
				Type: "function",
			})
			// set function fields
			tools[len(tools)-1].Function.Name = b.Name
			tools[len(tools)-1].Function.Arguments = string(args)
		}
	}
	return openai.ChatResult{
		Content:   text.String(),
		ToolCalls: tools,
		Usage: openai.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

// StreamChat 流式（SSE event stream）。
func (c *Client) StreamChat(ctx context.Context, baseURL, apiKey, model string, messages []openai.ChatMessage, opt openai.ChatOptions, onDelta func(openai.StreamDelta) error) (openai.Usage, error) {
	body, err := c.build(model, messages, true, opt)
	if err != nil {
		return openai.Usage{}, err
	}
	endpoint := normalizeMessagesURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return openai.Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return openai.Usage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return openai.Usage{}, openai.HumanizeHTTPError(resp.StatusCode, string(b))
	}
	var usage openai.Usage
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message *struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(payload), &ev) != nil {
			continue
		}
		if ev.Type == "content_block_delta" && ev.Delta.Text != "" {
			if err := onDelta(openai.StreamDelta{Content: ev.Delta.Text}); err != nil {
				return usage, err
			}
		}
		if ev.Usage != nil {
			usage.PromptTokens = ev.Usage.InputTokens
			usage.CompletionTokens = ev.Usage.OutputTokens
		}
		if ev.Message != nil {
			usage.PromptTokens = ev.Message.Usage.InputTokens
			usage.CompletionTokens = ev.Message.Usage.OutputTokens
		}
	}
	return usage, sc.Err()
}

func (c *Client) build(model string, messages []openai.ChatMessage, stream bool, opt openai.ChatOptions) ([]byte, error) {
	sys, msgs := convertMessages(messages)
	maxTok := opt.MaxTokens
	if maxTok <= 0 {
		maxTok = 4096
	}
	req := messageReq{
		Model: model, MaxTokens: maxTok, System: sys, Messages: msgs, Stream: stream,
		Temperature: opt.Temperature, TopP: opt.TopP,
	}
	for _, t := range opt.Tools {
		req.Tools = append(req.Tools, toolDef{
			Name: t.Function.Name, Description: t.Function.Description, InputSchema: t.Function.Parameters,
		})
	}
	return json.Marshal(req)
}

func convertMessages(in []openai.ChatMessage) (system string, out []msg) {
	for _, m := range in {
		switch m.Role {
		case "system":
			t := openai.TextContent(m.Content)
			if system != "" {
				system += "\n\n"
			}
			system += t
		case "assistant":
			out = append(out, msg{Role: "assistant", Content: openai.TextContent(m.Content)})
		case "user":
			out = append(out, msg{Role: "user", Content: convertUserContent(m.Content)})
		case "tool":
			// Anthropic tool_result blocks — simplified as user text
			out = append(out, msg{Role: "user", Content: "[tool_result " + m.ToolCallID + "] " + openai.TextContent(m.Content)})
		}
	}
	if len(out) == 0 {
		out = []msg{{Role: "user", Content: "hello"}}
	}
	return system, out
}

func convertUserContent(content any) any {
	switch v := content.(type) {
	case string:
		return v
	case []openai.ContentPart:
		parts := make([]map[string]any, 0, len(v))
		for _, p := range v {
			if p.Type == "text" {
				parts = append(parts, map[string]any{"type": "text", "text": p.Text})
			} else if p.Type == "image_url" && p.ImageURL != nil {
				// data:image/png;base64,xxxx
				url := p.ImageURL.URL
				media := "image/png"
				data := url
				if strings.HasPrefix(url, "data:") {
					// data:mime;base64,DATA
					rest := strings.TrimPrefix(url, "data:")
					if i := strings.Index(rest, ";base64,"); i >= 0 {
						media = rest[:i]
						data = rest[i+len(";base64,"):]
					}
				}
				parts = append(parts, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": media,
						"data":       data,
					},
				})
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return parts
	default:
		return openai.TextContent(content)
	}
}

func (c *Client) do(ctx context.Context, baseURL, apiKey string, body []byte) ([]byte, int, error) {
	endpoint := normalizeMessagesURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	cli := c.http
	if cli == nil {
		cli = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func normalizeMessagesURL(base string) string {
	u := strings.TrimRight(strings.TrimSpace(base), "/")
	if u == "" {
		return "https://api.anthropic.com/v1/messages"
	}
	low := strings.ToLower(u)
	if strings.HasSuffix(low, "/v1/messages") {
		return u
	}
	if strings.HasSuffix(low, "/v1") {
		return u + "/messages"
	}
	if strings.Contains(low, "/chat/completions") {
		return strings.Replace(u, "/chat/completions", "/messages", 1)
	}
	return u + "/v1/messages"
}
