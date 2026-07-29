package localapi

import (
	"strings"
	"testing"

	"devin-byok/internal/upstream/openai"
)

func TestParseTextToolCallsDevstralStyle(t *testing.T) {
	text := `[TOOL_CALLS]restricted_exec[ARGS]{"command1":{"type":"rg","pattern":"main"}}`
	calls := parseTextToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("got %d", len(calls))
	}
	if calls[0].Function.Name != "restricted_exec" {
		t.Fatalf("name=%s", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "rg") {
		t.Fatalf("args=%s", calls[0].Function.Arguments)
	}
}

func TestSynthesizeRestrictedExec(t *testing.T) {
	calls := synthesizeRestrictedExec("Find README and entry points", "")
	if len(calls) != 1 || calls[0].Function.Name != "restricted_exec" {
		t.Fatalf("%+v", calls)
	}
	if !strings.Contains(calls[0].Function.Arguments, "command1") {
		t.Fatalf("args=%s", calls[0].Function.Arguments)
	}
}

func TestDevstralNeedsForcedTool(t *testing.T) {
	if !devstralNeedsForcedTool(nil) {
		t.Fatal("empty should force")
	}
}


func TestDevstralHasToolResults(t *testing.T) {
	if devstralHasToolResults(nil) {
		t.Fatal("empty should be first turn")
	}
	if devstralHasToolResults([]openai.ChatMessage{{Role: "user", Content: "hi"}}) {
		t.Fatal("user only is first turn")
	}
	if !devstralHasToolResults([]openai.ChatMessage{{Role: "tool", Content: "ok", ToolCallID: "call_1"}}) {
		t.Fatal("tool role")
	}
	if !devstralHasToolResults([]openai.ChatMessage{{Role: "user", Content: "x", ToolCallID: "call_1"}}) {
		t.Fatal("tool_call_id")
	}
}

func TestCollectDevstralToolResultText(t *testing.T) {
	s := collectDevstralToolResultText([]openai.ChatMessage{
		{Role: "user", Content: "q"},
		{Role: "tool", Content: "README.md found"},
	})
	if !strings.Contains(s, "README") {
		t.Fatalf("%q", s)
	}
}
