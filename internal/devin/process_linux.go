//go:build linux

package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func stopDevinProcess() error {
	cmd := exec.Command("pkill", "-f", "Devin")
	if err := cmd.Run(); err != nil {
		// pkill 返回 1 表示无匹配进程，视为已停止
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return nil
		}
		return err
	}
	return nil
}

func startDevinProcess(installDir string) error {
	candidates := []string{}
	if strings.TrimSpace(installDir) != "" {
		candidates = append(candidates, filepath.Join(installDir, "Devin"))
	}
	for _, dir := range platformInstallCandidates() {
		candidates = append(candidates, filepath.Join(dir, "Devin"))
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return exec.Command(candidate).Start()
		}
	}
	return exec.Command("devin").Start()
}

func isDevinProcessRunning() bool {
	out, err := exec.Command("pgrep", "-f", "Devin").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func platformInstallCandidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, "Applications", "Devin"), filepath.Join(home, ".local", "opt", "Devin"))
	}
	return out
}