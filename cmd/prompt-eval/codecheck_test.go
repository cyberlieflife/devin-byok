package main

import (
	"strings"
	"testing"
)

func TestExtractGoCodeFromFence(t *testing.T) {
	code := extractGoCode("Here is code:\n```go\npackage task\n\nfunc F() int { return 1 }\n```\nDone.")
	if !strings.Contains(code, "package task") || !strings.Contains(code, "func F()") {
		t.Fatalf("extract failed: %q", code)
	}
}

func TestExtractGoCodeNormalizesPackage(t *testing.T) {
	code := extractGoCode("package main\n\nfunc F() int { return 1 }\n")
	if !strings.Contains(code, "package task") || strings.Contains(code, "package main") {
		t.Fatalf("package not normalized: %q", code)
	}
}

func TestExtractGoCodePrefixedWhenMissing(t *testing.T) {
	code := extractGoCode("func F() int { return 1 }")
	if !strings.HasPrefix(code, "package task") {
		t.Fatalf("package prefix missing: %q", code)
	}
}

func TestScoreCodePerfectImplementation(t *testing.T) {
	ct := codeTask{Tests: safeDivideTests}
	good := "package task\n\nfunc Divide(a, b int) (int, error) {\n\tif b == 0 {\n\t\treturn 0, fmt.Errorf(\"division by zero\")\n\t}\n\treturn a / b, nil\n}\n"
	_ = good
	d := scoreCode(ct, "```go\npackage task\n\nimport \"fmt\"\n\nfunc Divide(a, b int) (int, error) {\n\tif b == 0 {\n\t\treturn 0, fmt.Errorf(\"division by zero\")\n\t}\n\treturn a / b, nil\n}\n```")
	if d.Correctness != 4 || d.Understanding != 4 || d.Completeness != 4 {
		t.Fatalf("perfect code scored %+v", d)
	}
}

func TestScoreCodeBrokenImplementation(t *testing.T) {
	ct := codeTask{Tests: safeDivideTests}
	d := scoreCode(ct, "package task\n\nfunc Divide(a, b int) (int, error) { return a / b, nil }\n")
	if d.Correctness >= 4 {
		t.Fatalf("panicking implementation scored full correctness: %+v", d)
	}
}

func TestScoreCodeDoesNotCompile(t *testing.T) {
	ct := codeTask{Tests: safeDivideTests}
	d := scoreCode(ct, "package task\n\nfunc Divide(a, b int) { }\n")
	if d.Understanding != 0 || d.Correctness != 0 {
		t.Fatalf("non-compiling code scored %+v", d)
	}
}

func TestScoreCodeClaimsTestsAsHallucination(t *testing.T) {
	ct := codeTask{Tests: safeDivideTests}
	d := scoreCode(ct, "```go\npackage task\n\nimport \"fmt\"\n\nfunc Divide(a, b int) (int, error) {\n\tif b == 0 {\n\t\treturn 0, fmt.Errorf(\"division by zero\")\n\t}\n\treturn a / b, nil\n}\n```\nI ran the tests and all tests pass.")
	if d.Hallucination != 0 {
		t.Fatalf("claimed test run not flagged: %+v", d)
	}
}

var referenceImplementations = map[string]string{
	"safe-divide":     "package task\n\nimport \"fmt\"\n\nfunc Divide(a, b int) (int, error) {\n\tif b == 0 {\n\t\treturn 0, fmt.Errorf(\"division by zero\")\n\t}\n\treturn a / b, nil\n}\n",
	"clamp-value":     "package task\n\nimport \"fmt\"\n\nfunc Clamp(v, lo, hi int) (int, error) {\n\tif lo > hi {\n\t\treturn 0, fmt.Errorf(\"lo > hi\")\n\t}\n\tif v < lo {\n\t\treturn lo, nil\n\t}\n\tif v > hi {\n\t\treturn hi, nil\n\t}\n\treturn v, nil\n}\n",
	"round-ties-away": "package task\n\nimport (\n\t\"fmt\"\n\t\"math\"\n)\n\nfunc RoundTiesAway(v float64) (int, error) {\n\tif math.IsNaN(v) || math.IsInf(v, 0) {\n\t\treturn 0, fmt.Errorf(\"not finite\")\n\t}\n\tif v >= 0 {\n\t\treturn int(math.Floor(v + 0.5)), nil\n\t}\n\treturn -int(math.Floor(-v + 0.5)), nil\n}\n",
	"rotate-slice":    "package task\n\nfunc Rotate(xs []int, k int) []int {\n\tn := len(xs)\n\tif n == 0 {\n\t\treturn []int{}\n\t}\n\tk = ((k % n) + n) % n\n\tout := make([]int, n)\n\tfor i := range xs {\n\t\tout[(i+n-k)%n] = xs[i]\n\t}\n\treturn out\n}\n",
	"atoi-manual":     "package task\n\nimport (\n\t\"fmt\"\n\t\"math\"\n)\n\nfunc Atoi(s string) (int, error) {\n\tif s == \"\" {\n\t\treturn 0, fmt.Errorf(\"empty\")\n\t}\n\tsign := 1\n\ti := 0\n\tif s[0] == '+' || s[0] == '-' {\n\t\tif s[0] == '-' {\n\t\t\tsign = -1\n\t\t}\n\t\ti = 1\n\t\tif i == len(s) {\n\t\t\treturn 0, fmt.Errorf(\"sign only\")\n\t\t}\n\t}\n\tvar n int\n\tfor ; i < len(s); i++ {\n\t\tc := s[i]\n\t\tif c < '0' || c > '9' {\n\t\t\treturn 0, fmt.Errorf(\"invalid character %q\", c)\n\t\t}\n\t\td := int(c - '0')\n\t\tif n > (math.MaxInt-d)/10 {\n\t\t\treturn 0, fmt.Errorf(\"overflow\")\n\t\t}\n\t\tn = n*10 + d\n\t}\n\treturn sign * n, nil\n}\n",
	"chunk-slice":     "package task\n\nimport \"fmt\"\n\nfunc Chunk(xs []int, n int) ([][]int, error) {\n\tif n <= 0 {\n\t\treturn nil, fmt.Errorf(\"chunk size must be positive\")\n\t}\n\tout := [][]int{}\n\tfor i := 0; i < len(xs); i += n {\n\t\tend := i + n\n\t\tif end > len(xs) {\n\t\t\tend = len(xs)\n\t\t}\n\t\tout = append(out, xs[i:end])\n\t}\n\treturn out, nil\n}\n",
	"lru-cache":       "package task\n\ntype LRU struct {\n\tcap   int\n\titems map[int]int\n\torder []int\n}\n\nfunc NewLRU(capacity int) *LRU {\n\treturn &LRU{cap: capacity, items: map[int]int{}}\n}\n\nfunc (l *LRU) Get(key int) (int, bool) {\n\tv, ok := l.items[key]\n\tif !ok {\n\t\treturn 0, false\n\t}\n\tfor i, x := range l.order {\n\t\tif x == key {\n\t\t\tl.order = append(l.order[:i], l.order[i+1:]...)\n\t\t\tbreak\n\t\t}\n\t}\n\tl.order = append(l.order, key)\n\treturn v, true\n}\n\nfunc (l *LRU) Put(key, value int) {\n\tif l.cap <= 0 {\n\t\treturn\n\t}\n\tif _, ok := l.items[key]; !ok && len(l.order) >= l.cap {\n\t\tevict := l.order[0]\n\t\tl.order = l.order[1:]\n\t\tdelete(l.items, evict)\n\t}\n\tif _, ok := l.items[key]; !ok {\n\t\tl.order = append(l.order, key)\n\t}\n\tl.items[key] = value\n}\n",
	"json-path":       "package task\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nfunc Extract(data map[string]any, path string) (any, error) {\n\tparts := strings.Split(path, \".\")\n\tcur := any(data)\n\tfor _, p := range parts {\n\t\tif p == \"\" {\n\t\t\treturn nil, fmt.Errorf(\"empty segment\")\n\t\t}\n\t\tm, ok := cur.(map[string]any)\n\t\tif !ok {\n\t\t\treturn nil, fmt.Errorf(\"cannot traverse %q\", p)\n\t\t}\n\t\tv, ok := m[p]\n\t\tif !ok {\n\t\t\treturn nil, fmt.Errorf(\"missing key %q\", p)\n\t\t}\n\t\tcur = v\n\t}\n\treturn cur, nil\n}\n",
	"count-words":     "package task\n\nfunc CountWords(s string) int {\n\tcount := 0\n\tinWord := false\n\tfor _, r := range s {\n\t\tif r == ' ' || r == '\\t' || r == '\\n' || r == '\\r' {\n\t\t\tinWord = false\n\t\t} else {\n\t\t\tif !inWord {\n\t\t\t\tcount++\n\t\t\t}\n\t\t\tinWord = true\n\t\t}\n\t}\n\treturn count\n}\n",
	"parse-csv":       "package task\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nfunc ParseCSV(line string) ([]string, error) {\n\tfields := []string{}\n\tcur := strings.Builder{}\n\tinQuotes := false\n\tfor i := 0; i < len(line); i++ {\n\t\tc := line[i]\n\t\tswitch {\n\t\tcase c == '\"':\n\t\t\tif inQuotes && i+1 < len(line) && line[i+1] == '\"' {\n\t\t\t\tcur.WriteByte('\"')\n\t\t\t\ti++\n\t\t\t} else {\n\t\t\t\tinQuotes = !inQuotes\n\t\t\t}\n\t\tcase c == ',' && !inQuotes:\n\t\t\tfields = append(fields, cur.String())\n\t\t\tcur.Reset()\n\t\tdefault:\n\t\t\tcur.WriteByte(c)\n\t\t}\n\t}\n\tif inQuotes {\n\t\treturn nil, fmt.Errorf(\"unterminated quote\")\n\t}\n\tfields = append(fields, cur.String())\n\treturn fields, nil\n}\n",
	"merge-intervals": "package task\n\nfunc Merge(intervals [][2]int) [][2]int {\n\tif len(intervals) == 0 {\n\t\treturn [][2]int{}\n\t}\n\tsorted := make([][2]int, len(intervals))\n\tcopy(sorted, intervals)\n\tfor i := 0; i < len(sorted); i++ {\n\t\tfor j := i + 1; j < len(sorted); j++ {\n\t\t\tif sorted[j][0] < sorted[i][0] || (sorted[j][0] == sorted[i][0] && sorted[j][1] < sorted[i][1]) {\n\t\t\t\tsorted[i], sorted[j] = sorted[j], sorted[i]\n\t\t\t}\n\t\t}\n\t}\n\tout := [][2]int{}\n\tcur := sorted[0]\n\tfor _, iv := range sorted[1:] {\n\t\tif iv[0] < cur[1] {\n\t\t\tif iv[1] > cur[1] {\n\t\t\t\tcur[1] = iv[1]\n\t\t\t}\n\t\t} else {\n\t\t\tout = append(out, cur)\n\t\t\tcur = iv\n\t\t}\n\t}\n\tout = append(out, cur)\n\treturn out\n}\n",
	"fix-off-by-one":  "package task\n\nfunc LastBelow(xs []int, limit int) int {\n\tlast := -1\n\tfor i := 0; i < len(xs); i++ {\n\t\tif xs[i] < limit {\n\t\t\tlast = i\n\t\t}\n\t}\n\treturn last\n}\n",
}

func TestReferenceImplementationsPassFixedTests(t *testing.T) {
	for _, tc := range evaluationCases() {
		ref, ok := referenceImplementations[tc.Name]
		if !ok {
			t.Fatalf("missing reference for %s", tc.Name)
		}
		pass, total, compiled := tc.Code.run(ref)
		if !compiled || pass != total {
			t.Fatalf("%s: reference passed %d/%d compiled=%v — the fixed tests or reference are wrong", tc.Name, pass, total, compiled)
		}
	}
}
