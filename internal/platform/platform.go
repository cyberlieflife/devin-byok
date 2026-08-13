package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

func IsWindows() bool { return runtime.GOOS == "windows" }

func IsDarwin() bool { return runtime.GOOS == "darwin" }

func UserHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return ""
}

func DataDir() string {
	return dataDir()
}

func DevinDataDirs() []string {
	return devinDataDirs()
}

func DevinInstallCandidates() []string {
	return devinInstallCandidates()
}

func LanguageServerName() string {
	return languageServerName()
}

func RealLanguageServerName() string {
	return languageServerName() + ".real"
}

func WrapperExeName() string {
	return wrapperExeName()
}

func GUIName() string {
	return guiName()
}

func GUIBundleName() string {
	return guiBundleName()
}

func GUIPath(root string) string {
	if bundle := GUIBundleName(); bundle != "" {
		candidate := filepath.Join(root, bundle)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		// Accept the legacy bundle name during upgrades.
		legacy := filepath.Join(root, "devin-byok-gui.app")
		if st, err := os.Stat(legacy); err == nil && st.IsDir() {
			return legacy
		}
	}
	return filepath.Join(root, GUIName())
}

func CLIName() string {
	return cliName()
}

// BundledCLIPath returns the CLI location used by desktop autostart when a
// release ships as a single GUI artifact.
func BundledCLIPath(exe string) string {
	return bundledCLIPath(exe)
}

// ReleaseInstallDir returns the directory used by the updater.
// For a macOS .app executable this is the directory containing the bundle.
func ReleaseInstallDir(exe string) string {
	return releaseInstallDir(exe)
}

func AssetSuffix() string {
	return assetSuffix()
}

func KillCommand(pid string) []string {
	return killCommand(pid)
}

func DefaultInstallDir() string {
	return defaultInstallDir()
}

func IsValidInstallDir(dir string) bool {
	if dir == "" {
		return false
	}
	dir = normalizeInstallDir(dir)
	lsPath := filepath.Join(dir, extensionsBinSubPath(), LanguageServerName())
	realPath := filepath.Join(dir, extensionsBinSubPath(), RealLanguageServerName())
	_, lsErr := os.Stat(lsPath)
	_, realErr := os.Stat(realPath)
	return lsErr == nil || realErr == nil
}

func ExtensionsBinDir(installDir string) string {
	return filepath.Join(normalizeInstallDir(installDir), extensionsBinSubPath())
}

func SessionsHTMLPath(installDir string) string {
	return filepath.Join(normalizeInstallDir(installDir), sessionsHTMLSubPath())
}
