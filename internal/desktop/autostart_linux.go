//go:build linux

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Linux 自启通过 XDG autostart desktop entry（~/.config/autostart/）实现。
// 该目录下放置 .desktop 文件即可在登录后自动运行。

func AutostartPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "autostart", "devin-byok-serve.desktop")
}

func AutostartEnabled() bool {
	_, err := os.Stat(AutostartPath())
	return err == nil
}

func SetAutostart(on bool, projectDir, cliExe string) error {
	path := AutostartPath()
	if !on {
		_ = os.Remove(path)
		return nil
	}
	if strings.TrimSpace(cliExe) == "" {
		return fmt.Errorf("cli exe empty")
	}
	if strings.TrimSpace(projectDir) == "" {
		projectDir = filepath.Dir(cliExe)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Devin BYOK\nExec=%s start\nPath=%s\nX-GNOME-Autostart-enabled=true\n", cliExe, projectDir)
	return os.WriteFile(path, []byte(content), 0o644)
}