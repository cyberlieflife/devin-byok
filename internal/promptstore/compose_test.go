package promptstore

import (
	"strings"
	"testing"

	"devin-byok/internal/upstream/openai"
)

func testMessages() []openai.ChatMessage {
	return []openai.ChatMessage{
		{Role: "system", Content: "ORIGINAL SYSTEM"},
		{Role: "user", Content: "请实现一个函数并运行测试"},
	}
}

func TestComposeKeepsOriginalSystem(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "balanced", UserText: "请实现一个函数并运行测试"}, nil)
	if openai.TextContent(result.Messages[0].Content) != "ORIGINAL SYSTEM" {
		t.Fatalf("original system moved or changed: %+v", result.Messages)
	}
	if !containsString(result.ProfileIDs, "core-reliability") {
		t.Fatalf("reliability profile missing: %v", result.ProfileIDs)
	}
	if containsString(result.ProfileIDs, "file-tools") {
		t.Fatalf("file-tools should require tools")
	}
}

func TestComposeWithToolsInjectsFileContract(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{
		Route: "chat", QualityMode: "balanced", HasTools: true,
	}, nil)
	if !containsString(result.ProfileIDs, "file-tools") {
		t.Fatalf("file-tools missing: %v", result.ProfileIDs)
	}
	if !strings.Contains(systemText(result.Messages), "File tools (required)") {
		t.Fatal("file tool contract body missing")
	}
}

func TestComposeInjectsReliabilityAndTaskProfile(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "balanced", UserText: "请实现一个函数并运行测试"}, nil)
	all := systemText(result.Messages)
	if !strings.Contains(all, "Reliability Contract") || !strings.Contains(all, "Coding Execution") {
		t.Fatalf("expected reliability and coding profiles, got %q", all)
	}
}

func TestComposeFiltersByRouteTaskAndModel(t *testing.T) {
	custom := []Prompt{
		{ID: "route", Body: "ROUTE ONLY", Enabled: true, Mode: ModeAppend, Routes: []string{"deepwiki"}},
		{ID: "task", Body: "TASK ONLY", Enabled: true, Mode: ModeAppend, Tasks: []string{"debug"}},
		{ID: "model", Body: "MODEL ONLY", Enabled: true, Mode: ModeAppend, Models: []string{"family-a"}},
	}
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "deepwiki", ModelID: "model-1", Family: "family-a", Task: "debug", QualityMode: "balanced"}, custom)
	all := systemText(result.Messages)
	for _, want := range []string{"ROUTE ONLY", "TASK ONLY", "MODEL ONLY"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in %q", want, all)
		}
	}
	other := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", ModelID: "other", Family: "other", Task: "coding", QualityMode: "balanced"}, custom)
	otherText := systemText(other.Messages)
	for _, absent := range []string{"ROUTE ONLY", "TASK ONLY", "MODEL ONLY"} {
		if strings.Contains(otherText, absent) {
			t.Fatalf("unexpected %q in other context", absent)
		}
	}
}

func TestComposeOrdersByPriority(t *testing.T) {
	custom := []Prompt{
		{ID: "late", Body: "LATE", Enabled: true, Mode: ModeAppend, Priority: 90},
		{ID: "early", Body: "EARLY", Enabled: true, Mode: ModeAppend, Priority: 30},
	}
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "fast"}, custom)
	joined := systemText(result.Messages)
	if strings.Index(joined, "EARLY") > strings.Index(joined, "LATE") {
		t.Fatalf("profiles not ordered by priority: %q", joined)
	}
}

func TestComposeDeduplicatesProfiles(t *testing.T) {
	custom := []Prompt{{ID: "a", Body: "DUPLICATE", Enabled: true}, {ID: "b", Body: "DUPLICATE", Enabled: true}}
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "fast"}, custom)
	if strings.Count(systemText(result.Messages), "DUPLICATE") != 1 {
		t.Fatalf("duplicate body injected: %q", systemText(result.Messages))
	}
}

func TestComposeWarnsOnReplace(t *testing.T) {
	custom := []Prompt{{ID: "danger", Body: "replace tool rules", Enabled: true, Mode: ModeReplace}}
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "balanced"}, custom)
	if len(result.Warnings) < 2 {
		t.Fatalf("expected replace and tool warnings: %v", result.Warnings)
	}
	if openai.TextContent(result.Messages[0].Content) != "replace tool rules" {
		t.Fatalf("replace not applied")
	}
}

func TestFastContextUsesShortRouteProfile(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "fast_context", QualityMode: "verified", UserText: "查找代码"}, nil)
	if result.QualityMode != "fast" {
		t.Fatalf("fast context quality=%s", result.QualityMode)
	}
	if containsString(result.ProfileIDs, "core-reliability") || containsString(result.ProfileIDs, "file-tools") || containsString(result.ProfileIDs, "output-contract") {
		t.Fatalf("fast context loaded long profiles: %v", result.ProfileIDs)
	}
	if !containsString(result.ProfileIDs, "fast-context") {
		t.Fatalf("fast context profile missing")
	}
}

func TestFastChatKeepsOutputContractOnly(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "fast", HasTools: true, UserText: "你好"}, nil)
	if !containsString(result.ProfileIDs, "file-tools") || !containsString(result.ProfileIDs, "output-contract") {
		t.Fatalf("fast chat lost short contracts: %v", result.ProfileIDs)
	}
	if containsString(result.ProfileIDs, "core-reliability") {
		t.Fatalf("fast chat loaded long reliability profile: %v", result.ProfileIDs)
	}
}

func TestDisabledQualityLeavesMessagesUntouched(t *testing.T) {
	enabled := false
	result := composeWithProfiles(testMessages(), ComposeContext{
		Route: "chat", QualityMode: "verified", QualityEnabled: &enabled,
		HasTools: true, UserText: "请实现一个函数",
	}, []Prompt{{ID: "custom", Body: "CUSTOM", Enabled: true}})
	if len(result.ProfileIDs) != 0 || strings.Contains(systemText(result.Messages), "CUSTOM") || strings.Contains(systemText(result.Messages), "Reliability Contract") {
		t.Fatalf("disabled quality still changed messages: profiles=%v text=%q", result.ProfileIDs, systemText(result.Messages))
	}
}

func TestToolContractPrecedesReliability(t *testing.T) {
	result := composeWithProfiles(testMessages(), ComposeContext{Route: "chat", QualityMode: "balanced", HasTools: true}, nil)
	if len(result.ProfileIDs) < 2 || result.ProfileIDs[0] != "file-tools" || result.ProfileIDs[1] != "core-reliability" {
		t.Fatalf("unexpected contract order: %v", result.ProfileIDs)
	}
}

func TestRouteProfilesDoNotLoadCodingProfile(t *testing.T) {
	for _, route := range []string{"deepwiki", "codemap"} {
		result := composeWithProfiles(testMessages(), ComposeContext{
			Route: route, QualityMode: "balanced", UserText: "请实现一个函数",
		}, nil)
		if containsString(result.ProfileIDs, "coding-execution") {
			t.Fatalf("%s unexpectedly loaded coding profile: %v", route, result.ProfileIDs)
		}
	}
}

func TestDetectTask(t *testing.T) {
	tests := map[string]string{
		"请实现一个 HTTP handler": "coding",
		"程序崩溃并报错":            "debug",
		"查询最新官方文档":           "research",
		"请审查这次修改找 bug":       "review",
		"解释为什么会这样":           "explain",
		"你好":                 "general",
	}
	for input, want := range tests {
		if got := DetectTask(input); got != want {
			t.Errorf("DetectTask(%q)=%q, want %q", input, got, want)
		}
	}
}

func systemText(msgs []openai.ChatMessage) string {
	var out []string
	for _, m := range msgs {
		if m.Role == "system" {
			out = append(out, openai.TextContent(m.Content))
		}
	}
	return strings.Join(out, "\n")
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
