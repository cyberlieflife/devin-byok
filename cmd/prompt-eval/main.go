// prompt-eval runs a small, reproducible baseline-vs-composer evaluation.
// It prints scores and metadata only; model answers and credentials are never
// written to stdout or disk.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/paths"
	"devin-byok/internal/promptstore"
	"devin-byok/internal/upstream/anthropic"
	"devin-byok/internal/upstream/openai"
)

type evalCase struct {
	Name           string
	Prompt         string
	Required       []string
	RequiresSource bool
	Strict         bool
	Check          func(string) bool
}

type dimensions struct {
	Understanding int `json:"understanding"`
	Correctness   int `json:"correctness"`
	Completeness  int `json:"completeness"`
	Evidence      int `json:"evidence"`
	Hallucination int `json:"hallucination_control"`
}

type result struct {
	Round          int        `json:"round"`
	Case           string     `json:"case"`
	Baseline       dimensions `json:"baseline"`
	Optimized      dimensions `json:"optimized"`
	BaselineScore  float64    `json:"baseline_score"`
	OptimizedScore float64    `json:"optimized_score"`
	BaselineMS     int64      `json:"baseline_ms"`
	OptimizedMS    int64      `json:"optimized_ms"`
	BaselineOK     bool       `json:"baseline_ok"`
	OptimizedOK    bool       `json:"optimized_ok"`
	Order          string     `json:"order"`
	Profiles       []string   `json:"profiles"`
	Error          string     `json:"error,omitempty"`
}

var onlyYesNo = regexp.MustCompile(`^(YES|NO)[.!]?$`)
var onlyInteger = regexp.MustCompile(`^-?[0-9]+$`)

func main() {
	cfg, err := config.Load(paths.FindConfig())
	if err != nil {
		fatal("load config: " + err.Error())
	}
	client := openai.New(cfg.Upstream)
	maxRounds := envInt("PROMPT_EVAL_ROUNDS", 2)
	if maxRounds < 1 {
		maxRounds = 1
	}
	cases := evaluationCases()
	caseLimit := envInt("PROMPT_EVAL_CASE_LIMIT", len(cases))
	if caseLimit > 0 && caseLimit < len(cases) {
		cases = cases[:caseLimit]
	}
	for _, modelID := range evaluationModels(cfg) {
		prov, ok := cfg.ResolveProvider(modelID)
		if !ok || prov.UpstreamModel == "" {
			fmt.Fprintf(os.Stderr, "skip model %s: provider is not configured\n", modelID)
			continue
		}
		var all []result
		for round := 1; round <= maxRounds; round++ {
			for _, tc := range cases {
				row := evaluateOne(client, cfg, modelID, prov, tc, round)
				all = append(all, row)
				printRow(row)
			}
		}
		printResults(all, modelID, prov.UpstreamModel, maxRounds)
	}
}

func evaluateOne(client *openai.Client, cfg *config.File, modelID string, prov config.ProviderResolved, tc evalCase, round int) result {
	baseMessages := []openai.ChatMessage{
		{Role: "system", Content: "You are a capable software engineering assistant."},
		{Role: "user", Content: tc.Prompt},
	}
	positive := true
	optimized := promptstore.ComposeMessages(baseMessages, promptstore.ComposeContext{
		Route: "chat", ModelID: modelID, Family: prov.FamilyUID, UserText: tc.Prompt,
		QualityMode: "verified", QualityEnabled: &positive,
	})
	maxTokens := 1600
	if prov.ThinkingBudgetTokens > 0 && maxTokens <= prov.ThinkingBudgetTokens {
		maxTokens = prov.ThinkingBudgetTokens + 1024
	}
	opt := openai.ChatOptions{
		Thinking: cfg.ResolveThinking(modelID), ThinkingParam: prov.ThinkingParam,
		ThinkingType: prov.ThinkingType, ThinkingBudgetTokens: prov.ThinkingBudgetTokens,
		Temperature: cfg.Upstream.Sampling.Temperature, TopP: cfg.Upstream.Sampling.TopP,
		MaxTokens: maxTokens, BaseURL: prov.BaseURL, APIKey: prov.APIKey,
		HTTPTimeout: 75 * time.Second,
	}
	out := result{Round: round, Case: tc.Name, Profiles: optimized.ProfileIDs}
	// Alternate order by round to reduce warm-cache and service-load bias.
	if round%2 == 1 {
		out.Order = "baseline_first"
		runVariant(client, prov, baseMessages, opt, tc, "baseline", &out)
		runVariant(client, prov, optimized.Messages, opt, tc, "optimized", &out)
	} else {
		out.Order = "optimized_first"
		runVariant(client, prov, optimized.Messages, opt, tc, "optimized", &out)
		runVariant(client, prov, baseMessages, opt, tc, "baseline", &out)
	}
	return out
}

func runVariant(client *openai.Client, prov config.ProviderResolved, messages []openai.ChatMessage, opt openai.ChatOptions, tc evalCase, variant string, out *result) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	answer, err := chatProvider(ctx, client, prov, messages, opt)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		if out.Error != "" {
			out.Error += "; "
		}
		out.Error += variant + ": " + err.Error()
		return
	}
	dims := score(tc, answer)
	if variant == "baseline" {
		out.BaselineMS = elapsed
		out.BaselineOK = true
		out.Baseline = dims
		out.BaselineScore = scorePercent(dims)
		return
	}
	out.OptimizedMS = elapsed
	out.OptimizedOK = true
	out.Optimized = dims
	out.OptimizedScore = scorePercent(dims)
}

func chatProvider(ctx context.Context, client *openai.Client, prov config.ProviderResolved, messages []openai.ChatMessage, opt openai.ChatOptions) (string, error) {
	var (
		result openai.ChatResult
		err    error
	)
	switch config.NormalizeProvider(prov.Provider) {
	case "anthropic":
		result, err = anthropic.New().Chat(ctx, prov.BaseURL, prov.APIKey, prov.UpstreamModel, messages, opt)
	case "responses":
		result, err = client.ChatResponses(ctx, prov.UpstreamModel, messages, opt)
	default:
		result, err = client.Chat(ctx, prov.UpstreamModel, messages, opt)
	}
	return result.Content, err
}

func evaluationModels(cfg *config.File) []string {
	raw := strings.TrimSpace(os.Getenv("PROMPT_EVAL_MODELS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("PROMPT_EVAL_MODEL"))
	}
	if raw == "" {
		return []string{cfg.DefaultModelID()}
	}
	var models []string
	seen := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			models = append(models, id)
		}
	}
	return models
}

func envInt(name string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return v
}

func score(tc evalCase, answer string) dimensions {
	lower := strings.ToLower(strings.TrimSpace(answer))
	matched := 0
	for _, want := range tc.Required {
		if strings.Contains(lower, strings.ToLower(want)) {
			matched++
		}
	}
	complete := 0
	if len(tc.Required) == 0 || matched == len(tc.Required) {
		complete = 4
	} else if matched*2 >= len(tc.Required) {
		complete = 2
	}
	correct := 0
	if tc.Check != nil && tc.Check(answer) {
		correct = 4
	}
	format := 4
	if tc.Strict && !strictOutput(tc, answer) {
		format = 0
	}
	if format == 0 {
		correct = 0
		complete = 0
	}
	understanding := complete
	evidence := 4
	if tc.RequiresSource {
		evidence = 0
		if strings.Contains(lower, "given source") || strings.Contains(lower, "source a") || strings.Contains(lower, "source b") || strings.Contains(lower, "官方") {
			evidence = 4
		} else if strings.Contains(lower, "source") || strings.Contains(lower, "provided") {
			evidence = 2
		}
	}
	hallucination := 4
	for _, phrase := range []string{"i ran", "测试已通过", "test passed", "according to https://", "引用："} {
		if strings.Contains(lower, phrase) && !tc.RequiresSource {
			hallucination = 0
		}
	}
	return dimensions{Understanding: understanding, Correctness: correct, Completeness: complete, Evidence: evidence, Hallucination: hallucination}
}

func strictOutput(tc evalCase, answer string) bool {
	clean := strings.TrimSpace(strings.Trim(answer, "`"))
	if onlyYesNo.MatchString(clean) || onlyInteger.MatchString(clean) {
		return true
	}
	if strings.HasPrefix(clean, "[") || strings.HasPrefix(clean, "{") {
		var value any
		return json.Unmarshal([]byte(clean), &value) == nil
	}
	if tc.Name == "sql-vulnerability" {
		return len(strings.Fields(clean)) <= 3
	}
	return true
}

func scorePercent(d dimensions) float64 {
	return (float64(d.Correctness)*40 + float64(d.Completeness)*20 + float64(d.Evidence)*20 + float64(d.Understanding)*15 + float64(d.Hallucination)*5) / 4
}

func evaluationCases() []evalCase {
	return []evalCase{
		{Name: "arithmetic", Prompt: "Return only the integer answer to 17*19.", Required: []string{"323"}, Strict: true, Check: func(s string) bool {
			return onlyInteger.MatchString(strings.TrimSpace(s)) && strings.TrimSpace(s) == "323"
		}},
		{Name: "sorted-unique-json", Prompt: "Return only a JSON array containing the unique values of [3,1,3,2] sorted ascending.", Required: []string{"[1,2,3]"}, Strict: true, Check: func(s string) bool { return strings.TrimSpace(s) == "[1,2,3]" }},
		{Name: "adjacent-intervals", Prompt: "Assume intervals merge only when they overlap. Should [1,2] and [2,3] merge? Return only YES or NO.", Required: []string{"NO"}, Strict: true, Check: func(s string) bool {
			return onlyYesNo.MatchString(strings.ToUpper(strings.TrimSpace(s))) && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), "NO")
		}},
		{Name: "nil-slice", Prompt: "In Go, what happens when reading s[0] from var s []int? Return the panic reason in one short sentence.", Required: []string{"panic", "slice", "index"}, Check: func(s string) bool {
			l := strings.ToLower(s)
			return strings.Contains(l, "panic") && strings.Contains(l, "slice") && strings.Contains(l, "index")
		}},
		{Name: "sql-vulnerability", Prompt: "A program builds SQL by concatenating an email string into the query. Name the vulnerability in two words.", Required: []string{"sql", "injection"}, Strict: true, Check: func(s string) bool {
			l := strings.ToLower(s)
			return strings.Contains(l, "sql") && strings.Contains(l, "injection")
		}},
		{Name: "inclusive-sum", Prompt: "Return only the integer sum of all integers in the inclusive interval [4,7].", Required: []string{"22"}, Strict: true, Check: func(s string) bool { return strings.TrimSpace(s) == "22" }},
		{Name: "boolean-logic", Prompt: "Return only TRUE or FALSE: if A is true and B is false, is (A AND B) OR NOT B true?", Required: []string{"TRUE"}, Strict: true, Check: func(s string) bool { return strings.TrimSpace(strings.ToUpper(s)) == "TRUE" }},
		{Name: "empty-max-error", Prompt: "Return only YES or NO: the maximum of an empty list is undefined, so if an API must return an error instead of a value, should it return an error?", Required: []string{"YES"}, Strict: true, Check: func(s string) bool {
			return onlyYesNo.MatchString(strings.ToUpper(strings.TrimSpace(s))) && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), "YES")
		}},
		{Name: "multi-constraint", Prompt: "Return exactly three comma-separated items, in order: for input [1,1,2], state whether duplicates exist, whether it is sorted ascending, and its unique length. Use only YES/NO/number.", Required: []string{"YES", "YES", "2"}, Strict: true, Check: func(s string) bool {
			l := strings.ReplaceAll(strings.TrimSpace(strings.ToUpper(s)), " ", "")
			return l == "YES,YES,2"
		}},
		{Name: "debug-index", Prompt: "Given Go code `for i := 0; i <= len(xs); i++ { _ = xs[i] }`, name the bug and the minimal loop-bound fix. Include both the comparison and the corrected operator.", Required: []string{"<=", "<", "index"}, Check: func(s string) bool {
			l := strings.ToLower(s)
			return strings.Contains(l, "<=") && strings.Contains(l, "<") && strings.Contains(l, "index")
		}},
		{Name: "review-risk", Prompt: "Review this line: `exec.Command(\"sh\", \"-c\", \"grep \"+userInput)` . Return the vulnerability and one safe fix, separated by a semicolon.", Required: []string{"injection", "argument"}, Check: func(s string) bool {
			l := strings.ToLower(s)
			return strings.Contains(l, "injection") && (strings.Contains(l, "argument") || strings.Contains(l, "args") || strings.Contains(l, "不拼接"))
		}},
		{Name: "provided-evidence", Prompt: "Use only the supplied facts: Source A says timeout=30s; Source B says timeout=60s. Return JSON with keys `a`, `b`, and `conflict`, where conflict is true. No explanation.", Required: []string{"\"a\"", "\"b\"", "\"conflict\"", "true"}, RequiresSource: true, Strict: true, Check: func(s string) bool {
			var v map[string]any
			if json.Unmarshal([]byte(strings.TrimSpace(s)), &v) != nil {
				return false
			}
			return v["a"] == "30s" && v["b"] == "60s" && v["conflict"] == true
		}},
	}
}

func printResults(results []result, model, upstream string, rounds int) {
	var base, opt float64
	paired := 0
	baselineFailures := 0
	optimizedFailures := 0
	for _, row := range results {
		if !row.BaselineOK {
			baselineFailures++
		}
		if !row.OptimizedOK {
			optimizedFailures++
		}
		if row.BaselineOK && row.OptimizedOK {
			paired++
			base += row.BaselineScore
			opt += row.OptimizedScore
		}
	}
	if paired > 0 {
		base /= float64(paired)
		opt /= float64(paired)
	}
	change := 0.0
	if base > 0 {
		change = (opt - base) / base * 100
	}
	fmt.Printf("SUMMARY model=%s upstream=%s rounds=%d samples=%d paired=%d baseline_failures=%d optimized_failures=%d baseline_score=%.2f optimized_score=%.2f relative_change=%.2f%%\n", model, upstream, rounds, len(results), paired, baselineFailures, optimizedFailures, base, opt, change)
	if paired >= 20 && baselineFailures == 0 && optimizedFailures == 0 && change >= 40 {
		fmt.Println("RESULT target_reached=true")
	} else {
		fmt.Println("RESULT target_not_proven=true")
	}
}

func printRow(row result) {
	b, _ := json.Marshal(row)
	fmt.Println(string(b))
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
