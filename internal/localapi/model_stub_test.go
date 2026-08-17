package localapi

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"devin-byok/internal/config"
	"devin-byok/internal/pbwire"
)

func testServer(pureLocal bool) *Server {
	cfg := &config.File{}
	cfg.Upstream.Model = "grok-4.5"
	cfg.Upstream.BaseURL = "http://localhost:8317/v1"
	cfg.Upstream.Models = []config.ModelEntry{{ID: "grok-4.5", Label: "BYOK - grok-4.5", UpstreamModel: "grok-4.5"}}
	cfg.Auth.FakeName = "BYOK Local"
	cfg.Auth.FakeEmail = "byok@local"
	cfg.Auth.FakeAPIKey = "devin-byok-local-key"
	cfg.Features.PureLocal = pureLocal
	cfg.Features.EnableStream = true
	return &Server{cfg: cfg}
}

func TestLocalModelStubsContainUpstreamModel(t *testing.T) {
	cfg := &config.File{}
	cfg.Upstream.Model = "grok-4.5"
	cfg.Upstream.BaseURL = "http://localhost:8317/v1"
	cfg.Upstream.Models = []config.ModelEntry{
		{ID: "grok-4.5", Label: "BYOK - grok-4.5"},
		{ID: "gpt-4.1-mini", Label: "BYOK - gpt-4.1-mini"},
	}
	cfg.Auth.FakeName = "BYOK Local"
	cfg.Auth.FakeEmail = "byok@local"
	cfg.Auth.FakeAPIKey = "devin-byok-local-key"

	checks := map[string][]byte{
		"GetUserStatus":          buildGetUserStatusResponse(cfg),
		"GetCommandModelConfigs": buildGetCommandModelConfigsResponse(cfg),
		"GetCascadeModelConfigs": buildGetCascadeModelConfigsResponse(cfg),
		"GetModelStatuses":       buildGetModelStatusesResponse(cfg),
		"GetUserJwt":             buildGetUserJwtResponse(cfg),
		"RateLimit":              buildCheckUserMessageRateLimitResponse(),
	}
	for name, body := range checks {
		if name != "RateLimit" && name != "GetUserJwt" && !bytes.Contains(body, []byte("grok-4.5")) {
			t.Fatalf("%s missing model uid, len=%d", name, len(body))
		}
		if name == "GetCommandModelConfigs" && !bytes.Contains(body, []byte("gpt-4.1-mini")) {
			t.Fatalf("multi-model missing gpt-4.1-mini")
		}
		if len(body) < 8 {
			t.Fatalf("%s too short: %d", name, len(body))
		}
		t.Logf("%s bytes=%d", name, len(body))
	}

	jwt := buildLocalUserJWT(cfg)
	parts := strings.Split(jwt, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4))
		if err != nil {
			t.Fatalf("decode jwt payload: %v", err)
		}
	}
	if !bytes.Contains(payload, []byte("TEAMS_TIER_DEVIN_PRO")) {
		t.Fatalf("jwt payload missing DEVIN_PRO: %s", payload)
	}
	if !bytes.Contains(payload, []byte(`"pro":true`)) {
		t.Fatalf("jwt payload missing pro:true: %s", payload)
	}

	// 仅单机：任何模式都不透传官方
	s := testServer(false)
	if s.shouldProxyOfficial("exa.auth_pb.AuthService/GetUserJwt") {
		t.Fatal("single-node mode must not proxy GetUserJwt")
	}
	if s.shouldProxyOfficial("exa.seat_management_pb.SeatManagementService/GetUserStatus") {
		t.Fatal("GetUserStatus must stay local")
	}
	s2 := testServer(true)
	if s2.shouldProxyOfficial("exa.auth_pb.AuthService/GetUserJwt") {
		t.Fatal("pure_local GetUserJwt must not proxy")
	}
	if s2.shouldProxyOfficial("exa.cascade_plugins_pb.CascadePluginsService/GetAllAcpRegistries") {
		t.Fatal("pure_local should not proxy anything")
	}
}

func TestPureLocalNeverProxyChat(t *testing.T) {
	for _, pure := range []bool{true, false} {
		s := testServer(pure)
		body := []byte("tool find_code_context Instant Context agent MODEL_CHAT_GPT_4_1_MINI")
		if s.shouldProxyChatOfficial("exa.api_server_pb.ApiServerService/GetChatMessage", body) {
			t.Fatalf("pure=%v must never proxy chat (single-node only)", pure)
		}
		if s.shouldProxyOfficial("exa.auth_pb.AuthService/GetUserJwt") {
			t.Fatalf("pure=%v must never proxy auth", pure)
		}
	}
}

func TestFastContextDetection(t *testing.T) {
	plain := []byte("find_code_context Instant Context agent search")
	parsed := parseGetChatMessageRequest(plain)
	if !isFastContextChat(parsed, "find code context for architecture", plain) {
		t.Fatal("expected fast context detection")
	}
	if !looksLikeOfficialModelEnum([]byte("MODEL_CHAT_GPT_4_1_MINI_2025_04_14")) {
		t.Fatal("expected official model enum detection")
	}
}

func TestTeamConfigDisablesFastContextFalse(t *testing.T) {
	cfg := &config.File{}
	tc := buildTeamConfig(cfg)
	// field 33 bool false still encodes; at least message non-empty and contains team id
	if !bytes.Contains(tc, []byte("byok-local-team")) {
		t.Fatalf("team config missing team id: %v", tc)
	}
	us := buildGetUserStatusResponse(cfg)
	if !bytes.Contains(us, []byte("byok-local-team")) {
		t.Fatal("GetUserStatus should embed team_config")
	}
	if !bytes.Contains(us, []byte("Devin BYOK Pro")) {
		t.Fatal("GetUserStatus should spoof Pro plan name")
	}
}

func TestProfileDataDoesNotAdvertiseTextAsAvatarURL(t *testing.T) {
	if got := buildGetProfileDataResponse(); len(got) != 0 {
		t.Fatalf("profile data should stay empty for Devin 3.7.16, got %x", got)
	}
}

func TestDuplicateModelIDsNormalized(t *testing.T) {
	// 模拟用户写了两个相同 id
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
upstream:
  base_url: "http://localhost:8317/v1"
  api_key: "123"
  model: "grok-4.5"
  models:
    - id: "grok-4.5"
      label: "A"
    - id: "grok-4.5"
      label: "B"
features:
  pure_local: true
  enable_stream: true
  enable_chat: true
  stub_unknown_rpc: true
`)
	p := t.TempDir() + "/c.yaml"
	if err := writeFile(p, raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	ms := cfg.ModelList()
	if len(ms) != 2 {
		t.Fatalf("want 2 models, got %d", len(ms))
	}
	if ms[0].ID == ms[1].ID {
		t.Fatalf("ids still duplicate: %q", ms[0].ID)
	}
	if ms[0].ResolveUpstream() != "grok-4.5" || ms[1].ResolveUpstream() != "grok-4.5" {
		t.Fatalf("upstream resolve wrong: %+v", ms)
	}
	if cfg.ResolveUpstreamModel(ms[1].ID) != "grok-4.5" {
		t.Fatalf("ResolveUpstreamModel failed")
	}
	// pick longer first
	body := []byte(ms[0].ID + " " + ms[1].ID)
	got := pickModel(body, ms[0].ID, []string{ms[0].ID, ms[1].ID})
	if got != ms[1].ID && len(ms[1].ID) > len(ms[0].ID) {
		// if ms[1] is longer (has __2), should pick it when both present
		if strings.Contains(ms[1].ID, "__") && got != ms[1].ID {
			t.Fatalf("pickModel should prefer longer id, got %s want %s", got, ms[1].ID)
		}
	}
}

func writeFile(path string, b []byte) error {
	return os.WriteFile(path, b, 0o644)
}

func TestThinkingResolveAndFamily(t *testing.T) {
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
upstream:
  base_url: "http://localhost:8317/v1"
  api_key: "123"
  model: "grok-4.5"
  thinking:
    param: "reasoning_effort"
    default: "medium"
  models:
    - id: "g-low"
      label: "L"
      upstream_model: "grok-4.5"
      thinking: "low"
      family: "Grok BYOK"
    - id: "g-high"
      label: "H"
      upstream_model: "grok-4.5"
      thinking: "high"
      family: "Grok BYOK"
features:
  pure_local: true
  enable_stream: true
  enable_chat: true
  stub_unknown_rpc: true
`)
	path := t.TempDir() + "/t.yaml"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ResolveThinking("g-low") != "low" {
		t.Fatalf("low: %s", cfg.ResolveThinking("g-low"))
	}
	if cfg.ResolveThinking("g-high") != "high" {
		t.Fatalf("high: %s", cfg.ResolveThinking("g-high"))
	}
	// Devin family metadata present
	b := buildClientModelConfigFor(cfg, "g-high", "H")
	if !bytes.Contains(b, []byte("Reasoning Effort")) || !bytes.Contains(b, []byte("High")) {
		t.Fatalf("missing family metadata: %x", b)
	}
	if !bytes.Contains(b, []byte("Grok BYOK")) {
		t.Fatalf("missing family label")
	}
}

func TestFamilyUIDSharedInModelInfo(t *testing.T) {
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
upstream:
  base_url: "http://localhost:8317/v1"
  api_key: "x"
  model: "grok-4.5"
  thinking: {param: "reasoning_effort", default: "medium"}
  families:
    - uid: "grok-4.5-byok"
      label: "Grok 4.5 BYOK"
      context_window: 200000
      max_tokens: 8192
  models:
    - id: "g-low"
      label: "Grok 4.5 BYOK Low"
      upstream_model: "grok-4.5"
      thinking: "low"
      family: "Grok 4.5 BYOK"
      family_uid: "grok-4.5-byok"
    - id: "g-high"
      label: "Grok 4.5 BYOK High"
      upstream_model: "grok-4.5"
      thinking: "high"
      family: "Grok 4.5 BYOK"
      family_uid: "grok-4.5-byok"
features:
  pure_local: true
  enable_stream: true
  enable_chat: true
  stub_unknown_rpc: true
`)
	path := t.TempDir() + "/f.yaml"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	low := buildClientModelConfigFor(cfg, "g-low", "Grok 4.5 BYOK Low")
	high := buildClientModelConfigFor(cfg, "g-high", "Grok 4.5 BYOK High")
	if !bytes.Contains(low, []byte("grok-4.5-byok")) || !bytes.Contains(high, []byte("grok-4.5-byok")) {
		t.Fatalf("family_uid missing in model card")
	}
	// both should contain shared family uid more than unique model ids alone
	if !bytes.Contains(low, []byte("Reasoning Effort")) || !bytes.Contains(low, []byte("Low")) {
		t.Fatalf("low missing reasoning effort meta")
	}
	if !bytes.Contains(high, []byte("High")) {
		t.Fatalf("high missing effort name")
	}
	// context/max from family
	if cfg.ModelList()[0].ContextWindow != 200000 || cfg.ModelList()[0].MaxTokens != 8192 {
		t.Fatalf("family token defaults not applied: %+v", cfg.ModelList()[0])
	}
}

func TestClientModelConfigContextUsesContextWindow(t *testing.T) {
	// UI contextLimit ? ClientModelConfig.max_tokens(field 18)???? context_window????? max_tokens?
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
auth: {fake_api_key: "k"}
upstream:
  base_url: "http://localhost:1/v1"
  api_key: "x"
  model: "m"
  families:
    - uid: "fam-x"
      label: "Fam X"
      context_window: 200000
      max_tokens: 8192
  models:
    - id: "m-med"
      label: "Fam X Medium"
      upstream_model: "m"
      thinking: "medium"
      family: "Fam X"
      family_uid: "fam-x"
features:
  enable_chat: true
  stub_unknown_rpc: true
`)
	path := t.TempDir() + "/ctx.yaml"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	card := buildClientModelConfigFor(cfg, "m-med", "Fam X Medium")
	fields := pbwire.ParseFields(card)
	var topMax int
	var miCtx, miOut int
	for _, f := range fields {
		if f.Number == 18 && f.Wire == 0 {
			topMax = int(f.Varint)
		}
		if f.Number == 23 && f.Wire == 2 {
			for _, sf := range pbwire.ParseFields(f.Bytes) {
				if sf.Number == 4 && sf.Wire == 0 {
					miCtx = int(sf.Varint)
				}
				if sf.Number == 13 && sf.Wire == 0 {
					miOut = int(sf.Varint)
				}
			}
		}
	}
	if topMax != 200000 {
		t.Fatalf("ClientModelConfig.max_tokens(field18)=%d want 200000 (context window)", topMax)
	}
	if miCtx != 200000 {
		t.Fatalf("model_info.max_tokens(field4)=%d want 200000", miCtx)
	}
	if miOut != 8192 {
		t.Fatalf("model_info.max_output_tokens(field13)=%d want 8192", miOut)
	}
}

func TestFamilyBaseURLWrittenToModelInfo(t *testing.T) {
	// Family 级配置（无 legacy 全局 upstream.base_url）时，model_info.base_url
	// （字段 11）必须携带 family 的 base_url；否则 Devin LS 判定模型 provider
	// 不可达（"Model provider unreachable"）并从模型列表过滤 BYOK 模型。
	raw := []byte(`
server: {host: "127.0.0.1", port: 8787}
auth: {fake_api_key: "k"}
upstream:
  api_key: "x"
  model: "m"
  families:
    - uid: "nine-router-byok"
      label: "9router"
      base_url: "https://router.example.com/v1"
      api_key: "family-key"
      upstream_model: "m"
      context_window: 200000
      max_tokens: 8192
  models:
    - id: "nine-router-byok-medium"
      label: "9router Medium"
      upstream_model: "m"
      thinking: "medium"
      family: "9router"
      family_uid: "nine-router-byok"
features:
  enable_chat: true
  stub_unknown_rpc: true
`)
	path := t.TempDir() + "/fam.yaml"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(cfg.Upstream.BaseURL) != "" {
		t.Fatalf("test setup should have no legacy global base_url, got %q", cfg.Upstream.BaseURL)
	}
	card := buildClientModelConfigFor(cfg, "nine-router-byok-medium", "9router Medium")
	fields := pbwire.ParseFields(card)
	found := false
	for _, f := range fields {
		if f.Number == 23 && f.Wire == 2 { // model_info
			for _, sf := range pbwire.ParseFields(f.Bytes) {
				if sf.Number == 11 && sf.Wire == 2 { // base_url
					if string(sf.Bytes) != "https://router.example.com/v1" {
						t.Fatalf("model_info.base_url=%q want family base_url", string(sf.Bytes))
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("model_info.base_url (field 11) missing for family-based config")
	}

	// 兼容回退：uid 不在模型列表时仍写 legacy 全局 base_url（旧行为不变）。
	legacy := &config.File{}
	legacy.Upstream.BaseURL = "http://localhost:8317/v1"
	legacy.Upstream.Model = "grok-4.5"
	legacy.Upstream.Models = []config.ModelEntry{{ID: "grok-4.5", Label: "BYOK - grok-4.5", UpstreamModel: "grok-4.5"}}
	cardLegacy := buildClientModelConfigFor(legacy, "grok-4.5", "BYOK - grok-4.5")
	fieldsLegacy := pbwire.ParseFields(cardLegacy)
	foundLegacy := false
	for _, f := range fieldsLegacy {
		if f.Number == 23 && f.Wire == 2 {
			for _, sf := range pbwire.ParseFields(f.Bytes) {
				if sf.Number == 11 && sf.Wire == 2 {
					if string(sf.Bytes) != "http://localhost:8317/v1" {
						t.Fatalf("legacy model_info.base_url=%q want global base_url", string(sf.Bytes))
					}
					foundLegacy = true
				}
			}
		}
	}
	if !foundLegacy {
		t.Fatal("legacy model_info.base_url (field 11) missing")
	}
}
