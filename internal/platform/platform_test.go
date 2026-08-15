package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReleaseInstallDir(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific release layout")
	}
	got := ReleaseInstallDir("/tmp/devin-byok-gui.app/Contents/MacOS/devin-byok-gui")
	if want := "/tmp"; got != want {
		t.Fatalf("ReleaseInstallDir() = %q, want %q", got, want)
	}
	if got := ReleaseInstallDir("/tmp/devin-byok-gui"); got != "/tmp" {
		t.Fatalf("plain executable install dir = %q", got)
	}
}

func TestGUIPathPrefersAppBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific GUI layout")
	}
	root := t.TempDir()
	bundle := filepath.Join(root, GUIBundleName())
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := GUIPath(root); got != bundle {
		t.Fatalf("GUIPath() = %q, want %q", got, bundle)
	}
}

func TestMacInstallDirAcceptsLegacyAppSubdir(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific install layout")
	}
	legacy := "/Applications/Devin.app/Contents/Resources/app"
	want := "/Applications/Devin.app/Contents/Resources/app/extensions/windsurf/bin"
	if got := ExtensionsBinDir(legacy); got != want {
		t.Fatalf("ExtensionsBinDir() = %q, want %q", got, want)
	}
}

func TestMacAssetSuffix(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific release asset")
	}
	want := "darwin-arm64.dmg"
	if runtime.GOARCH != "arm64" {
		// 与 pack-release.sh 产物命名对齐（darwin-amd64.dmg）
		want = "darwin-amd64.dmg"
	}
	if got := AssetSuffix(); got != want {
		t.Fatalf("AssetSuffix() = %q, want %q", got, want)
	}
}
