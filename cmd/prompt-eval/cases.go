package main

func codePrompt(fn, contract string) string {
	return "Write Go code for a single file that starts with `package task` and contains ONLY the requested function plus required imports. No tests, no main function, no explanations, no markdown fences, no example usage.\n\nFunction: " + fn + "\n\nContract:\n" + contract + "\n\nReturn only the code."
}

const mergeIntervalsTests = `package task

import "testing"

` + assertSliceHelper + `
func TestMerge(t *testing.T) {
	cases := []struct {
		in  [][2]int
		out [][2]int
	}{
		{nil, nil},
		{[][2]int{{1, 2}}, [][2]int{{1, 2}}},
		{[][2]int{{1, 3}, {2, 4}}, [][2]int{{1, 4}}},
		{[][2]int{{1, 2}, {2, 3}}, [][2]int{{1, 2}, {2, 3}}},
		{[][2]int{{5, 7}, {1, 2}, {2, 6}}, [][2]int{{1, 2}, {2, 7}}},
		{[][2]int{{1, 10}, {3, 4}}, [][2]int{{1, 10}}},
	}
	for i, c := range cases {
		got := Merge(c.in)
		if len(got) != len(c.out) {
			t.Fatalf("case %d Merge(%v)=%v want %v", i, c.in, got, c.out)
		}
		for j := range got {
			if got[j] != c.out[j] {
				t.Fatalf("case %d Merge(%v)=%v want %v", i, c.in, got, c.out)
			}
		}
	}
}
`

const safeDivideTests = `package task

import "testing"

func TestDivide(t *testing.T) {
	cases := []struct {
		a, b int
		want int
	}{
		{10, 2, 5},
		{7, 2, 3},
		{0, 5, 0},
		{-10, 2, -5},
	}
	for _, c := range cases {
		got, err := Divide(c.a, c.b)
		if err != nil {
			t.Fatalf("Divide(%d,%d) unexpected error: %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Fatalf("Divide(%d,%d)=%d want %d", c.a, c.b, got, c.want)
		}
	}
	if _, err := Divide(5, 0); err == nil {
		t.Fatal("Divide(5,0) expected error, got nil")
	}
	if _, err := Divide(0, 0); err == nil {
		t.Fatal("Divide(0,0) expected error, got nil")
	}
}
`

const clampTests = `package task

import "testing"

func TestClamp(t *testing.T) {
	cases := []struct {
		v, lo, hi, want int
	}{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{20, 1, 10, 10},
		{1, 1, 10, 1},
		{10, 1, 10, 10},
	}
	for _, c := range cases {
		got, err := Clamp(c.v, c.lo, c.hi)
		if err != nil {
			t.Fatalf("Clamp(%d,%d,%d) unexpected error: %v", c.v, c.lo, c.hi, err)
		}
		if got != c.want {
			t.Fatalf("Clamp(%d,%d,%d)=%d want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
	if _, err := Clamp(5, 10, 1); err == nil {
		t.Fatal("Clamp(5,10,1) expected error for lo>hi")
	}
}
`

const roundTiesAwayTests = `package task

import "testing"

func TestRoundTiesAway(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0.5, 1},
		{-0.5, -1},
		{2.5, 3},
		{-2.5, -3},
		{1.49, 1},
		{-1.49, -1},
		{0.0, 0},
		{3.5, 4},
		{-3.5, -4},
	}
	for _, c := range cases {
		got, err := RoundTiesAway(c.in)
		if err != nil {
			t.Fatalf("RoundTiesAway(%v) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("RoundTiesAway(%v)=%d want %d", c.in, got, c.want)
		}
	}
	for _, bad := range []float64{nan(), inf(), -inf()} {
		if _, err := RoundTiesAway(bad); err == nil {
			t.Fatalf("RoundTiesAway(%v) expected error, got nil", bad)
		}
	}
}

func nan() float64 {
	var z float64
	return 0 / z
}

func inf() float64 {
	var z float64
	return 1 / z
}
`

const rotateTests = `package task

import "testing"

` + assertSliceHelper + `
func TestRotate(t *testing.T) {
	cases := []struct {
		in   []int
		k    int
		want []int
	}{
		{[]int{1, 2, 3, 4}, 2, []int{3, 4, 1, 2}},
		{[]int{1, 2, 3, 4}, 4, []int{1, 2, 3, 4}},
		{[]int{1, 2, 3, 4}, 6, []int{3, 4, 1, 2}},
		{[]int{1, 2, 3, 4}, -1, []int{4, 1, 2, 3}},
		{[]int{1, 2, 3, 4}, 0, []int{1, 2, 3, 4}},
		{nil, 3, nil},
		{[]int{1}, 5, []int{1}},
	}
	for _, c := range cases {
		assertSlices(t, Rotate(c.in, c.k), c.want)
	}
}
`

const atoiTests = `package task

import "testing"

func TestAtoi(t *testing.T) {
	good := []struct {
		in   string
		want int
	}{
		{"42", 42},
		{"+42", 42},
		{"-42", -42},
		{"0", 0},
		{"+0", 0},
		{"2147483647", 2147483647},
	}
	for _, c := range good {
		got, err := Atoi(c.in)
		if err != nil {
			t.Fatalf("Atoi(%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("Atoi(%q)=%d want %d", c.in, got, c.want)
		}
	}
	bad := []string{"", "+", "-", "+-", "4a2", " 42", "42 ", "99999999999999999999", "4.2"}
	for _, in := range bad {
		if _, err := Atoi(in); err == nil {
			t.Fatalf("Atoi(%q) expected error, got nil", in)
		}
	}
}
`

const chunkTests = `package task

import "testing"

func TestChunk(t *testing.T) {
	got, err := Chunk([]int{1, 2, 3, 4, 5}, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]int{{1, 2}, {3, 4}, {5}}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("chunk %d len=%d want %d", i, len(got[i]), len(want[i]))
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("chunk %d at %d got %v want %v", i, j, got[i], want[i])
			}
		}
	}
	if g2, err := Chunk([]int{1}, 2); err != nil || len(g2) != 1 || g2[0][0] != 1 {
		t.Fatalf("single-element chunk wrong: %v %v", g2, err)
	}
	if g3, err := Chunk([]int{}, 2); err != nil || len(g3) != 0 {
		t.Fatalf("empty chunk wrong: %v %v", g3, err)
	}
	for _, n := range []int{0, -1} {
		if _, err := Chunk([]int{1, 2}, n); err == nil {
			t.Fatalf("Chunk with n=%d expected error, got nil", n)
		}
	}
}
`

const lruTests = `package task

import "testing"

func TestLRU(t *testing.T) {
	l := NewLRU(2)
	l.Put(1, 10)
	l.Put(2, 20)
	if v, ok := l.Get(1); !ok || v != 10 {
		t.Fatalf("Get(1)=%d,%v want 10,true", v, ok)
	}
	l.Put(3, 30)
	if _, ok := l.Get(2); ok {
		t.Fatal("Get(2) should evict the least recently used key")
	}
	if v, ok := l.Get(1); !ok || v != 10 {
		t.Fatalf("Get(1)=%d,%v want 10,true", v, ok)
	}
	if v, ok := l.Get(3); !ok || v != 30 {
		t.Fatalf("Get(3)=%d,%v want 30,true", v, ok)
	}
	l.Put(1, 99)
	if v, _ := l.Get(1); v != 99 {
		t.Fatalf("overwrite failed: %d", v)
	}
}

func TestLRUZeroCapacity(t *testing.T) {
	l := NewLRU(0)
	l.Put(1, 1)
	if _, ok := l.Get(1); ok {
		t.Fatal("zero-capacity LRU must not store")
	}
}
`

const jsonPathTests = `package task

import "testing"

func TestExtract(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": map[string]any{"c": 42}}}
	if v, err := Extract(data, "a.b.c"); err != nil || v != 42 {
		t.Fatalf("Extract(a.b.c)=%v,%v want 42", v, err)
	}
	if v, err := Extract(data, "a.b"); err != nil {
		t.Fatalf("Extract(a.b) error: %v", err)
	} else if m, ok := v.(map[string]any); !ok || m["c"] != 42 {
		t.Fatalf("Extract(a.b)=%v", v)
	}
	if _, err := Extract(data, "x"); err == nil {
		t.Fatal("Extract missing key expected error")
	}
	if _, err := Extract(data, ""); err == nil {
		t.Fatal("Extract empty path expected error")
	}
	flat := map[string]any{"a": 1}
	if _, err := Extract(flat, "a.b"); err == nil {
		t.Fatal("Extract through non-object expected error")
	}
}
`

const countWordsTests = `package task

import "testing"

func TestCountWords(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"hello world", 2},
		{"  a  b  ", 2},
		{"a\nb\tc", 3},
		{"", 0},
		{"   ", 0},
		{"one", 1},
		{"a\r\nb", 2},
	}
	for _, c := range cases {
		if got := CountWords(c.in); got != c.want {
			t.Fatalf("CountWords(%q)=%d want %d", c.in, got, c.want)
		}
	}
}
`

const parseCSVTests = `package task

import "testing"

` + assertStringSlicesHelper + `
func TestParseCSV(t *testing.T) {
	good := []struct {
		in   string
		want []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{` + "`\"a,b\",c`" + `, []string{"a,b", "c"}},
		{` + "`\"a\"\"b\",c`" + `, []string{` + "`a\"b`" + `, "c"}},
		{"", []string{""}},
		{` + "`\"\"`" + `, []string{""}},
		{"a,", []string{"a", ""}},
	}
	for _, c := range good {
		got, err := ParseCSV(c.in)
		if err != nil {
			t.Fatalf("ParseCSV(%q) unexpected error: %v", c.in, err)
		}
		assertStringSlices(t, got, c.want)
	}
	for _, in := range []string{` + "`a,\"b`" + `, ` + "`\"a\"b\"`" + `} {
		if _, err := ParseCSV(in); err == nil {
			t.Fatalf("ParseCSV(%q) expected error, got nil", in)
		}
	}
}
`

const lastBelowTests = `package task

import "testing"

func TestLastBelow(t *testing.T) {
	cases := []struct {
		xs    []int
		limit int
		want  int
	}{
		{[]int{1, 2, 3}, 2, 0},
		{[]int{}, 0, -1},
		{[]int{5, 1, 4}, 3, 1},
		{[]int{1, 2, 3}, 0, -1},
		{[]int{3, 2, 1}, 10, 2},
	}
	for _, c := range cases {
		if got := LastBelow(c.xs, c.limit); got != c.want {
			t.Fatalf("LastBelow(%v,%d)=%d want %d", c.xs, c.limit, got, c.want)
		}
	}
}
`

const assertStringSlicesHelper = `
func assertStringSlices(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (got %v want %v)", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("at %d got %v want %v", i, got, want)
		}
	}
}
`

func evaluationCases() []evalCase {
	return []evalCase{
		{
			Name:      "safe-divide",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Divide(a, b int) (int, error)",
				Contract: "Integer division truncating toward zero. Division by zero (including 0/0) must return a non-nil error instead of panicking.",
				Tests:    safeDivideTests,
			},
		},
		{
			Name:      "clamp-value",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Clamp(v, lo, hi int) (int, error)",
				Contract: "Return v clamped into [lo, hi] inclusive. If lo > hi the input is invalid and the function must return a non-nil error.",
				Tests:    clampTests,
			},
		},
		{
			Name:      "round-ties-away",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "RoundTiesAway(v float64) (int, error)",
				Contract: "Round to the nearest integer, rounding half away from zero: 0.5 -> 1, -0.5 -> -1, 2.5 -> 3, -2.5 -> -3. NaN and infinities must return a non-nil error.",
				Tests:    roundTiesAwayTests,
			},
		},
		{
			Name:      "rotate-slice",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Rotate(xs []int, k int) []int",
				Contract: "Return a new slice rotated left by k positions. k may be any integer: larger than the length (wrap around), zero (identity), or negative (rotate right). Handle nil and single-element input without panic.",
				Tests:    rotateTests,
			},
		},
		{
			Name:      "atoi-manual",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Atoi(s string) (int, error)",
				Contract: "Parse an integer without using strconv or fmt.Sscan. Support optional leading + or -. Reject: empty input, sign without digits, non-digit characters, whitespace anywhere, decimal points, and overflow (return a non-nil error in every rejected case).",
				Tests:    atoiTests,
			},
		},
		{
			Name:      "chunk-slice",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Chunk(xs []int, n int) ([][]int, error)",
				Contract: "Split xs into consecutive chunks of size n; the final chunk may be smaller. n <= 0 must return a non-nil error. Empty input returns an empty outer slice.",
				Tests:    chunkTests,
			},
		},
		{
			Name:      "lru-cache",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "type LRU struct; func NewLRU(capacity int) *LRU; func (l *LRU) Get(key int) (int, bool); func (l *LRU) Put(key, value int)",
				Contract: "Least-recently-used cache with the exact names above. Get returns the value and refreshes recency. Put evicts the least recently used entry when over capacity. Capacity <= 0 means nothing is ever stored. Only these three functions and the type, nothing else.",
				Tests:    lruTests,
			},
		},
		{
			Name:      "json-path",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Extract(data map[string]any, path string) (any, error)",
				Contract: "Resolve a dotted path like \"a.b.c\" through nested map[string]any values. Missing keys, empty path, empty segments and traversing through non-object values must return a non-nil error.",
				Tests:    jsonPathTests,
			},
		},
		{
			Name:      "count-words",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "CountWords(s string) int",
				Contract: "Count whitespace-separated words. Words are maximal runs of non-whitespace characters; punctuation attached to a word is part of it. Consecutive spaces, tabs, newlines and carriage returns separate words but do not create words. Empty and all-whitespace input return 0.",
				Tests:    countWordsTests,
			},
		},
		{
			Name:      "parse-csv",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "ParseCSV(line string) ([]string, error)",
				Contract: "Split one CSV line into fields. Double quotes wrap fields containing commas; doubled quotes inside a quoted field escape a literal quote. Unterminated quotes and stray quotes inside an unquoted field must return a non-nil error. An empty line is a single empty field.",
				Tests:    parseCSVTests,
			},
		},
		{
			Name:      "merge-intervals",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "Merge(intervals [][2]int) [][2]int",
				Contract: "Merge intervals that strictly overlap (share at least one interior integer point). Intervals that only touch at an endpoint, such as [1,2] and [2,3], must NOT be merged. Handle empty input and unsorted input.",
				Tests:    mergeIntervalsTests,
			},
		},
		{
			Name:      "fix-off-by-one",
			MaxTokens: 2400,
			Code: &codeTask{
				Function: "LastBelow(xs []int, limit int) int",
				Contract: "The buggy version below panics on some inputs: `for i := 0; i <= len(xs); i++ { if xs[i] < limit { last = i } }` with last initialized to -1. Return the corrected function that returns the index of the last element strictly less than limit, or -1 when none or the slice is empty. Do not change the signature.",
				Tests:    lastBelowTests,
			},
		},
	}
}
