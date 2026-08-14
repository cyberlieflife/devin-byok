//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func defaultDevinDataDir() string {
	home := UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Devin")
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
			cands = append([]string{normalizeInstallDir(v)}, cands...)
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

func guiBundleName() string {
	return "Devin BYOK.app"
}

func cliName() string {
	return "devin-byok"
}

func bundledCLIPath(exe string) string {
	exe = filepath.Clean(exe)
	if filepath.Base(filepath.Dir(exe)) == "MacOS" && filepath.Base(filepath.Dir(filepath.Dir(exe))) == "Contents" {
		// The GUI owns the embedded API, so a LaunchAgent can start its
		// executable directly without a separate CLI file.
		return exe
	}
	return filepath.Join(filepath.Dir(exe), cliName())
}

func assetSuffix() string {
	switch runtime.GOARCH {
	case "arm64":
		return "darwin-arm64.dmg"
	default:
		return "darwin-x64.dmg"
	}
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
			return candidate
		}
	}
	return ""
}

func normalizeInstallDir(dir string) string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	legacySuffix := string(os.PathSeparator) + filepath.Join("Contents", "Resources", "app")
	if strings.HasSuffix(dir, legacySuffix) {
		return strings.TrimSuffix(dir, legacySuffix)
	}
	return dir
}

func releaseInstallDir(exe string) string {
	exe = filepath.Clean(exe)
	macOSDir := filepath.Dir(exe)
	contentsDir := filepath.Dir(macOSDir)
	appDir := filepath.Dir(contentsDir)
	if filepath.Base(contentsDir) == "Contents" && strings.HasSuffix(filepath.Base(appDir), ".app") {
		return filepath.Dir(appDir)
	}
	return filepath.Dir(exe)
}

func extensionsBinSubPath() string {
	return filepath.Join("Contents", "Resources", "app", "extensions", "windsurf", "bin")
}

func sessionsHTMLSubPath() string {
	return filepath.Join("Contents", "Resources", "app", "out", "vs", "sessions", "electron-browser", "sessions.html")
}
