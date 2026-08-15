//go:build darwin

package devin

import (
	"os"
	"os/exec"
	"strings"
)

func stopDevinProcess() error {
	cmd := exec.Command("pkill", "-x", "Devin")
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return nil
		}
		return err
	}
	return nil
}

func startDevinProcess(installDir string) error {
	if strings.TrimSpace(installDir) != "" {
		if st, err := os.Stat(installDir); err == nil && st.IsDir() {
			return exec.Command("open", installDir).Start()
		}
	}
	return exec.Command("open", "-a", "Devin").Start()
}

func isDevinProcessRunning() bool {
	return exec.Command("pgrep", "-x", "Devin").Run() == nil
}
