package localapi

import (
	"strings"
	"testing"

	"devin-byok/internal/config"
	"devin-byok/internal/upstream/openai"
)

func TestFilterToolsByPolicy(t *testing.T) {
	tools := []openai.Tool{
		{Type: "function", Function: openai.ToolFunction{Name: "read_file"}},
		{Type: "function", Function: openai.ToolFunction{Name: "write_to_file"}},
		{Type: "function", Function: openai.ToolFunction{Name: "run_command"}},
		{Type: "function", Function: openai.ToolFunction{Name: "do_not_call"}},
	}
	cfg := &config.File{
		Features: config.FeaturesConfig{EnableCascadeTools: true},
		Tools:    config.ToolsConfig{Mode: "readonly"},
	}
	out, mode := filterToolsByPolicy(tools, cfg)
	if mode != "readonly" || len(out) != 1 || out[0].Function.Name != "read_file" {
		t.Fatalf("readonly out=%+v mode=%s", out, mode)
	}
	cfg.Tools.Mode = "standard"
	out, _ = filterToolsByPolicy(tools, cfg)
	if len(out) != 2 {
		t.Fatalf("standard want 2 got %d", len(out))
	}
	cfg.Tools.Mode = "full"
	out, _ = filterToolsByPolicy(tools, cfg)
	if len(out) != 3 {
		t.Fatalf("full want 3 got %d", len(out))
	}
}

func TestValidateToolCallsJSON(t *testing.T) {
	good := openai.ToolCall{ID: "1", Type: "function"}
	good.Function.Name = "read_file"
	good.Function.Arguments = `{"file_path":"D:\\\\a.go"}`
	bad := openai.ToolCall{ID: "2", Type: "function"}
	bad.Function.Name = "write_to_file"
	bad.Function.Arguments = `{"file_path":"x","content":"hel`
	ok, errText := validateToolCalls([]openai.ToolCall{good, bad})
	if len(ok) != 1 || ok[0].Function.Name != "read_file" {
		t.Fatalf("ok=%+v err=%s", ok, errText)
	}
	if errText == "" {
		t.Fatal("want partial error text")
	}
}

func TestValidateGrepOutsideWorkspace(t *testing.T) {
	tc := openai.ToolCall{ID: "g1", Type: "function"}
	tc.Function.Name = "grep_search"
	tc.Function.Arguments = `{"SearchPath":"D:\\\\Devin-byok","Query":"ToolsMode"}`
	ok, errText := validateToolCallsEx([]openai.ToolCall{tc}, []string{`D:\Code\DevinTest`})
	if len(ok) != 0 {
		t.Fatalf("should reject outside workspace: %+v", ok)
	}
	if !strings.Contains(errText, "???") && !strings.Contains(errText, "workspace") && !strings.Contains(errText, "run_command") {
		t.Fatalf("want workspace error, got %s", errText)
	}
	// inside workspace ok
	tc.Function.Arguments = `{"SearchPath":"D:\\\\Code\\\\DevinTest","Query":"ToolsMode"}`
	ok, errText = validateToolCallsEx([]openai.ToolCall{tc}, []string{`D:\Code\DevinTest`})
	if len(ok) != 1 || errText != "" {
		t.Fatalf("inside should pass ok=%+v err=%s", ok, errText)
	}
}

func TestExtractWorkspaceRoots(t *testing.T) {
	plain := []byte(`blah workspace folder d:\Code\DevinTest more`)
	roots := extractWorkspaceRoots(plain)
	if len(roots) == 0 {
		t.Fatal("expected roots")
	}
	found := false
	for _, r := range roots {
		if strings.Contains(strings.ToLower(r), `d:\code\devintest`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("roots=%v", roots)
	}
}

func TestNeedsWorkspaceHint(t *testing.T) {
	if !needsWorkspaceHint([]byte("user has no open workspace now"), "hi", nil) {
		t.Fatal("expected hint")
	}
}

func TestHumanizeWorkspaceSearchError(t *testing.T) {
	msg := humanizeChatError(errString("error executing cascade step: Search path D:/Devin-byok is not within any current workspace."))
	if !strings.Contains(msg, "???") {
		t.Fatalf("msg=%s", msg)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
