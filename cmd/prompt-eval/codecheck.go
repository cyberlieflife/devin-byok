package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// codeTask is a programming problem verified by running the model's code
// against a fixed, pre-written test file. The model can only return code;
// the evaluator supplies the tests, so the model cannot weaken them.
type codeTask struct {
	Function string // function signature description shown to the model
	Contract string // edge-case contract shown to the model
	Tests    string // fixed test file content (package task)
}

var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z0-9]*[ \t]*\n?(.*?)```")
var packageRe = regexp.MustCompile("(?m)^\\s*package\\s+\\w+.*$")

// extractGoCode pulls the first fenced code block, falling back to the whole
// answer, and normalizes the package clause to package task.
func extractGoCode(answer string) string {
	code := ""
	if m := fenceRe.FindStringSubmatch(answer); m != nil {
		code = m[1]
	} else {
		code = answer
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if packageRe.MatchString(code) {
		code = packageRe.ReplaceAllString(code, "package task")
	} else {
		code = "package task\n" + code
	}
	return code
}

// run compiles the model code against the fixed tests and returns
// passed/total test counts and whether the package compiled.
func (ct *codeTask) run(answer string) (pass, total int, compiled bool) {
	code := extractGoCode(answer)
	if !strings.Contains(code, "package task") {
		return 0, 0, false
	}
	dir, err := os.MkdirTemp("", "prompt-eval-*")
	if err != nil {
		return 0, 0, false
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module eval\n\ngo 1.22\n"), 0o644); err != nil {
		return 0, 0, false
	}
	if err := os.WriteFile(filepath.Join(dir, "task.go"), []byte(code), 0o644); err != nil {
		return 0, 0, false
	}
	if err := os.WriteFile(filepath.Join(dir, "task_test.go"), []byte(ct.Tests), 0o644); err != nil {
		return 0, 0, false
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return 0, 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// 沙箱化：模型返回的代码可能含 init()/os/exec/net/http 等副作用，
	// 以当前用户权限编译执行有风险。这里限制网络与模块解析：
	//   - GOPROXY=off / GOSUMDB=off：禁止拉取任何远程依赖
	//   - GOFLAGS=-mod=readonly：禁止代码改写 go.mod/go.sum
	//   - -run 固定评测方测试名：只运行我们提供的测试，忽略模型可能
	//     注入的其它测试/示例
	cmd := exec.CommandContext(ctx, goBin, "test", "-json", "-count=1", "-run", "^Test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOFLAGS=-mod=readonly",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GONOSUMDB=*",
		"GONOSUMCHECK=1",
	)
	out, err := cmd.Output()
	_ = err
	type event struct {
		Action string
		Test   string
	}
	started := false
	for _, line := range strings.Split(string(out), "\n") {
		var ev event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "run":
			started = true
		case "pass":
			total++
			pass++
		case "fail":
			total++
		}
	}
	return pass, total, started && total > 0
}

// scoreCode scores a code task. Correctness and completeness come from real
// test results; the model claiming it ran tests without tools is a hallucination.
func scoreCode(ct codeTask, answer string) dimensions {
	pass, total, compiled := ct.run(answer)
	d := dimensions{Evidence: 4, Hallucination: 4}
	if total > 0 {
		d.Correctness = int(math.Round(4 * float64(pass) / float64(total)))
		switch {
		case pass == total:
			d.Completeness = 4
		case pass*2 >= total:
			d.Completeness = 2
		}
	}
	// Understanding 不再只由"能否编译"决定：签名正确但逻辑全错的实现
	// 也编译通过，若直接给 4 分则维度区分度趋零。按真实测试通过率分级：
	//   编译 + 全过 = 4；编译 + 部分过 = 2~3；编译但全挂 = 1。
	if compiled {
		switch {
		case total > 0 && pass == total:
			d.Understanding = 4
		case total > 0 && pass > 0:
			d.Understanding = 3
		case total > 0:
			d.Understanding = 1
		default:
			d.Understanding = 2 // 编译通过但无测试被运行（如测试文件未生效）
		}
	}
	lower := strings.ToLower(strings.TrimSpace(answer))
	for _, phrase := range []string{"i ran", "test passed", "测试已通过", "all tests pass", "tests pass"} {
		if strings.Contains(lower, phrase) {
			d.Hallucination = 0
		}
	}
	return d
}

// assertSlice is the shared comparison helper used by fixed test files to
// accept nil and empty slices as equal.
const assertSliceHelper = `
func assertSlices(t *testing.T, got, want []int) {
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
