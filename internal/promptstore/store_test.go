package promptstore

import (
	"strings"
	"testing"

	"devin-byok/internal/upstream/openai"
)

func TestApplyToMessagesAlwaysInjectsBuiltin(t *testing.T) {
	msgs := []openai.ChatMessage{{Role: "user", Content: "hi"}}
	out := ApplyToMessages(msgs)
	found := false
	for _, m := range out {
		if m.Role == "system" && strings.Contains(openai.TextContent(m.Content), "File tools (required)") {
			found = true
		}
	}
	if !found {
		t.Fatalf("builtin file tools prompt not injected: %+v", out)
	}
	if !strings.Contains(BuiltinFileToolsPrompt, "write_to_file") {
		t.Fatal("builtin text missing write_to_file")
	}
	if !strings.Contains(BuiltinFileToolsPrompt, "Prefer tools over commands") {
		t.Fatal("builtin text missing prefer tools sentence")
	}
}
