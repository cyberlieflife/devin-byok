//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

func dataDir() string {
	home := UserHomeDir()
	if home == "" {
		return ".devin-byok"
	}
	return filepath.Join(home, "Library", "Application Support", "devin-byok")
}

func devinDataDirs() []string {
	var out []string
	home := UserHomeDir()
	cands := []string{
		filepath.Join(home, "Library", "Application Support", "Devin"),
		filepath.Join(home, ".devin"),
		filepath.Join(home, "Library", "Application Support", "Windsurf"),
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
	var cands []string
	home := UserHomeDir()
	paths := []string{
		"/Applications/Devin.app",
		filepath.Join(home, "Applications", "Devin.app"),
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			cands = append(cands, p)
		}
	}
	if v := os.Getenv("DEVIN_INSTALL_DIR"); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			cands = append([]string{v}, cands...)
		}
	}
	return cands
}

func languageServerName() string {
	return "language_server_macos_arm"
}

func wrapperExeName() string {
	return "devin-byok-ls-wrapper"
}

func guiName() string {
	return "devin-byok-gui"
}

func assetSuffix() string {
	switch runtime.GOARCH {
	case "arm64":
		return "darwin-arm64.zip"
	default:
		return "darwin-x64.zip"
	}
}

func killCommand(pid string) []string {
	return []string{"kill", pid}
}

func defaultInstallDir() string {
	if st, err := os.Stat("/Applications/Devin.app"); err == nil && st.IsDir() {
		return "/Applications/Devin.app"
	}
	return ""
}

func extensionsBinSubPath() string {
	return filepath.Join("Contents", "Resources", "app", "extensions", "windsurf", "bin")
}

func sessionsHTMLSubPath() string {
	return filepath.Join("Contents", "Resources", "app", "out", "vs", "sessions", "electron-browser", "sessions.html")
}
