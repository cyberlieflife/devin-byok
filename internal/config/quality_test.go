package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"devin-byok/internal/config"
)

func loadQualityConfig(t *testing.T, qualityYAML string) *config.File {
	t.Helper()
	raw := []byte("upstream:\n  base_url: http://127.0.0.1:1/v1\n  api_key: test\n  model: test-model\n" + qualityYAML)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLegacyConfigDefaultsQualityToBalanced(t *testing.T) {
	f := loadQualityConfig(t, "")
	if !f.Quality.Enabled {
		t.Fatal("legacy config should enable quality defaults")
	}
	if got := f.QualityMode(); got != "balanced" {
		t.Fatalf("quality mode=%q, want balanced", got)
	}
	if f.Quality.MaxVerificationRounds != 1 {
		t.Fatalf("verification rounds=%d, want 1", f.Quality.MaxVerificationRounds)
	}
}

func TestExplicitQualityDisabledStaysDisabled(t *testing.T) {
	f := loadQualityConfig(t, "quality:\n  enabled: false\n  mode: verified\n")
	if f.Quality.Enabled {
		t.Fatal("explicit quality.enabled=false was re-enabled")
	}
	if got := f.QualityMode(); got != "fast" {
		t.Fatalf("quality mode=%q, want fast when disabled", got)
	}
}

func TestQualityModeNormalization(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "balanced default", raw: "quality:\n  enabled: true\n", want: "balanced"},
		{name: "strict alias", raw: "quality:\n  enabled: true\n  mode: strict\n", want: "verified"},
		{name: "fast", raw: "quality:\n  enabled: true\n  mode: fast\n", want: "fast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadQualityConfig(t, tc.raw).QualityMode(); got != tc.want {
				t.Fatalf("quality mode=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestGUIPatchCanDisableQuality(t *testing.T) {
	f := loadQualityConfig(t, "")
	value := false
	f.ApplyGUIPatch(config.GUIPatch{QualityEnabled: &value})
	if f.Quality.Enabled {
		t.Fatal("GUI patch did not disable quality")
	}
	if got := f.QualityMode(); got != "fast" {
		t.Fatalf("quality mode=%q, want fast", got)
	}
}
