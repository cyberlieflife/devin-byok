package localapi

import (
	"context"
	"strings"

	"devin-byok/internal/config"
	"devin-byok/internal/upstream/anthropic"
	"devin-byok/internal/upstream/openai"
)

// chatDispatch 按供应商调用 OpenAI 兼容、Responses 或 Anthropic API。
func (s *Server) chatOnce(ctx context.Context, prov config.ProviderResolved, model string, msgs []openai.ChatMessage, opt openai.ChatOptions) (openai.ChatResult, error) {
	opt.BaseURL = firstNonEmptyStr(opt.BaseURL, prov.BaseURL)
	opt.APIKey = firstNonEmptyStr(opt.APIKey, prov.APIKey)
	switch config.NormalizeProvider(prov.Provider) {
	case "anthropic":
		c := anthropic.New()
		return c.Chat(ctx, opt.BaseURL, opt.APIKey, model, msgs, opt)
	case "responses":
		return s.upstream.ChatResponses(ctx, model, msgs, opt)
	default:
		return s.upstream.Chat(ctx, model, msgs, opt)
	}
}

func (s *Server) chatStream(ctx context.Context, prov config.ProviderResolved, model string, msgs []openai.ChatMessage, opt openai.ChatOptions, onDelta func(openai.StreamDelta) error) (openai.Usage, error) {
	opt.BaseURL = firstNonEmptyStr(opt.BaseURL, prov.BaseURL)
	opt.APIKey = firstNonEmptyStr(opt.APIKey, prov.APIKey)
	switch config.NormalizeProvider(prov.Provider) {
	case "anthropic":
		c := anthropic.New()
		return c.StreamChat(ctx, opt.BaseURL, opt.APIKey, model, msgs, opt, onDelta)
	case "responses":
		return s.upstream.StreamChatResponses(ctx, model, msgs, opt, onDelta)
	default:
		return s.upstream.StreamChat(ctx, model, msgs, opt, onDelta)
	}
}

func firstNonEmptyStr(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
