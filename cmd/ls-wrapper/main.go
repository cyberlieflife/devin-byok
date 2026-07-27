package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// language_server 包装器：强制把 api/inference URL 指到本地兼容层。
func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wrapper: executable: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Dir(exe)
	real := filepath.Join(dir, "language_server_windows_x64.real.exe")
	if _, err := os.Stat(real); err != nil {
		fmt.Fprintf(os.Stderr, "wrapper: missing real binary: %s\n", real)
		os.Exit(1)
	}

	api := strings.TrimRight(os.Getenv("DEVIN_BYOK_API_SERVER_URL"), "/")
	if api == "" {
		api = "http://127.0.0.1:8787/_route/api_server"
	}
	inf := strings.TrimRight(os.Getenv("DEVIN_BYOK_INFERENCE_API_SERVER_URL"), "/")
	if inf == "" {
		inf = api
	}

	args := rewriteArgs(os.Args[1:], api, inf)

	// 记录启动参数，便于验证
	logPath := filepath.Join(os.Getenv("APPDATA"), "devin-byok", "ls-wrapper-last.json")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf(`{"real":%q,"api":%q,"inference":%q,"args":%q}`, real, api, inf, strings.Join(args, " "))), 0o644)

	cmd := exec.Command(real, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	// Windows: 同控制台组
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "wrapper: run real: %v\n", err)
		os.Exit(1)
	}
}

func rewriteArgs(in []string, api, inf string) []string {
	out := make([]string, 0, len(in)+4)
	skipNext := false
	seenAPI, seenInf := false, false
	for i := 0; i < len(in); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		a := in[i]
		switch a {
		case "--api_server_url":
			out = append(out, a, api)
			seenAPI = true
			if i+1 < len(in) {
				skipNext = true
			}
		case "--inference_api_server_url":
			out = append(out, a, inf)
			seenInf = true
			if i+1 < len(in) {
				skipNext = true
			}
		default:
			// 兼容 --api_server_url=xxx
			if strings.HasPrefix(a, "--api_server_url=") {
				out = append(out, "--api_server_url", api)
				seenAPI = true
				continue
			}
			if strings.HasPrefix(a, "--inference_api_server_url=") {
				out = append(out, "--inference_api_server_url", inf)
				seenInf = true
				continue
			}
			out = append(out, a)
		}
	}
	if !seenAPI {
		out = append(out, "--api_server_url", api)
	}
	if !seenInf {
		out = append(out, "--inference_api_server_url", inf)
	}
	return out
}
