package localapi

import (
	"testing"

	"devin-byok/internal/upstream/openai"
)

func TestValidateWriteRequiresTargetFile(t *testing.T) {
	calls := []openai.ToolCall{{
		ID:   "1",
		Type: "function",
	}}
	calls[0].Function.Name = "write_to_file"
	calls[0].Function.Arguments = `{"Contents":"print(1)"}`
	ok, warn := validateToolCallsEx(calls, nil)
	if len(ok) != 0 {
		t.Fatalf("expected skip incomplete write, got %#v warn=%q", ok, warn)
	}
	if warn == "" {
		t.Fatal("expected warning about missing TargetFile")
	}

	calls[0].Function.Arguments = `{"TargetFile":"bubble_sort.py","Contents":"print(1)"}`
	ok, warn = validateToolCallsEx(calls, nil)
	if len(ok) != 1 {
		t.Fatalf("expected accept write with TargetFile, got %#v warn=%q", ok, warn)
	}
}