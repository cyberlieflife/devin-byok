package devin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureLocalAccountAtCreatesAndReusesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), localAccountFileName)
	random := bytes.NewReader(bytes.Repeat([]byte{0x5a}, 64))
	now := time.Date(2026, time.August, 13, 8, 30, 0, 0, time.UTC)

	first, created, err := ensureLocalAccountAt(path, random, now)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected the first call to create an account")
	}
	if !strings.HasPrefix(first.ID, "byok-local-") {
		t.Fatalf("unexpected id: %q", first.ID)
	}
	if !strings.HasSuffix(first.Email, "@local.invalid") {
		t.Fatalf("unexpected email: %q", first.Email)
	}
	if !strings.HasPrefix(first.APIKey, "sk-ws-01-byok-") {
		t.Fatalf("unexpected api key format: %q", first.APIKey)
	}
	if first.CreatedAt != now.Format(time.RFC3339) {
		t.Fatalf("created_at = %q", first.CreatedAt)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	second, created, err := ensureLocalAccountAt(path, bytes.NewReader(nil), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected the second call to reuse the existing account")
	}
	if *second != *first {
		t.Fatalf("account changed between calls: first=%+v second=%+v", first, second)
	}
}

func TestLoadLocalAccountAtRejectsNonLocalEmail(t *testing.T) {
	path := filepath.Join(t.TempDir(), localAccountFileName)
	data := `{"id":"byok-local-test","name":"BYOK Local","email":"user@example.com","api_key":"sk-ws-01-byok-test","created_at":"2026-08-13T08:30:00Z"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLocalAccountAt(path); err == nil {
		t.Fatal("expected a non-.invalid email to be rejected")
	}
}

func TestLocalAccountImportedPreservesRequiredSettingsContract(t *testing.T) {
	settings := map[string]any{"custom.setting": "keep"}
	if !applyDevKeysToSettings(settings, "/tmp/devin-byok-wrapper") {
		t.Fatal("expected dev keys to be applied")
	}
	if settings["custom.setting"] != "keep" {
		t.Fatal("custom Devin setting was overwritten")
	}
	for key, want := range map[string]any{
		"codeiumDev.languageServerBinaryPath": "/tmp/devin-byok-wrapper",
		"devin.multiTenantMode":                true,
		"devin.cascade.enabled":                true,
		"sync.enableSettings":                   false,
	} {
		if settings[key] != want {
			t.Fatalf("%s = %#v, want %#v", key, settings[key], want)
		}
	}
}
