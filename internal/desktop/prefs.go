package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"

	"devin-byok/internal/platform"
)

// Prefs GUI 偏好（存平台数据目录，不进 config.yaml）。
type Prefs struct {
	// Autostart 开机自启 serve
	Autostart bool `json:"autostart"`
	// StartMinimized GUI 启动后直接进托盘
	StartMinimized bool `json:"start_minimized"`
	// MinimizeToTray 最小化时隐藏到托盘
	MinimizeToTray bool `json:"minimize_to_tray"`
}

func prefsPath() string {
	return filepath.Join(platform.DataDir(), "gui.json")
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
