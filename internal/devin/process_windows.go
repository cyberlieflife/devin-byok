//go:build windows

package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func stopDevinProcess() error {
	cmd := exec.Command("taskkill", "/IM", "Devin.exe", "/T", "/F")
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 128 {
			return nil
		}
		return err
	}
	return nil
}

func startDevinProcess(installDir string) error {
	candidates := []string{}
	if strings.TrimSpace(installDir) != "" {
		candidates = append(candidates, filepath.Join(installDir, "Devin.exe"))
	}
	for _, dir := range platformInstallCandidates() {
		candidates = append(candidates, filepath.Join(dir, "Devin.exe"))
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return exec.Command(candidate).Start()
		}
	}
	return exec.Command("cmd", "/c", "start", "", "Devin").Start()
}

func isDevinProcessRunning() bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq Devin.exe", "/NH").Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "devin.exe")
}

func platformInstallCandidates() []string {
	var out []string
	if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
		out = append(out, filepath.Join(local, "Programs", "Devin"), filepath.Join(local, "Devin"))
	}
	if pf := strings.TrimSpace(os.Getenv("ProgramFiles")); pf != "" {
		out = append(out, filepath.Join(pf, "Devin"))
	}
	return out
}
