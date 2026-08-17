package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"devin-byok/internal/platform"
)

// Devin ACP agent（devin.exe）包装器：
// Devin 扩展通过 ACP（stdio）启动 devin.exe acp，并在 authenticate meta 里
// 传入官方 api_server_url（getConfig 走官方默认，无法用 settings 覆盖）。
// 本包装器注入 WINDSURF_API_SERVER_URL 环境变量（devin.exe 内部优先于
// ACP meta 的 api_server_url，已实测），把 Agent/ACP 通道的 API 请求全部
// 指回本地兼容层，从而模型列表/账号/会话走本地 stub。
func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "devin-wrapper: executable: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Dir(exe)
	real := filepath.Join(dir, platform.RealDevinExeName())
	if _, err := os.Stat(real); err != nil {
		fmt.Fprintf(os.Stderr, "devin-wrapper: missing real binary: %s\n", real)
		os.Exit(1)
	}

	api := os.Getenv("DEVIN_BYOK_API_SERVER_URL")
	if api == "" {
		api = "http://127.0.0.1:8787"
	}

	// 记录启动参数/覆盖目标，便于诊断 ACP 通道。
	logPath := filepath.Join(platform.DataDir(), "devin-wrapper-last.json")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	_ = os.WriteFile(logPath, []byte(fmt.Sprintf(`{"real":%q,"api":%q,"args":%q}`, real, api, joinArgs(os.Args[1:]))), 0o644)

	cmd := exec.Command(real, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "WINDSURF_API_SERVER_URL="+api)
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "devin-wrapper: run real: %v\n", err)
		os.Exit(1)
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
