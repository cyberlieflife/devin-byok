package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"devin-byok/internal/config"
)

func TestOpenAIReasoningEffortPayload(t *testing.T) {
	c := &Client{cfg: config.UpstreamConfig{Thinking: config.ThinkingConfig{Param: "reasoning_effort"}}}
	body := c.buildChatRequest("grok", []ChatMessage{{Role: "user", Content: "hi"}}, false, ChatOptions{Thinking: "high"})
	b, _ := json.Marshal(body)
	if !strings.Contains(string(b), `"reasoning_effort":"high"`) {
		t.Fatalf("missing effort: %s", b)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("unexpected anthropic thinking: %s", b)
	}
}

func TestOpenAIReasoningNestedPayload(t *testing.T) {
	c := &Client{cfg: config.UpstreamConfig{Thinking: config.ThinkingConfig{Param: "reasoning.effort"}}}
	body := c.buildChatRequest("grok", nil, false, ChatOptions{Thinking: "medium"})
	if body.Reasoning == nil || body.Reasoning.Effort != "medium" {
		t.Fatalf("nested reasoning missing: %+v", body)
	}
}
