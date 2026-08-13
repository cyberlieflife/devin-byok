//go:build windows

package platform

import (
	"os"
	"path/filepath"
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

func devinInstallCandidates() []string {
	cands := []string{`D:\Devin`, `C:\Program Files\Devin`, `C:\Program Files (x86)\Devin`}
	if v := os.Getenv("DEVIN_INSTALL_DIR"); v != "" {
		cands = append([]string{v}, cands...)
	}
	return cands
}

func languageServerName() string {
	return "language_server_windows_x64.exe"
}

func wrapperExeName() string {
	return "devin-byok-ls-wrapper.exe"
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
	return "windows-amd64.exe"
}

func killCommand(pid string) []string {
	return []string{"taskkill", "/F", "/PID", pid}
}

func defaultInstallDir() string {
	return `D:\Devin`
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
