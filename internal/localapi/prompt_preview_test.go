package localapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devin-byok/internal/config"
)

func TestPromptPreviewMatchesQualityAndRoute(t *testing.T) {
	cfg := &config.File{}
	cfg.Upstream.Model = "model-medium"
	cfg.Upstream.Models = []config.ModelEntry{{
		ID: "model-medium", FamilyUID: "family-a", Family: "Family A", UpstreamModel: "upstream-model", Thinking: "medium",
	}}
	cfg.Quality.Enabled = true
	cfg.Quality.Mode = "balanced"
	s := &Server{cfg: cfg}

	req := httptest.NewRequest(http.MethodGet, "/api/prompts/preview?route=chat&model=model-medium&task=coding&quality_mode=verified", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		OK          bool     `json:"ok"`
		Route       string   `json:"route"`
		Task        string   `json:"task"`
		QualityMode string   `json:"quality_mode"`
		Profiles    []string `json:"profile_ids"`
		Hash        string   `json:"prompt_hash"`
		Messages    []struct {
			Role    string `json:"role"`
			Preview string `json:"preview"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Route != "chat" || got.Task != "coding" || got.QualityMode != "verified" {
		t.Fatalf("unexpected preview metadata: %+v", got)
	}
	for _, want := range []string{"core-reliability", "coding-execution", "verification", "output-contract"} {
		found := false
		for _, id := range got.Profiles {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("profile %q missing: %v", want, got.Profiles)
		}
	}
	if !strings.HasPrefix(got.Hash, "sha256:") || len(got.Messages) == 0 {
		t.Fatalf("missing hash/messages: %+v", got)
	}
	if strings.Contains(rr.Body.String(), "api_key") {
		t.Fatal("preview should not expose api key fields")
	}
}

func TestFastContextPreviewOnlyUsesShortProfile(t *testing.T) {
	cfg := &config.File{}
	cfg.Upstream.Model = "model"
	cfg.Upstream.Models = []config.ModelEntry{{ID: "model", UpstreamModel: "model"}}
	cfg.Quality.Enabled = true
	s := &Server{cfg: cfg}
	req := httptest.NewRequest(http.MethodGet, "/api/prompts/preview?route=fast_context&quality_mode=verified", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		QualityMode string   `json:"quality_mode"`
		Profiles    []string `json:"profile_ids"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.QualityMode != "fast" || len(got.Profiles) != 1 || got.Profiles[0] != "fast-context" {
		t.Fatalf("unexpected fast preview: %+v", got)
	}
}
