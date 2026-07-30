package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"devin-byok/internal/config"
)

// Client 调用 OpenAI 兼容聊天接口。
type Client struct {
	mu         sync.RWMutex
	httpClient *http.Client
	cfg        config.UpstreamConfig
	endpoint   string
}

// ContentPart OpenAI 多模态 content 项（text / image_url）。
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL 图片引用（data URL 或 https）。
type ImageURL struct {
	URL string `json:"url"`
}

// ChatMessage OpenAI chat message；Content 可为 string 或 []ContentPart。
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// TextContent 取纯文本（多模态时拼接 text parts）。
func TextContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []ContentPart:
		var b strings.Builder
		for _, p := range v {
			if p.Type == "text" && p.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(p.Text)
			}
		}
		return b.String()
	default:
		return ""
	}
}

type chatRequest struct {
	Model           string         `json:"model"`
	Messages        []ChatMessage  `json:"messages"`
	Stream          bool           `json:"stream"`
	Temperature     *float64       `json:"temperature,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	TopP            *float64       `json:"top_p,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Reasoning       *reasoningBody `json:"reasoning,omitempty"`
	Tools           []Tool         `json:"tools,omitempty"`
	ToolChoice      any            `json:"tool_choice,omitempty"`
	// PromptCacheKey 对齐 cursor-byok：OpenAI 兼容 prompt 缓存键
	PromptCacheKey string `json:"prompt_cache_key,omitempty"`
	// StreamOptions 让流式末帧带回 usage（含 cached_tokens）
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type reasoningBody struct {
	Effort string `json:"effort,omitempty"`
}

// Tool OpenAI 兼容 tools 项（B5 透传用）。
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall 上游返回的工具调用。
// ToolCall 上游返回的工具调用。
type ToolCall struct {
	// Index 流式增量中的 OpenAI index（可选）。
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MergeToolCallDeltas 按 index 合并流式 tool_calls 增量。
func MergeToolCallDeltas(dst []ToolCall, deltas []ToolCall) []ToolCall {
	if len(deltas) == 0 {
		return dst
	}
	// ? map ? index …
	type slot struct {
		tc  ToolCall
		idx int
	}
	byIdx := map[int]int{} // index -> pos in out
	out := append([]ToolCall(nil), dst...)
	for i := range out {
		k := i
		if out[i].Index != nil {
			k = *out[i].Index
		}
		byIdx[k] = i
	}
	nextFree := len(out)
	for _, d := range deltas {
		k := nextFree
		if d.Index != nil {
			k = *d.Index
		} else if d.ID != "" {
			// ? id ??
			found := false
			for i := range out {
				if out[i].ID == d.ID {
					k = i
					// … key …
					byIdx[k] = i
					found = true
					// merge into out[i]
					if d.Type != "" {
						out[i].Type = d.Type
					}
					if d.Function.Name != "" {
						out[i].Function.Name = d.Function.Name
					}
					out[i].Function.Arguments += d.Function.Arguments
					break
				}
			}
			if found {
				continue
			}
		}
		if pos, ok := byIdx[k]; ok {
			if d.ID != "" {
				out[pos].ID = d.ID
			}
			if d.Type != "" {
				out[pos].Type = d.Type
			}
			if d.Function.Name != "" {
				out[pos].Function.Name = d.Function.Name
			}
			out[pos].Function.Arguments += d.Function.Arguments
			if d.Index != nil {
				out[pos].Index = d.Index
			}
			continue
		}
		tc := d
		if tc.Type == "" {
			tc.Type = "function"
		}
		byIdx[k] = len(out)
		out = append(out, tc)
		if d.Index == nil {
			nextFree = len(out)
		}
	}
	return out
}

// ChatOptions 单次请求可选项。
type ChatOptions struct {
	Thinking       string
	ThinkingParam  string
	Temperature    *float64
	MaxTokens      int
	TopP           *float64
	Tools          []Tool
	// ToolChoice OpenAI tool_choice：auto|required|none|或指定函数；空则有 tools 时默认 auto
	ToolChoice     any
	PromptCacheKey string
	// 每模型供应商覆盖（cursor-byok 模式）
	BaseURL string
	APIKey  string
	// HTTPTimeout 单次请求总超时；0 表示仅依赖 context（适合长工具/流式）
	HTTPTimeout time.Duration
}

// Usage 上游 token/缓存统计（cursor-byok 同款口径）。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	// CachedTokens OpenAI: prompt_tokens_details.cached_tokens
	CachedTokens int `json:"cached_tokens"`
	// CacheReadTokens / CacheWriteTokens 部分中转/Anthropic 风格字段
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

func (u Usage) EffectiveCached() int {
	if u.CachedTokens > 0 {
		return u.CachedTokens
	}
	return u.CacheReadTokens
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string     `json:"content"`
			ReasoningContent string     `json:"reasoning_content"`
			Reasoning        string     `json:"reasoning"`
			Thinking         string     `json:"thinking"`
			ToolCalls        []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *usageWire `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

type usageWire struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	// OpenAI style
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	// alternate fields used by some gateways
	CacheReadTokens      int `json:"cache_read_tokens"`
	CacheWriteTokens     int `json:"cache_write_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
	CacheCreationTokens  int `json:"cache_creation_input_tokens"`
}

func (u *usageWire) ToUsage() Usage {
	if u == nil {
		return Usage{}
	}
	out := Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	}
	if out.CacheReadTokens == 0 {
		out.CacheReadTokens = u.CacheReadInputTokens
	}
	if out.CacheWriteTokens == 0 {
		out.CacheWriteTokens = u.CacheCreationTokens
	}
	if u.PromptTokensDetails != nil {
		out.CachedTokens = u.PromptTokensDetails.CachedTokens
	}
	if out.CachedTokens == 0 {
		out.CachedTokens = out.CacheReadTokens
	}
	return out
}

// ChatResult 上游完整回复。
type ChatResult struct {
	Content   string
	Thinking  string
	ToolCalls []ToolCall
	Usage     Usage
}

// StreamDelta 流式增量。
type StreamDelta struct {
	Content   string
	Thinking  string
	ToolCalls []ToolCall
	Usage     *Usage
}

func New(cfg config.UpstreamConfig) *Client {
	// Client.Timeout=0：总超时交给 context / ChatOptions.HTTPTimeout，避免 tools 长请求被 120s 误杀
	return &Client{
		httpClient: newHTTPClient(0),
		cfg:        cfg,
		endpoint:   config.NormalizeChatCompletionsURL(cfg.BaseURL),
	}
}

// Update 热更新上游配置。
func (c *Client) Update(cfg config.UpstreamConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
	c.endpoint = config.NormalizeChatCompletionsURL(cfg.BaseURL)
	c.httpClient = newHTTPClient(0)
}

func (c *Client) snapshot() (config.UpstreamConfig, string, *http.Client) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg, c.endpoint, c.httpClient
}

func (c *Client) Endpoint() string {
	_, ep, _ := c.snapshot()
	return ep
}

func (c *Client) resolveEndpointKey(opt ChatOptions) (endpoint, apiKey string, headers map[string]string, httpClient *http.Client) {
	cfg, ep, _ := c.snapshot()
	endpoint, apiKey = ep, cfg.APIKey
	httpClient = c.clientFor(opt)
	headers = map[string]string{}
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	if strings.TrimSpace(opt.BaseURL) != "" {
		endpoint = config.NormalizeChatCompletionsURL(opt.BaseURL)
	}
	if strings.TrimSpace(opt.APIKey) != "" {
		apiKey = opt.APIKey
	}
	return endpoint, apiKey, headers, httpClient
}

func newHTTPClient(total time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// 不设 ResponseHeaderTimeout：非流式+工具时模型可能长时间才返回首包
	}
	return &http.Client{Timeout: total, Transport: tr}
}

// clientFor 按本次选项构造 Client；HTTPTimeout>0 时限制总时长，否则仅靠 ctx。
func (c *Client) clientFor(opt ChatOptions) *http.Client {
	if opt.HTTPTimeout > 0 {
		return newHTTPClient(opt.HTTPTimeout)
	}
	_, _, hc := c.snapshot()
	if hc != nil {
		return hc
	}
	return newHTTPClient(0)
}

func (c *Client) buildChatRequest(model string, messages []ChatMessage, stream bool, opt ChatOptions) chatRequest {
	cfg, _, _ := c.snapshot()
	req := chatRequest{Model: model, Messages: messages, Stream: stream}

	// sampling：单次 opt 优先，否则用配置
	temp := opt.Temperature
	if temp == nil {
		temp = cfg.Sampling.Temperature
	}
	topP := opt.TopP
	if topP == nil {
		topP = cfg.Sampling.TopP
	}
	maxTok := opt.MaxTokens
	if maxTok == 0 {
		maxTok = cfg.Sampling.MaxTokens
	}
	req.Temperature = temp
	req.TopP = topP
	if maxTok > 0 {
		req.MaxTokens = maxTok
	}

	param := strings.TrimSpace(opt.ThinkingParam)
	if param == "" {
		param = strings.TrimSpace(cfg.Thinking.Param)
	}
	if param == "" {
		param = "reasoning_effort"
	}
	level := config.NormalizeThinkingLevel(opt.Thinking)
	if level == "" {
		level = config.NormalizeThinkingLevel(cfg.Thinking.Default)
	}
	if level != "" && level != "none" && !strings.EqualFold(param, "none") {
		switch strings.ToLower(param) {
		case "reasoning.effort", "reasoning":
			req.Reasoning = &reasoningBody{Effort: level}
		default:
			req.ReasoningEffort = level
		}
	}
	if len(opt.Tools) > 0 {
		req.Tools = opt.Tools
		if opt.ToolChoice != nil {
			req.ToolChoice = opt.ToolChoice
		} else {
			req.ToolChoice = "auto"
		}
	}
	if key := strings.TrimSpace(opt.PromptCacheKey); key != "" {
		req.PromptCacheKey = key
	}
	if stream {
		req.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return req
}

func isRetryableStatusCode(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 504
}

func doWithRetry(ctx context.Context, fn func() (*http.Response, error)) (*http.Response, error) {
	const maxRetries = 4
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(3+attempt) * time.Second
			select {
			case <-ctx.Done():
				if lastResp != nil {
					return lastResp, ctx.Err()
				}
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := fn()
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}

		if isRetryableStatusCode(resp.StatusCode) && attempt < maxRetries {
			resp.Body.Close()
			lastResp = nil
			continue
		}

		return resp, nil
	}

	return lastResp, lastErr
}
func (c *Client) doJSON(ctx context.Context, body any) ([]byte, int, error) {
	cfg, endpoint, httpClient := c.snapshot()
	if endpoint == "" {
		return nil, 0, fmt.Errorf("upstream.base_url 为空")
	}
	raw, _ := json.Marshal(body)
	resp, err := doWithRetry(ctx, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}
		return httpClient.Do(req)
	})
	if err != nil {
		return nil, 0, HumanizeError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// Ping 验证上游。
func (c *Client) Ping(ctx context.Context) (string, error) {
	cfg, _, _ := c.snapshot()
	if cfg.APIKey == "" || strings.HasPrefix(cfg.APIKey, "sk-xxx") {
		return "", fmt.Errorf("请先在 config.yaml 填写有效 upstream.api_key")
	}
	body := c.buildChatRequest(cfg.Model, []ChatMessage{{Role: "user", Content: "ping"}}, false, ChatOptions{})
	data, code, err := c.doJSON(ctx, body)
	if err != nil {
		return "", err
	}
	if code >= 300 {
		return "", HumanizeHTTPError(code, string(data))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("上游返回无法解析: %v", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("上游错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("上游无 choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

// Chat 非流式。
func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage, opt ChatOptions) (ChatResult, error) {
	var zero ChatResult
	if model == "" {
		cfg, _, _ := c.snapshot()
		model = cfg.Model
	}
	body := c.buildChatRequest(model, messages, false, opt)
	endpoint, apiKey, headers, httpClient := c.resolveEndpointKey(opt)
	if endpoint == "" {
		return zero, fmt.Errorf("模型未配置 base_url（请在 Family 供应商中填写）")
	}
	raw, _ := json.Marshal(body)
	resp, err := doWithRetry(ctx, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return httpClient.Do(req)
	})
	if err != nil {
		return zero, HumanizeError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	code := resp.StatusCode
	if code >= 300 {
		return zero, HumanizeHTTPError(code, string(data))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return zero, fmt.Errorf("上游返回无法解析: %v ; body=%s", err, truncate(string(data), 300))
	}
	if parsed.Error != nil {
		return zero, fmt.Errorf("上游错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return zero, fmt.Errorf("上游无 choices")
	}
	msg := parsed.Choices[0].Message
	thinking := firstNonEmpty(msg.ReasoningContent, msg.Reasoning, msg.Thinking)
	return ChatResult{Content: msg.Content, Thinking: thinking, ToolCalls: msg.ToolCalls, Usage: parsed.Usage.ToUsage()}, nil
}

// StreamChat 流式。
func (c *Client) StreamChat(ctx context.Context, model string, messages []ChatMessage, opt ChatOptions, onDelta func(StreamDelta) error) (Usage, error) {
	cfg, _, _ := c.snapshot()
	if model == "" {
		model = cfg.Model
	}
	body := c.buildChatRequest(model, messages, true, opt)
	endpoint, apiKey, headers, httpClient := c.resolveEndpointKey(opt)
	if endpoint == "" {
		return Usage{}, fmt.Errorf("模型未配置 base_url（请在 Family 供应商中填写）")
	}
	raw, _ := json.Marshal(body)
	resp, err := doWithRetry(ctx, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "text/event-stream")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return httpClient.Do(req)
	})
	if err != nil {
		return Usage{}, HumanizeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return Usage{}, HumanizeHTTPError(resp.StatusCode, string(data))
	}

	// 上游若未按 SSE 返回（仍给 JSON），旧逻辑会一直扫流卡住且无日志。
	br := bufio.NewReaderSize(resp.Body, 64*1024)
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	prefix, _ := br.Peek(32)
	prefixTrim := bytes.TrimSpace(prefix)
	isSSE := strings.Contains(ct, "text/event-stream") || bytes.HasPrefix(prefixTrim, []byte("data:"))
	if !isSSE {
		data, err := io.ReadAll(br)
		if err != nil {
			return Usage{}, HumanizeError(err)
		}
		var parsed chatResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return Usage{}, fmt.Errorf("upstream non-SSE body parse failed: %v ; body=%s", err, truncate(string(data), 300))
		}
		if parsed.Error != nil {
			return Usage{}, fmt.Errorf("upstream error: %s", parsed.Error.Message)
		}
		if len(parsed.Choices) == 0 {
			return Usage{}, fmt.Errorf("upstream empty choices (non-SSE)")
		}
		msg := parsed.Choices[0].Message
		thinking := firstNonEmpty(msg.ReasoningContent, msg.Reasoning, msg.Thinking)
		if msg.Content != "" || thinking != "" || len(msg.ToolCalls) > 0 {
			if err := onDelta(StreamDelta{Content: msg.Content, Thinking: thinking, ToolCalls: msg.ToolCalls}); err != nil {
				return Usage{}, err
			}
		}
		if parsed.Usage != nil {
			return parsed.Usage.ToUsage(), nil
		}
		return Usage{}, nil
	}

	sc := bufio.NewScanner(br)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	var finalUsage Usage
	gotAny := false
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string     `json:"content"`
					ReasoningContent string     `json:"reasoning_content"`
					Reasoning        string     `json:"reasoning"`
					Thinking         string     `json:"thinking"`
					ToolCalls        []ToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *usageWire `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return Usage{}, fmt.Errorf("upstream error: %s", chunk.Error.Message)
		}
		if chunk.Usage != nil {
			finalUsage = chunk.Usage.ToUsage()
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		thinking := firstNonEmpty(d.ReasoningContent, d.Reasoning, d.Thinking)
		if d.Content == "" && thinking == "" && len(d.ToolCalls) == 0 {
			continue
		}
		gotAny = true
		if err := onDelta(StreamDelta{Content: d.Content, Thinking: thinking, ToolCalls: d.ToolCalls}); err != nil {
			return Usage{}, err
		}
	}
	if err := sc.Err(); err != nil {
		return Usage{}, HumanizeError(err)
	}
	if !gotAny && finalUsage.PromptTokens == 0 && finalUsage.CompletionTokens == 0 {
		// SSE 扫完却无任何 delta：常见于代理返回畸形流
		return Usage{}, fmt.Errorf("upstream SSE empty stream (no content deltas)")
	}
	return finalUsage, nil
}

// HumanizeError 将网络错误转成中文提示。
func HumanizeError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "context deadline exceeded") || strings.Contains(low, "timeout"):
		return fmt.Errorf("上游超时：模型响应太慢或网络不通，请检查 upstream.base_url / timeout_sec")
	case strings.Contains(low, "connection refused"):
		return fmt.Errorf("无法连接上游：%s（请确认本地模型服务已启动）", extractHost(msg))
	case strings.Contains(low, "no such host"):
		return fmt.Errorf("上游域名无法解析，请检查 base_url")
	case strings.Contains(low, "tls") || strings.Contains(low, "x509"):
		return fmt.Errorf("上游 TLS/证书错误：%v", err)
	default:
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return fmt.Errorf("上游超时：%v", err)
		}
		return fmt.Errorf("上游网络错误：%v", err)
	}
}

// HumanizeHTTPError 将 HTTP 状态转成中文提示。
func HumanizeHTTPError(code int, body string) error {
	b := strings.ToLower(body)
	switch code {
	case 401, 403:
		return fmt.Errorf("上游鉴权失败(%d)：请检查 api_key 是否正确/是否有权限", code)
	case 404:
		return fmt.Errorf("上游接口或模型不存在(404)：请检查 base_url 与 model 名")
	case 429:
		return fmt.Errorf("上游限流(429)：请求过快或额度用尽，请稍后重试")
	case 500, 502, 503, 504:
		return fmt.Errorf("上游服务异常(%d)：%s", code, truncate(body, 160))
	default:
		if strings.Contains(b, "model") && (strings.Contains(b, "not found") || strings.Contains(b, "does not exist")) {
			return fmt.Errorf("模型不存在：请检查 config.yaml 的 model / upstream_model")
		}
		if strings.Contains(b, "quota") || strings.Contains(b, "billing") || strings.Contains(b, "balance") {
			return fmt.Errorf("上游额度/账单问题(%d)：%s", code, truncate(body, 160))
		}
		return fmt.Errorf("上游 HTTP %d：%s", code, truncate(body, 200))
	}
}

func extractHost(msg string) string {
	// 粗提取 host:port
	for _, p := range strings.Fields(msg) {
		if strings.Contains(p, "://") || strings.Contains(p, "localhost") || strings.Contains(p, "127.0.0.1") {
			return strings.Trim(p, "\"' ")
		}
	}
	return "上游地址"
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
