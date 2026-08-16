//go:build windows

package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// hiddenProcAttr 隐藏子进程控制台窗口：GUI 程序无控制台，
// 直接 exec 控制台程序（taskkill/tasklist/cmd）会闪现终端窗口。
func hiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

func stopDevinProcess() error {
	cmd := exec.Command("taskkill", "/IM", "Devin.exe", "/T", "/F")
	cmd.SysProcAttr = hiddenProcAttr()
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
			cmd := exec.Command(candidate)
			cmd.SysProcAttr = hiddenProcAttr()
			return cmd.Start()
		}
	}
	cmd := exec.Command("cmd", "/c", "start", "", "Devin")
	cmd.SysProcAttr = hiddenProcAttr()
	return cmd.Start()
}

func isDevinProcessRunning() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq Devin.exe", "/NH")
	cmd.SysProcAttr = hiddenProcAttr()
	out, err := cmd.Output()
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
