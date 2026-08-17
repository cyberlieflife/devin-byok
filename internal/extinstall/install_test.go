package extinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"devin-byok/internal/extinstall"
)

func TestInstallFromFS(t *testing.T) {
	dst, err := extinstall.InstallFromFS(extinstall.ExtFS, extinstall.ExtRoot)
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(dst, "package.json")
	if _, err := os.Stat(pkg); err != nil {
		t.Fatal(err)
	}
	if !extinstall.IsInstalled() {
		t.Fatal("expected installed")
	}
	t.Log("installed at", dst)
}

func TestInstallDevShellUninstall(t *testing.T) {
	dst, err := extinstall.InstallDevShell()
	if err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(dst, "package.json")
	if _, err := os.Stat(pkg); err != nil {
		t.Fatal(err)
	}
	if !extinstall.IsDevShellInstalled() {
		t.Fatal("expected dev shell installed")
	}
	if err := extinstall.EnableDevShell(); err != nil {
		t.Fatal(err)
	}
	if err := extinstall.DisableDevShell(); err != nil {
		t.Fatal(err)
	}
	if err := extinstall.UninstallDevShell(); err != nil {
		t.Fatal(err)
	}
	if extinstall.IsDevShellInstalled() {
		t.Fatal("dev shell should be removed after uninstall")
	}
}
