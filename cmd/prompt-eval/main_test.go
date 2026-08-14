package main

import (
	"os"
	"testing"

	"devin-byok/internal/config"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/openai"
)

func TestEvaluationHasAtLeastTenDeterministicCases(t *testing.T) {
	cases := evaluationCases()
	if len(cases) < 10 {
		t.Fatalf("evaluation cases = %d, want at least 10", len(cases))
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.Name == "" {
			t.Fatalf("invalid case: %+v", tc)
		}
		if tc.Code == nil && tc.Check == nil {
			t.Fatalf("case %s has neither Code nor Check", tc.Name)
		}
		if tc.Code != nil && tc.Prompt != "" {
			t.Fatalf("case %s must not set Prompt for code tasks", tc.Name)
		}
		if seen[tc.Name] {
			t.Fatalf("duplicate case name: %s", tc.Name)
		}
		seen[tc.Name] = true
	}
}

func TestEvaluationCodeTasksUseMaxTokens(t *testing.T) {
	for _, tc := range evaluationCases() {
		if tc.Code != nil && tc.MaxTokens < 2000 {
			t.Fatalf("code case %s needs MaxTokens >= 2000, got %d", tc.Name, tc.MaxTokens)
		}
	}
}

func TestEvaluationModelsParsesUniqueList(t *testing.T) {
	oldModels, hadModels := os.LookupEnv("PROMPT_EVAL_MODELS")
	oldModel, hadModel := os.LookupEnv("PROMPT_EVAL_MODEL")
	t.Cleanup(func() {
		if hadModels {
			_ = os.Setenv("PROMPT_EVAL_MODELS", oldModels)
		} else {
			_ = os.Unsetenv("PROMPT_EVAL_MODELS")
		}
		if hadModel {
			_ = os.Setenv("PROMPT_EVAL_MODEL", oldModel)
		} else {
			_ = os.Unsetenv("PROMPT_EVAL_MODEL")
		}
	})
	_ = os.Setenv("PROMPT_EVAL_MODELS", "grok-high, claude, grok-high")
	_ = os.Unsetenv("PROMPT_EVAL_MODEL")
	got := evaluationModels(&config.File{})
	if len(got) != 2 || got[0] != "grok-high" || got[1] != "claude" {
		t.Fatalf("models = %v", got)
	}
}

func TestScoreRejectsStrictFormatNoise(t *testing.T) {
	tc := evalCase{Name: "arithmetic", Required: []string{"323"}, Strict: true, Check: func(s string) bool { return s == "323" }}
	good := score(tc, "323")
	bad := score(tc, "The answer is 323")
	if scorePercent(good) <= scorePercent(bad) {
		t.Fatalf("strict format noise was not penalized: good=%+v bad=%+v", good, bad)
	}
}

// TestCodeCaseComposerUsesFullVerifiedProfiles 防止回归：code 评测的
// ComposeContext.UserText 不再包含 "Return only the code." 等 strict 触发词，
// 否则 core-reliability/verification/coding-execution 会被跳过，baseline 与
// optimized 的差异退化为两个短契约，评测无法证明 composer 主干的价值。
func TestCodeCaseComposerUsesFullVerifiedProfiles(t *testing.T) {
	for _, tc := range evaluationCases() {
		if tc.Code == nil {
			continue
		}
		prompt := codePrompt(tc.Code.Function, tc.Code.Contract)
		composerText := prompt
		if tc.Code != nil {
			composerText = "实现一个 Go 函数：" + tc.Code.Function + "。需要正确处理空输入、边界值和非法输入，并补充测试。"
		}
		_ = prompt
		positive := true
		composed := promptstore.ComposeMessages([]openai.ChatMessage{
			{Role: "system", Content: "You are a capable software engineering assistant."},
			{Role: "user", Content: prompt},
		}, promptstore.ComposeContext{
			Route: "chat", ModelID: "model", Family: "family", UserText: composerText,
			QualityMode: "verified", QualityEnabled: &positive,
		})
		for _, want := range []string{"core-reliability", "verification", "coding-execution"} {
			found := false
			for _, id := range composed.ProfileIDs {
				if id == want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("case %s: profile %q missing from verified composer (strict short-circuit regression?) profiles=%v", tc.Name, want, composed.ProfileIDs)
			}
		}
		if composed.Task != "coding" {
			t.Fatalf("case %s: task = %q, want coding", tc.Name, composed.Task)
		}
	}
}
