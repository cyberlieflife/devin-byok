package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"devin-byok/internal/config"
)

func TestResolveModelUIDFuzzy(t *testing.T) {
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
auth: {fake_api_key: "k"}
upstream:
  base_url: "http://localhost:8317/v1"
  api_key: "x"
  model: "grok-4.5-byok-medium"
  families:
    - uid: "grok-4.5-byok"
      label: "Grok 4.5"
      provider: openai
      base_url: "http://localhost:8317/v1"
      api_key: "x"
      upstream_model: "grok-4.5"
      context_window: 200000
      max_tokens: 8192
  models:
    - id: "grok-4.5-byok-low"
      label: "Grok 4.5 Low"
      upstream_model: "grok-4.5"
      thinking: "low"
      family: "Grok 4.5"
      family_uid: "grok-4.5-byok"
    - id: "grok-4.5-byok-medium"
      label: "Grok 4.5 Medium"
      upstream_model: "grok-4.5"
      thinking: "medium"
      family: "Grok 4.5"
      family_uid: "grok-4.5-byok"
    - id: "grok-4.5-byok-high"
      label: "Grok 4.5 High"
      upstream_model: "grok-4.5"
      thinking: "high"
      family: "Grok 4.5"
      family_uid: "grok-4.5-byok"
features: {enable_chat: true}
`)
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := cfg.ResolveModelUID("grok4.5")
	if !ok {
		t.Fatal("expected resolve grok4.5")
	}
	if m.ID != "grok-4.5-byok-medium" {
		t.Fatalf("got %s want medium default", m.ID)
	}
	prov, ok := cfg.ResolveProvider(m.ID)
	if !ok || prov.BaseURL == "" || prov.UpstreamModel == "" {
		t.Fatalf("provider resolve fail: %+v ok=%v", prov, ok)
	}
	m2, ok := cfg.ResolveModelUID("grok-4.5-byok-high")
	if !ok || m2.ID != "grok-4.5-byok-high" {
		t.Fatalf("exact high failed: %+v", m2)
	}
}
