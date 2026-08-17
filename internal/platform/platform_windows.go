//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func dataDir() string {
	if app := os.Getenv("APPDATA"); app != "" {
		return filepath.Join(app, "devin-byok")
	}
	if home := UserHomeDir(); home != "" {
		return filepath.Join(home, ".devin-byok")
	}
	return ".devin-byok"
}

func devinDataDirs() []string {
	var out []string
	appdata := os.Getenv("APPDATA")
	home := UserHomeDir()
	cands := []string{
		filepath.Join(appdata, "Devin"),
		filepath.Join(home, ".devin"),
		filepath.Join(appdata, "Windsurf"),
		filepath.Join(home, ".windsurf"),
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

func defaultDevinDataDir() string {
	if appdata := strings.TrimSpace(os.Getenv("APPDATA")); appdata != "" {
		return filepath.Join(appdata, "Devin")
	}
	if home := UserHomeDir(); home != "" {
		return filepath.Join(home, ".devin")
	}
	return ""
}

func devinInstallCandidates() []string {
	cands := []string{}
	if v := os.Getenv("DEVIN_INSTALL_DIR"); v != "" {
		cands = append(cands, v)
	}
	if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
		cands = append(cands, filepath.Join(local, "Programs", "Devin"), filepath.Join(local, "Devin"))
	}
	if pf := strings.TrimSpace(os.Getenv("ProgramFiles")); pf != "" {
		cands = append(cands, filepath.Join(pf, "Devin"))
	}
	if pf86 := strings.TrimSpace(os.Getenv("ProgramFiles(x86)")); pf86 != "" {
		cands = append(cands, filepath.Join(pf86, "Devin"))
	}
	cands = append(cands, `C:\Program Files\Devin`, `C:\Program Files (x86)\Devin`, `D:\Devin`)
	return cands
}

func languageServerName() string {
	// 这是 Devin 客户端自带的 language server 文件名，随 Devin 安装架构而定：
	// arm64 Windows 上为原生 arm64 版，其余架构（含 x64）为 x64 版。
	switch runtime.GOARCH {
	case "arm64":
		return "language_server_windows_arm64.exe"
	default:
		return "language_server_windows_x64.exe"
	}
}

func wrapperExeName() string {
	return "devin-byok-ls-wrapper.exe"
}

func devinExeName() string {
	return "devin.exe"
}

func guiName() string {
	return "devin-byok-gui.exe"
}

func guiBundleName() string {
	return ""
}

func cliName() string {
	return "devin-byok.exe"
}

func bundledCLIPath(exe string) string {
	// The Windows release is a single GUI executable; it starts the embedded
	// service itself and therefore is also the autostart target.
	return exe
}

func assetSuffix() string {
	// 命名必须与 scripts/pack-release.ps1 的产物一致（devin-byok-<ver>-windows-<GOARCH>.exe），
	// 否则在线更新器匹配不到对应架构的安装包。
	switch runtime.GOARCH {
	case "arm64":
		return "windows-arm64.exe"
	default:
		return "windows-amd64.exe"
	}
}

func killCommand(pid string) []string {
	return []string{"taskkill", "/F", "/PID", pid}
}

func defaultInstallDir() string {
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
