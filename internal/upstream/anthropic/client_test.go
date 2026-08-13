package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"devin-byok/internal/upstream/openai"
)

func TestAnthropicThinkingPayload(t *testing.T) {
	c := New()
	b, err := c.build("claude", []openai.ChatMessage{{Role: "user", Content: "hi"}}, false, openai.ChatOptions{MaxTokens: 4096, ThinkingType: "enabled", ThinkingBudgetTokens: 3000})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	thinking, ok := got["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(3000) {
		t.Fatalf("bad thinking payload: %s", b)
	}
}

func TestAnthropicNonThinkingOmitsThinking(t *testing.T) {
	b, err := New().build("claude", nil, false, openai.ChatOptions{MaxTokens: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"thinking"`) {
		t.Fatalf("unexpected thinking: %s", b)
	}
}

func TestAnthropicThinkingBudgetValidation(t *testing.T) {
	_, err := New().build("claude", nil, false, openai.ChatOptions{MaxTokens: 3000, ThinkingType: "enabled", ThinkingBudgetTokens: 3000})
	if err == nil || !strings.Contains(err.Error(), "must be less than max_tokens") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}
