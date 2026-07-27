package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Prefs GUI/桌面相关偏好（存 APPDATA，不进 config.yaml）。
type Prefs struct {
	// Autostart 开机自启 serve
	Autostart bool `json:"autostart"`
	// StartMinimized GUI 启动后直接进托盘
	StartMinimized bool `json:"start_minimized"`
	// MinimizeToTray 最小化时隐藏到托盘
	MinimizeToTray bool `json:"minimize_to_tray"`
}

func prefsPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "devin-byok", "gui.json")
}

func DefaultPrefs() Prefs {
	return Prefs{
		Autostart:      false,
		StartMinimized: false,
		MinimizeToTray: true,
	}
}

func LoadPrefs() Prefs {
	p := DefaultPrefs()
	b, err := os.ReadFile(prefsPath())
	if err != nil {
		return p
	}
	_ = json.Unmarshal(b, &p)
	return p
}

func SavePrefs(p Prefs) error {
	dir := filepath.Dir(prefsPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(prefsPath(), b, 0o644)
}

// AutostartPath 开机启动脚本路径。
func AutostartPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "devin-byok-serve.cmd")
}

func AutostartEnabled() bool {
	_, err := os.Stat(AutostartPath())
	return err == nil
}

// SetAutostart 写入/删除 Startup 脚本（启动 CLI start，会 apply）。
func SetAutostart(on bool, projectDir, cliExe string) error {
	lnk := AutostartPath()
	if !on {
		_ = os.Remove(lnk)
		return nil
	}
	if strings.TrimSpace(cliExe) == "" {
		return fmt.Errorf("cli exe empty")
	}
	if strings.TrimSpace(projectDir) == "" {
		projectDir = filepath.Dir(cliExe)
	}
	content := fmt.Sprintf("@echo off\r\ncd /d \"%s\"\r\n\"%s\" start\r\n", projectDir, cliExe)
	return os.WriteFile(lnk, []byte(content), 0o644)
}
