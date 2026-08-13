package localapi

import (
	"testing"

	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/openai"
)

func TestPromptHashChangesCacheKey(t *testing.T) {
	msgs := []openai.ChatMessage{{Role: "user", Content: "same"}}
	a := respCacheKey("model", "high", "sha256:a", msgs, nil)
	b := respCacheKey("model", "high", "sha256:b", msgs, nil)
	if a == b {
		t.Fatal("different effective prompts share response cache key")
	}
}

func TestDifferentToolsDoNotShareCacheKey(t *testing.T) {
	msgs := []openai.ChatMessage{{Role: "user", Content: "same"}}
	tool := openai.Tool{Type: "function", Function: openai.ToolFunction{Name: "read_file", Parameters: map[string]any{"type": "object"}}}
	if respCacheKey("model", "high", "p", msgs, nil) == respCacheKey("model", "high", "p", msgs, []openai.Tool{tool}) {
		t.Fatal("different tools share cache key")
	}
}

func TestDifferentQualityModesDoNotShareCacheKey(t *testing.T) {
	msgs := []openai.ChatMessage{{Role: "system", Content: "upstream"}, {Role: "user", Content: "implement"}}
	balanced := promptstore.ComposeMessages(msgs, promptstore.ComposeContext{
		Route: "chat", UserText: "implement", QualityMode: "balanced",
	})
	verified := promptstore.ComposeMessages(msgs, promptstore.ComposeContext{
		Route: "chat", UserText: "implement", QualityMode: "verified",
	})
	if balanced.Hash == verified.Hash {
		t.Fatal("quality modes produced the same effective prompt hash")
	}
	if respCacheKey("model", "high", balanced.Hash, balanced.Messages, nil) == respCacheKey("model", "high", verified.Hash, verified.Messages, nil) {
		t.Fatal("different quality modes share response cache key")
	}
}
