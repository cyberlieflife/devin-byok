package ideinject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devin-byok/internal/platform"
)

func overrideDataDir(t *testing.T) {
	t.Helper()
	metaDir := t.TempDir()
	old := dataDir
	dataDir = func() string { return metaDir }
	t.Cleanup(func() { dataDir = old })
}

func setupInstall(t *testing.T) (installDir, htmlPath string) {
	t.Helper()
	installDir = t.TempDir()
	htmlPath = platform.SessionsHTMLPath(installDir)
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte("<html><head></head><body></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return installDir, htmlPath
}

func TestApplyContextUsageDonutInjects(t *testing.T) {
	overrideDataDir(t)
	installDir, htmlPath := setupInstall(t)

	if err := ApplyContextUsageDonut(installDir); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(htmlPath)
	if _, err := os.Stat(filepath.Join(dir, scriptName)); err != nil {
		t.Fatalf("js not written: %v", err)
	}
	raw, _ := os.ReadFile(htmlPath)
	html := string(raw)
	if !strings.Contains(html, markerBegin) || !strings.Contains(html, markerEnd) {
		t.Fatalf("markers not injected: %q", html)
	}
	if !strings.Contains(html, `src="./`+scriptName+`"`) {
		t.Fatalf("script tag missing: %q", html)
	}
	if _, err := os.Stat(metaPath()); err != nil {
		t.Fatalf("meta not written: %v", err)
	}
	if n := countBackups(dir); n != 1 {
		t.Fatalf("backup count = %d, want 1", n)
	}
}

func TestApplyContextUsageDonutIdempotent(t *testing.T) {
	overrideDataDir(t)
	installDir, htmlPath := setupInstall(t)

	if err := ApplyContextUsageDonut(installDir); err != nil {
		t.Fatal(err)
	}
	if err := ApplyContextUsageDonut(installDir); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(htmlPath)
	html := string(raw)
	if strings.Count(html, markerBegin) != 1 {
		t.Fatalf("markerBegin count = %d, want 1", strings.Count(html, markerBegin))
	}
	if n := countBackups(filepath.Dir(htmlPath)); n != 1 {
		t.Fatalf("backup count = %d, want 1 (no duplicate backup)", n)
	}
}

func TestRestoreAtRemovesInjection(t *testing.T) {
	overrideDataDir(t)
	installDir, htmlPath := setupInstall(t)

	if err := ApplyContextUsageDonut(installDir); err != nil {
		t.Fatal(err)
	}
	if err := restoreAt(installDir); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(htmlPath)
	if strings.Contains(string(raw), markerBegin) {
		t.Fatalf("marker still present after restore")
	}
	dir := filepath.Dir(htmlPath)
	if _, err := os.Stat(filepath.Join(dir, scriptName)); err == nil {
		t.Fatalf("js still present after restore")
	}
	if _, err := os.Stat(metaPath()); err == nil {
		t.Fatalf("meta still present after restore")
	}
	if n := countBackups(dir); n != 0 {
		t.Fatalf("backup count = %d, want 0 after restore", n)
	}
}

func countBackups(dir string) int {
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sessions.html.bak_byok_ctx_") {
			n++
		}
	}
	return n
}

func TestJSONStringField(t *testing.T) {
	s := `{"install_dir": "/tmp/foo", "html_path": "/tmp/a.html", "js_path": "/tmp/a.js"}`
	cases := map[string]string{
		"install_dir": "/tmp/foo",
		"html_path":   "/tmp/a.html",
		"js_path":     "/tmp/a.js",
	}
	for k, want := range cases {
		if got := jsonStringField(s, k); got != want {
			t.Errorf("jsonStringField(%s) = %q, want %q", k, got, want)
		}
	}
}
