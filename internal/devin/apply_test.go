package devin

import "testing"

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
	if got := settings["custom.setting"]; got != "keep" {
		t.Fatalf("custom.setting = %#v, want keep", got)
	}

	if applyDevKeysToSettings(settings, "/tmp/devin-byok-ls-wrapper") {
		t.Fatal("expected a second apply to be idempotent")
	}
}
