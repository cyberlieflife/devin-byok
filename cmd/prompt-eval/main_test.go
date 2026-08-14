package main

import (
	"os"
	"testing"

	"devin-byok/internal/config"
)

func TestEvaluationHasAtLeastTenDeterministicCases(t *testing.T) {
	cases := evaluationCases()
	if len(cases) < 10 {
		t.Fatalf("evaluation cases = %d, want at least 10", len(cases))
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if tc.Name == "" || tc.Prompt == "" || tc.Check == nil {
			t.Fatalf("invalid case: %+v", tc)
		}
		if seen[tc.Name] {
			t.Fatalf("duplicate case name: %s", tc.Name)
		}
		seen[tc.Name] = true
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
