package localapi

import (
	"strings"
	"context"

	"devin-byok/internal/config"
	"devin-byok/internal/upstream/anthropic"
	"devin-byok/internal/upstream/openai"
)

// chatDispatch 按供应商调用 OpenAI 兼容或 Anthropic 原生 Messages API。
func (s *Server) chatOnce(ctx context.Context, prov config.ProviderResolved, model string, msgs []openai.ChatMessage, opt openai.ChatOptions) (openai.ChatResult, error) {
	if config.NormalizeProvider(prov.Provider) == "anthropic" {
		c := anthropic.New()
		return c.Chat(ctx, firstNonEmptyStr(opt.BaseURL, prov.BaseURL), firstNonEmptyStr(opt.APIKey, prov.APIKey), model, msgs, opt)
	}
	return s.upstream.Chat(ctx, model, msgs, opt)
}

func (s *Server) chatStream(ctx context.Context, prov config.ProviderResolved, model string, msgs []openai.ChatMessage, opt openai.ChatOptions, onDelta func(openai.StreamDelta) error) (openai.Usage, error) {
	if config.NormalizeProvider(prov.Provider) == "anthropic" {
		c := anthropic.New()
		return c.StreamChat(ctx, firstNonEmptyStr(opt.BaseURL, prov.BaseURL), firstNonEmptyStr(opt.APIKey, prov.APIKey), model, msgs, opt, onDelta)
	}
	return s.upstream.StreamChat(ctx, model, msgs, opt, onDelta)
}


func firstNonEmptyStr(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
