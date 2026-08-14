package localapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devin-byok/internal/config"
)

func TestOpenAIProxyRoutesByModelFamily(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotModel = payload.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer upstream.Close()

	cfg := &config.File{}
	cfg.Upstream.BaseURL = "http://127.0.0.1:1/v1"
	cfg.Upstream.APIKey = "wrong-global-key"
	cfg.Upstream.Models = []config.ModelEntry{{ID: "grok-medium", FamilyUID: "grok", Family: "Grok", UpstreamModel: "grok-4.5", Thinking: "medium"}}
	cfg.Upstream.Families = []config.FamilyConfig{{UID: "grok", Label: "Grok", Provider: "openai", BaseURL: upstream.URL, APIKey: "family-key", UpstreamModel: "grok-4.5"}}
	s := New(cfg, t.TempDir())

	body := `{"model":"grok-medium","messages":[{"role":"user","content":"hello"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer family-key" || gotModel != "grok-4.5" {
		t.Fatalf("proxy route/auth/model mismatch path=%q auth=%q model=%q", gotPath, gotAuth, gotModel)
	}
	var response map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["choices"] == nil {
		t.Fatalf("unexpected proxy response: %s", rr.Body.String())
	}
}

func TestOpenAIProxyDefaultModelStillRoutesFamily(t *testing.T) {
	var called bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer upstream.Close()
	cfg := &config.File{}
	cfg.Upstream.BaseURL = upstream.URL
	cfg.Upstream.APIKey = "family-key"
	cfg.Upstream.Models = []config.ModelEntry{{ID: "default", UpstreamModel: "default"}}
	s := New(cfg, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Fatalf("default route failed called=%v status=%d body=%s", called, rr.Code, rr.Body.String())
	}
}
