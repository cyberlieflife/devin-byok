package devin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyDevKeysToSettingsEnablesCascade(t *testing.T) {
	settings := map[string]any{
		"devin.cascade.enabled": false,
		"custom.setting":        "keep",
	}

	if !applyDevKeysToSettings(settings, "/tmp/devin-byok-ls-wrapper") {
		t.Fatal("expected settings to change")
	}
	if got, ok := settings["devin.cascade.enabled"].(bool); !ok || !got {
		t.Fatalf("cascade.enabled = %#v, want true", settings["devin.cascade.enabled"])
	}
	if got, ok := settings["security.workspace.trust.enabled"].(bool); ok && got {
		t.Fatalf("workspace trust must be disabled for ACP agents in untrusted workspaces")
	}
	if got := settings["custom.setting"]; got != "keep" {
		t.Fatalf("custom.setting = %#v, want keep", got)
	}

	if applyDevKeysToSettings(settings, "/tmp/devin-byok-ls-wrapper") {
		t.Fatal("expected a second apply to be idempotent")
	}
}

func TestApplySettingsAtIsIdempotentAndPreservesOriginalValues(t *testing.T) {
	dir := t.TempDir()
	paths := &Paths{SettingsJSON: filepath.Join(dir, "User", "settings.json")}
	if err := os.MkdirAll(filepath.Dir(paths.SettingsJSON), 0o755); err != nil {
		t.Fatal(err)
	}
	original := map[string]any{"devin.portalUrl": "https://original.example", "custom.setting": "keep"}
	if err := saveSettings(paths.SettingsJSON, original); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(dir, "last-apply.json")
	keys := []string{"devin.portalUrl", "devin.cascade.enabled"}
	firstValues := map[string]any{"devin.portalUrl": "http://127.0.0.1:8787", "devin.cascade.enabled": true}
	if _, err := applySettingsAt(paths, "http://127.0.0.1:8787", "http://127.0.0.1:8787/_route/api_server", keys, firstValues, metaPath); err != nil {
		t.Fatal(err)
	}
	secondValues := map[string]any{"devin.portalUrl": "http://127.0.0.1:9999", "devin.cascade.enabled": true}
	if _, err := applySettingsAt(paths, "http://127.0.0.1:9999", "http://127.0.0.1:9999/_route/api_server", keys, secondValues, metaPath); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Old map[string]any `json:"old_settings"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Old["devin.portalUrl"] != "https://original.example" || meta.Old["devin.cascade.enabled"] != nil {
		t.Fatalf("original values were overwritten: %#v", meta.Old)
	}
	settings, err := loadSettings(paths.SettingsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if settings["devin.portalUrl"] != "http://127.0.0.1:9999" || settings["custom.setting"] != "keep" {
		t.Fatalf("unexpected applied settings: %#v", settings)
	}
}

func TestMergeSettingKeysDeduplicatesAndKeepsOrder(t *testing.T) {
	got := mergeSettingKeys([]string{"a", "b"}, []string{"b", "c", ""})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("keys = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys = %v, want %v", got, want)
		}
	}
}
