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
	lsPath := filepath.Join(dir, "resources", "app", "extensions", "windsurf", "bin", LanguageServerName())
	realPath := filepath.Join(dir, "resources", "app", "extensions", "windsurf", "bin", RealLanguageServerName())
	_, lsErr := os.Stat(lsPath)
	_, realErr := os.Stat(realPath)
	return lsErr == nil || realErr == nil
}
