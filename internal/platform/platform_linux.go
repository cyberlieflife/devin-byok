//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// Linux 是开发/CI 支持的辅助平台（官方发布物为 Windows 与 macOS）。
// 数据目录遵循 XDG；Devin 桌面应用在 Linux 上无官方安装布局，
// 相关函数返回合理默认值以保证其余代码路径可编译、可测试。

func dataDir() string {
	if x := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); x != "" {
		return filepath.Join(x, "devin-byok")
	}
	if home := UserHomeDir(); home != "" {
		return filepath.Join(home, ".local", "share", "devin-byok")
	}
	return ".devin-byok"
}

func devinDataDirs() []string {
	var out []string
	home := UserHomeDir()
	if home == "" {
		return out
	}
	cands := []string{
		filepath.Join(home, ".local", "share", "Devin"),
		filepath.Join(home, ".devin"),
		filepath.Join(home, ".config", "Windsurf"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

func defaultDevinDataDir() string {
	home := UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "Devin")
}

func devinInstallCandidates() []string {
	var cands []string
	if v := strings.TrimSpace(os.Getenv("DEVIN_INSTALL_DIR")); v != "" {
		cands = append(cands, v)
	}
	home := UserHomeDir()
	if home != "" {
		cands = append(cands,
			filepath.Join(home, "Applications", "Devin"),
			filepath.Join(home, ".local", "opt", "Devin"),
		)
	}
	return cands
}

func languageServerName() string {
	return "language_server_linux_x64"
}

func wrapperExeName() string {
	return "devin-byok-ls-wrapper"
}

func guiName() string {
	return "devin-byok-gui"
}

func guiBundleName() string {
	return ""
}

func cliName() string {
	return "devin-byok"
}

func bundledCLIPath(exe string) string {
	return filepath.Join(filepath.Dir(exe), cliName())
}

func assetSuffix() string {
	return "linux-amd64.tar.gz"
}

func killCommand(pid string) []string {
	return []string{"kill", pid}
}

func defaultInstallDir() string {
	if v := strings.TrimSpace(os.Getenv("DEVIN_INSTALL_DIR")); v != "" {
		return normalizeInstallDir(v)
	}
	for _, candidate := range devinInstallCandidates() {
		if IsValidInstallDir(candidate) {
			return normalizeInstallDir(candidate)
		}
	}
	return ""
}

func normalizeInstallDir(dir string) string {
	return strings.TrimSpace(dir)
}

func releaseInstallDir(exe string) string {
	return filepath.Dir(exe)
}

func extensionsBinSubPath() string {
	return filepath.Join("resources", "app", "extensions", "windsurf", "bin")
}

func sessionsHTMLSubPath() string {
	return filepath.Join("resources", "app", "out", "vs", "sessions", "electron-browser", "sessions.html")
}
