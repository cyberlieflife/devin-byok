package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// File 为 devin-byok 主配置。
type File struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Upstream UpstreamConfig `yaml:"upstream"`
	Devin    DevinConfig    `yaml:"devin"`
	Features FeaturesConfig `yaml:"features"`
	Update   UpdateConfig   `yaml:"update"`
	Tools    ToolsConfig    `yaml:"tools"`
	Cache    CacheConfig    `yaml:"cache"`
}

type ServerConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	PublicBase string `yaml:"public_base"`
}

type AuthConfig struct {
	FakeAPIKey string `yaml:"fake_api_key"`
	FakeEmail  string `yaml:"fake_email"`
	FakeName   string `yaml:"fake_name"`
}

// ThinkingConfig 思考强度：上游参数 + Devin UI 映射。
type SamplingConfig struct {
	// Temperature 采样温度；nil 表示不发送该字段
	Temperature *float64 `yaml:"temperature"`
	// MaxTokens 最大生成 token；0 表示不发送
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`
	// TopP nucleus sampling；nil 表示不发送
	TopP *float64 `yaml:"top_p"`
}

type ThinkingConfig struct {
	// Param 上游请求参数形态：
	//   reasoning_effort  -> {"reasoning_effort":"high"}
	//   reasoning.effort  -> {"reasoning":{"effort":"high"}}
	//   none              -> 不发送
	Param string `yaml:"param"`
	// Default 默认强度：low|medium|high|xhigh|minimal|none
	Default string `yaml:"default"`
}

// ModelEntry 可在 UI 中选择的模型。
// Devin 用 model_uid(=ID) 区分；ID 必须唯一。
type ModelEntry struct {
	ID            string `yaml:"id" json:"id"`
	Label         string `yaml:"label" json:"label"`
	UpstreamModel string `yaml:"upstream_model" json:"upstream_model"`
	Thinking      string `yaml:"thinking" json:"thinking"`
	Family        string `yaml:"family" json:"family"`
	FamilyUID     string `yaml:"family_uid" json:"family_uid"`
	FamilyOrder   int    `yaml:"family_order" json:"family_order"`
	ContextWindow int    `yaml:"context_window" json:"context_window"`
	MaxTokens     int    `yaml:"max_tokens" json:"max_tokens"`
	// 模型级供应商（可覆盖 family；对齐 cursor-byok 每模型 baseURL/apiKey/modelID）
	Provider string            `yaml:"provider" json:"provider"` // openai | anthropic
	BaseURL  string            `yaml:"base_url" json:"base_url"`
	APIKey   string            `yaml:"api_key" json:"api_key"`
	Headers  map[string]string `yaml:"headers" json:"headers,omitempty"`
	TimeoutSec int             `yaml:"timeout_sec" json:"timeout_sec,omitempty"`
}

// FamilyConfig 以 family 为单位的默认上下文/输出上限。
type FamilyConfig struct {
	UID           string `yaml:"uid" json:"uid"`
	Label         string `yaml:"label" json:"label"`
	ContextWindow int    `yaml:"context_window" json:"context_window"`
	MaxTokens     int    `yaml:"max_tokens" json:"max_tokens"`
	// 供应商配置（family 内所有思考强度变体默认共用）
	Provider      string            `yaml:"provider" json:"provider"` // openai | anthropic
	BaseURL       string            `yaml:"base_url" json:"base_url"`
	APIKey        string            `yaml:"api_key" json:"api_key"`
	UpstreamModel string            `yaml:"upstream_model" json:"upstream_model"`
	Headers       map[string]string `yaml:"headers" json:"headers,omitempty"`
	TimeoutSec    int               `yaml:"timeout_sec" json:"timeout_sec,omitempty"`
}

type UpstreamConfig struct {
	BaseURL    string            `yaml:"base_url"`
	APIKey     string            `yaml:"api_key"`
	Model      string            `yaml:"model"`
	Models     []ModelEntry      `yaml:"models"`
	Families   []FamilyConfig     `yaml:"families"`
	TimeoutSec int               `yaml:"timeout_sec"`
	Headers    map[string]string `yaml:"default_headers"`
	Thinking   ThinkingConfig    `yaml:"thinking"`
	Sampling   SamplingConfig    `yaml:"sampling"`
}

type DevinConfig struct {
	InstallDir    string   `yaml:"install_dir"`
	PortalURLKeys []string `yaml:"portal_url_keys"`
}

type FeaturesConfig struct {
	EnableChat         bool `yaml:"enable_chat"`
	EnableCascadeTools bool `yaml:"enable_cascade_tools"`
	StubUnknownRPC     bool `yaml:"stub_unknown_rpc"`
	PureLocal          bool `yaml:"pure_local"`
	EnableStream       bool `yaml:"enable_stream"`
	// EnableDeepWiki 符号文档/DeepWiki 流式生成（默认 true）
	EnableDeepWiki bool `yaml:"enable_deepwiki"`
	// EnableCodeMap CodeMap 列表/分享 stub + GenerateCodeMap 上游生成
	EnableCodeMap bool `yaml:"enable_codemap"`
	// DeepWikiModel / CodeMap*Model：选用已有 models[].id（思考强度变体）
	DeepWikiModel     string `yaml:"deepwiki_model"`
	CodeMapModel      string `yaml:"codemap_model"` // 兼容旧键；未分模式时作为 Smart 回退
	CodeMapFastModel  string `yaml:"codemap_fast_model"`
	CodeMapSmartModel string `yaml:"codemap_smart_model"`
}

// UpdateConfig 在线更新（GitHub Releases）。
type UpdateConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	Repo          string `yaml:"repo" json:"repo"` // owner/name
	AssetContains string `yaml:"asset_contains" json:"asset_contains"`
	AutoApply     bool   `yaml:"auto_apply" json:"auto_apply"`
	CheckURL      string `yaml:"check_url" json:"check_url"`
}



// ToolsConfig Cascade ?????P0/P1??
// CacheConfig 本地响应/会话缓存。
type CacheConfig struct {
	// Enabled 总开关（prompt 缓存 + 可选本地响应缓存）
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Mode: prompt | response | both
	// prompt=对齐 cursor-byok（prompt_cache_key + 统计 cached_tokens）
	// response=本地整段回复缓存（仅无 tools 时）
	// both=两者
	Mode string `yaml:"mode" json:"mode"`
	// TTLSec/MaxEntries 仅 response 模式
	TTLSec     int `yaml:"ttl_sec" json:"ttl_sec"`
	MaxEntries int `yaml:"max_entries" json:"max_entries"`
	// PromptCacheKey 固定键；空则按会话自动生成（devin-byok:<conv>:<model>）
	PromptCacheKey string `yaml:"prompt_cache_key" json:"prompt_cache_key"`
	// IncludeCacheWriteInHitRate 是否把 cache write 算进命中分母（cursor-byok 有同名配置）
	IncludeCacheWriteInHitRate bool `yaml:"include_cache_write_in_hit_rate" json:"include_cache_write_in_hit_rate"`
}

type ToolsConfig struct {
	// Mode: off | readonly | standard | full
	// off=?? tools?readonly=???standard=??????full=? run_command
	Mode string `yaml:"mode"`
	// TimeoutSec ???????????0 ??????? 300s ? upstream.timeout_sec?
	TimeoutSec int `yaml:"timeout_sec"`
	// WorkspaceHint ?????????????? true?
	WorkspaceHint *bool `yaml:"workspace_hint"`
	// Allow ????????
	Allow []string `yaml:"allow"`
	// Deny ???????????? mode/allow?
	Deny []string `yaml:"deny"`
}

// Load 读取 YAML 配置并填充默认值。
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	f.applyDefaults()
	if err := f.validate(); err != nil {
		return nil, err
	}
	return &f, nil
}

// Save 将配置写回 YAML（会重写文件，注释可能丢失）。
func Save(path string, f *File) error {
	if f == nil {
		return fmt.Errorf("nil config")
	}
	f.applyDefaults()
	if err := f.validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// GUIPatch 管理页可改字段。
type GUIPatch struct {
	BaseURL              *string `json:"base_url"`
	APIKey               *string `json:"api_key"`
	Model                *string `json:"model"`
	TimeoutSec           *int    `json:"timeout_sec"`
	ToolsMode            *string `json:"tools_mode"`
	ToolsTimeoutSec      *int    `json:"tools_timeout_sec"`
	EnableStream         *bool   `json:"enable_stream"`
	EnableCascadeTools   *bool   `json:"enable_cascade_tools"`
	PureLocal            *bool   `json:"pure_local"`
	DeepWikiModel        *string `json:"deepwiki_model"`
	CodeMapModel         *string `json:"codemap_model"`
	CodeMapFastModel     *string `json:"codemap_fast_model"`
	CodeMapSmartModel    *string `json:"codemap_smart_model"`
	UpdateEnabled        *bool   `json:"update_enabled"`
	UpdateAutoApply      *bool   `json:"update_auto_apply"`
	UpdateRepo           *string `json:"update_repo"`
}

// ApplyGUIPatch 应用管理页补丁。
func (f *File) ApplyGUIPatch(p GUIPatch) {
	if p.BaseURL != nil {
		f.Upstream.BaseURL = strings.TrimSpace(*p.BaseURL)
	}
	if p.APIKey != nil {
		f.Upstream.APIKey = strings.TrimSpace(*p.APIKey)
	}
	if p.Model != nil {
		f.Upstream.Model = strings.TrimSpace(*p.Model)
	}
	if p.TimeoutSec != nil && *p.TimeoutSec > 0 {
		f.Upstream.TimeoutSec = *p.TimeoutSec
	}
	if p.ToolsMode != nil {
		f.Tools.Mode = NormalizeToolsMode(*p.ToolsMode)
	}
	if p.ToolsTimeoutSec != nil {
		f.Tools.TimeoutSec = *p.ToolsTimeoutSec
	}
	if p.EnableStream != nil {
		f.Features.EnableStream = *p.EnableStream
	}
	if p.EnableCascadeTools != nil {
		f.Features.EnableCascadeTools = *p.EnableCascadeTools
	}
	if p.PureLocal != nil {
		f.Features.PureLocal = *p.PureLocal
	}
	if p.DeepWikiModel != nil {
		f.Features.DeepWikiModel = strings.TrimSpace(*p.DeepWikiModel)
	}
	if p.CodeMapModel != nil {
		f.Features.CodeMapModel = strings.TrimSpace(*p.CodeMapModel)
	}
	if p.CodeMapFastModel != nil {
		f.Features.CodeMapFastModel = strings.TrimSpace(*p.CodeMapFastModel)
	}
	if p.CodeMapSmartModel != nil {
		f.Features.CodeMapSmartModel = strings.TrimSpace(*p.CodeMapSmartModel)
	}
	if p.UpdateEnabled != nil {
		f.Update.Enabled = *p.UpdateEnabled
	}
	if p.UpdateAutoApply != nil {
		f.Update.AutoApply = *p.UpdateAutoApply
	}
	if p.UpdateRepo != nil {
		f.Update.Repo = strings.TrimSpace(*p.UpdateRepo)
	}
}

// MaskAPIKey 脱敏展示。
func MaskAPIKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}

func (f *File) applyDefaults() {
	if f.Server.Host == "" {
		f.Server.Host = "127.0.0.1"
	}
	if f.Server.Port == 0 {
		f.Server.Port = 8787
	}
	if f.Server.PublicBase == "" {
		f.Server.PublicBase = fmt.Sprintf("http://%s:%d", f.Server.Host, f.Server.Port)
	}
	if f.Auth.FakeAPIKey == "" {
		f.Auth.FakeAPIKey = "devin-byok-local-key"
	}
	if f.Auth.FakeEmail == "" {
		f.Auth.FakeEmail = "byok@local"
	}
	if f.Auth.FakeName == "" {
		f.Auth.FakeName = "BYOK Local"
	}
	if f.Upstream.TimeoutSec == 0 {
		f.Upstream.TimeoutSec = 120
	}
	if f.Devin.InstallDir == "" {
		f.Devin.InstallDir = `D:\Devin`
	}
	if len(f.Devin.PortalURLKeys) == 0 {
		f.Devin.PortalURLKeys = []string{"devin.portalUrl", "windsurf.portalUrl"}
	}
	if !f.Features.EnableChat && !f.Features.EnableCascadeTools && !f.Features.StubUnknownRPC {
		f.Features.EnableChat = true
		f.Features.StubUnknownRPC = true
	}
		if f.Update.Repo == "" {
		f.Update.Repo = "cyberlieflife/devin-byok"
	}
	if f.Update.AssetContains == "" {
		f.Update.AssetContains = "windows-amd64.zip"
	}
	// update.enabled 默认 false；config.example 中建议 true
	if f.Upstream.Thinking.Param == "" {
		f.Upstream.Thinking.Param = "reasoning_effort"
	}
	if f.Upstream.Thinking.Default == "" {
		f.Upstream.Thinking.Default = "medium"
	}
	// tools ??
	if f.Tools.Mode == "" {
		if f.Features.EnableCascadeTools {
			f.Tools.Mode = "standard"
		} else {
			f.Tools.Mode = "off"
		}
	}
	f.Tools.Mode = NormalizeToolsMode(f.Tools.Mode)
	// cache 默认开启
	if !f.Cache.Enabled && f.Cache.TTLSec == 0 && f.Cache.MaxEntries == 0 {
		// 零值：启用默认缓存
		f.Cache.Enabled = true
	}
	if f.Cache.TTLSec <= 0 {
		f.Cache.TTLSec = 180
	}
	if f.Cache.MaxEntries <= 0 {
		f.Cache.MaxEntries = 128
	}
	if strings.TrimSpace(f.Cache.Mode) == "" {
		f.Cache.Mode = "prompt"
	}
	f.Cache.Mode = strings.ToLower(strings.TrimSpace(f.Cache.Mode))
	if f.Tools.WorkspaceHint == nil {
		v := true
		f.Tools.WorkspaceHint = &v
	}
	f.Upstream.Models = normalizeModelEntries(f.Upstream.Models, f.Upstream.Model, f.Upstream.Thinking.Default, f.Upstream.Families)
	if len(f.Upstream.Models) == 0 && f.Upstream.Model != "" {
		th := NormalizeThinkingLevel(f.Upstream.Thinking.Default)
		if th == "" {
			th = "medium"
		}
		f.Upstream.Models = []ModelEntry{{
			ID: f.Upstream.Model, Label: "BYOK - " + f.Upstream.Model, UpstreamModel: f.Upstream.Model,
			Thinking: th, Family: f.Upstream.Model, FamilyUID: slugID(f.Upstream.Model),
			FamilyOrder: DevinFamilyOrder(th), ContextWindow: 128000, MaxTokens: 8192,
		}}
	}
	if f.Upstream.Model == "" && len(f.Upstream.Models) > 0 {
		f.Upstream.Model = f.Upstream.Models[0].ResolveUpstream()
	}
	f.migrateProviderToFamilies()
}

func normalizeModelEntries(in []ModelEntry, defaultUpstream, defaultThinking string, families []FamilyConfig) []ModelEntry {
	if len(in) == 0 {
		return nil
	}
	famByLabel := map[string]FamilyConfig{}
	famByUID := map[string]FamilyConfig{}
	for _, f := range families {
		if f.Label != "" {
			famByLabel[f.Label] = f
		}
		uid := f.UID
		if uid == "" {
			uid = slugID(f.Label)
		}
		if uid != "" {
			famByUID[uid] = f
		}
	}

	seen := map[string]int{}
	out := make([]ModelEntry, 0, len(in))
	for _, m := range in {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		up := strings.TrimSpace(m.UpstreamModel)
		if up == "" {
			up = id
		}
		baseID := id
		seen[baseID]++
		if seen[baseID] > 1 {
			id = fmt.Sprintf("%s__%d", baseID, seen[baseID])
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = "BYOK - " + id
		}
		thinking := NormalizeThinkingLevel(m.Thinking)
		if thinking == "" {
			thinking = NormalizeThinkingLevel(defaultThinking)
		}
		if thinking == "" {
			thinking = "medium"
		}
		family := strings.TrimSpace(m.Family)
		if family == "" {
			family = up
		}
		familyUID := strings.TrimSpace(m.FamilyUID)
		if familyUID == "" {
			// 优先 families 配置
			if fc, ok := famByLabel[family]; ok {
				if fc.UID != "" {
					familyUID = fc.UID
				} else {
					familyUID = slugID(fc.Label)
				}
			}
		}
		if familyUID == "" {
			familyUID = slugID(family)
		}
		// 继承 family 级 token 配置
		ctxWin := m.ContextWindow
		maxTok := m.MaxTokens
		if fc, ok := famByUID[familyUID]; ok {
			if ctxWin <= 0 {
				ctxWin = fc.ContextWindow
			}
			if maxTok <= 0 {
				maxTok = fc.MaxTokens
			}
		} else if fc, ok := famByLabel[family]; ok {
			if ctxWin <= 0 {
				ctxWin = fc.ContextWindow
			}
			if maxTok <= 0 {
				maxTok = fc.MaxTokens
			}
		}
		if ctxWin <= 0 {
			ctxWin = 128000
		}
		if maxTok <= 0 {
			maxTok = 8192
		}
		order := m.FamilyOrder
		if order < 0 {
			order = 0
		}
		// 若用户未显式写 family_order（yaml 0），按 thinking 自动
		// 无法区分“显式 0”与“未写”；low 本身就是 0，OK
		if m.FamilyOrder == 0 && thinking != "low" && thinking != "none" && thinking != "minimal" {
			order = DevinFamilyOrder(thinking)
		} else if m.FamilyOrder == 0 {
			order = DevinFamilyOrder(thinking)
		}
		out = append(out, ModelEntry{
			ID:            id,
			Label:         label,
			UpstreamModel: up,
			Thinking:      thinking,
			Family:        family,
			FamilyUID:     familyUID,
			FamilyOrder:   order,
			ContextWindow: ctxWin,
			MaxTokens:     maxTok,
		})
	}
	_ = defaultUpstream
	return out
}

// slugID 生成稳定 family_uid。
func slugID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "family"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "family"
	}
	return out
}

// DevinFamilyOrder 与官方 Grok 一致：Low=0, Medium=1, High=2...
func DevinFamilyOrder(level string) int {
	switch NormalizeThinkingLevel(level) {
	case "none":
		return 0
	case "minimal":
		return 0
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	case "xhigh":
		return 3
	case "max":
		return 4
	default:
		return 1
	}
}
// NormalizeThinkingLevel 统一思考强度命名。
func NormalizeThinkingLevel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "default":
		return ""
	case "none", "off", "disable", "disabled":
		return "none"
	case "min", "minimal", "lowest":
		return "minimal"
	case "low", "lite":
		return "low"
	case "med", "medium", "mid", "normal":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "x-high", "extra_high", "extrahigh", "very_high", "very-high":
		return "xhigh"
	case "max", "maximum", "highest":
		return "max"
	default:
		return s
	}
}

// ThinkingOrder 族内排序（数值越大越强）。
func ThinkingOrder(level string) int {
	switch NormalizeThinkingLevel(level) {
	case "none":
		return 0
	case "minimal":
		return 1
	case "low":
		return 2
	case "medium":
		return 3
	case "high":
		return 4
	case "xhigh":
		return 5
	case "max":
		return 6
	default:
		return 3
	}
}

// DevinThinkingName 映射到 Devin 模型族里的显示名（Reasoning Effort）。
func DevinThinkingName(level string) string {
	switch NormalizeThinkingLevel(level) {
	case "none":
		return "None"
	case "minimal":
		return "Minimal"
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "XHigh"
	case "max":
		return "Max"
	default:
		// 未知值原样标题化
		if level == "" {
			return "Medium"
		}
		return strings.ToUpper(level[:1]) + level[1:]
	}
}

func (f *File) validate() error {
	if f.Server.Port <= 0 || f.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", f.Server.Port)
	}
	seen := map[string]bool{}
	for _, m := range f.ModelList() {
		if seen[m.ID] {
			return fmt.Errorf("duplicate model id after normalize: %s", m.ID)
		}
		seen[m.ID] = true
		if m.ContextWindow > 0 && m.MaxTokens > 0 && m.MaxTokens > m.ContextWindow {
			return fmt.Errorf("model %s: max_tokens(%d) > context_window(%d)????????", m.ID, m.MaxTokens, m.ContextWindow)
		}
	}
	switch NormalizeToolsMode(f.Tools.Mode) {
	case "off", "readonly", "standard", "full":
	default:
		return fmt.Errorf("invalid tools.mode: %q (use off|readonly|standard|full)", f.Tools.Mode)
	}
	return nil
}

// NormalizeToolsMode ????????
func NormalizeToolsMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "", "std", "default":
		return "standard"
	case "read", "read_only", "read-only":
		return "readonly"
	case "all", "command", "commands":
		return "full"
	case "off", "false", "0", "none", "disable", "disabled":
		return "off"
	case "readonly", "standard", "full":
		return strings.ToLower(strings.TrimSpace(m))
	default:
		return strings.ToLower(strings.TrimSpace(m))
	}
}

// ToolsMode ??????? tools.mode?
func (f *File) ToolsMode() string {
	return NormalizeToolsMode(f.Tools.Mode)
}

// ToolsWorkspaceHint ???????????
func (f *File) ToolsWorkspaceHint() bool {
	if f.Tools.WorkspaceHint == nil {
		return true
	}
	return *f.Tools.WorkspaceHint
}

// ResolveChatTimeoutSec ??????????? tools.timeout_sec ???
func (f *File) ResolveChatTimeoutSec(hasTools bool) int {
	base := f.Upstream.TimeoutSec
	if base <= 0 {
		base = 120
	}
	if !hasTools {
		return base
	}
	if f.Tools.TimeoutSec > 0 {
		return f.Tools.TimeoutSec
	}
	if base < 300 {
		return 300
	}
	return base
}

func (f *File) Addr() string {
	return fmt.Sprintf("%s:%d", f.Server.Host, f.Server.Port)
}

func (f *File) APIBasePath() string {
	return "/_route/api_server"
}

func (f *File) ModelList() []ModelEntry {
	if len(f.Upstream.Models) > 0 {
		return append([]ModelEntry(nil), f.Upstream.Models...)
	}
	id := f.Upstream.Model
	if id == "" {
		id = "byok-default"
	}
	th := NormalizeThinkingLevel(f.Upstream.Thinking.Default)
	if th == "" {
		th = "medium"
	}
	return []ModelEntry{{
		ID: id, Label: "BYOK - " + id, UpstreamModel: id,
		Thinking: th, Family: id, FamilyUID: slugID(id), FamilyOrder: DevinFamilyOrder(th),
		ContextWindow: 128000, MaxTokens: 8192,
	}}
}

func (f *File) DefaultModelID() string {
	ms := f.ModelList()
	if len(ms) == 0 {
		return ""
	}
	// 优先 medium 思考强度变体（family 默认）
	for _, m := range ms {
		if m.Thinking == "medium" {
			return m.ID
		}
	}
	return ms[0].ID
}


// compactModelKey 去掉分隔符，便于 grok4.5 对齐 grok-4.5-byok-high。
func compactModelKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ResolveModelUID 解析 UI/Command 传来的模型标识：精确 ID、忽略大小写、去连字符匹配、family/upstream。
// 多候选时优先 default_model，其次 medium thinking。
func (f *File) ResolveModelUID(selectedUID string) (ModelEntry, bool) {
	selectedUID = strings.TrimSpace(selectedUID)
	if selectedUID == "" {
		return ModelEntry{}, false
	}
	if m, ok := f.FindModel(selectedUID); ok {
		return m, true
	}
	low := strings.ToLower(selectedUID)
	list := f.ModelList()
	for _, m := range list {
		if strings.ToLower(m.ID) == low {
			return m, true
		}
	}
	key := compactModelKey(selectedUID)
	if key == "" {
		return ModelEntry{}, false
	}
	var matches []ModelEntry
	for _, m := range list {
		cands := []string{m.ID, m.ResolveUpstream(), m.FamilyUID, m.Family, m.Label}
		hit := false
		for _, c := range cands {
			ck := compactModelKey(c)
			if ck == "" {
				continue
			}
			if ck == key || strings.HasPrefix(ck, key) || strings.Contains(ck, key) || strings.Contains(key, ck) {
				hit = true
				break
			}
		}
		if hit {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return ModelEntry{}, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	def := strings.TrimSpace(f.DefaultModelID())
	for _, m := range matches {
		if m.ID == def {
			return m, true
		}
	}
	for _, m := range matches {
		if NormalizeThinkingLevel(m.Thinking) == "medium" {
			return m, true
		}
	}
	return matches[0], true
}

func (f *File) FindModel(selectedUID string) (ModelEntry, bool) {
	selectedUID = strings.TrimSpace(selectedUID)
	for _, m := range f.ModelList() {
		if m.ID == selectedUID {
			return m, true
		}
	}
	return ModelEntry{}, false
}

func (f *File) ResolveUpstreamModel(selectedUID string) string {
	// 仅使用请求选中的 family 模型条目的 upstream_model，不再回落全局 upstream.model / 原始 uid
	if m, ok := f.FindModel(selectedUID); ok {
		return m.ResolveUpstream()
	}
	return ""
}

// ResolveThinking 返回选中模型的思考强度（已 normalize）。
func (f *File) ResolveThinking(selectedUID string) string {
	if m, ok := f.FindModel(selectedUID); ok {
		if m.Thinking != "" {
			return m.Thinking
		}
	}
	th := NormalizeThinkingLevel(f.Upstream.Thinking.Default)
	if th == "" {
		return "medium"
	}
	return th
}

// FeatureModelID 返回 DeepWiki/CodeMap 选用的模型变体 id（思考强度为单位）。
// kind: deepwiki | codemap | codemap_fast | codemap_smart
// 无效或空时按回退链解析，最终 DefaultModelID。
func (f *File) FeatureModelID(kind string) string {
	if f == nil {
		return ""
	}
	k := strings.ToLower(strings.TrimSpace(kind))
	candidates := []string{}
	switch k {
	case "deepwiki", "wiki":
		candidates = append(candidates, f.Features.DeepWikiModel)
	case "codemap_fast", "fast":
		candidates = append(candidates, f.Features.CodeMapFastModel, f.Features.CodeMapModel)
	case "codemap_smart", "smart":
		candidates = append(candidates, f.Features.CodeMapSmartModel, f.Features.CodeMapModel)
	case "codemap", "code_map", "map":
		// 未分模式：优先 Smart，再旧键
		candidates = append(candidates, f.Features.CodeMapSmartModel, f.Features.CodeMapModel, f.Features.CodeMapFastModel)
	default:
		candidates = append(candidates, f.Features.CodeMapModel)
	}
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := f.FindModel(id); ok {
			return id
		}
	}
	return f.DefaultModelID()
}

// FeatureModelIDForCodeMapMode 按 CodeMap 请求 mode（fast/smart/agent）选模型。
func (f *File) FeatureModelIDForCodeMapMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "fast":
		return f.FeatureModelID("codemap_fast")
	case "smart", "agent", "":
		// agent/空 默认走 Smart
		return f.FeatureModelID("codemap_smart")
	default:
		if strings.Contains(m, "fast") {
			return f.FeatureModelID("codemap_fast")
		}
		if strings.Contains(m, "smart") {
			return f.FeatureModelID("codemap_smart")
		}
		return f.FeatureModelID("codemap")
	}
}


func (m ModelEntry) ResolveUpstream() string {
	if strings.TrimSpace(m.UpstreamModel) != "" {
		return m.UpstreamModel
	}
	return m.ID
}

func NormalizeChatCompletionsURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		return ""
	}
	low := strings.ToLower(u)
	if strings.HasSuffix(low, "/chat/completions") {
		return u
	}
	if strings.HasSuffix(low, "/v1") {
		return u + "/chat/completions"
	}
	return u + "/v1/chat/completions"
}


// FamilyGroup 以 family 聚合（含供应商配置）。
type FamilyGroup struct {
	UID           string       `json:"uid"`
	Label         string       `json:"label"`
	ContextWindow int          `json:"context_window"`
	MaxTokens     int          `json:"max_tokens"`
	Provider      string       `json:"provider"`
	BaseURL       string       `json:"base_url"`
	APIKeyMasked  string       `json:"api_key_masked"`
	APIKeySet     bool         `json:"api_key_set"`
	UpstreamModel string       `json:"upstream_model"`
	TimeoutSec    int          `json:"timeout_sec"`
	Variants      []ModelEntry `json:"variants"`
}

// GroupModelsByFamily 返回 family 卡片数据。
func (f *File) GroupModelsByFamily() []FamilyGroup {
	ms := f.ModelList()
	order := []string{}
	m := map[string]*FamilyGroup{}
	famByUID := map[string]FamilyConfig{}
	for _, fc := range f.Upstream.Families {
		fu := fc.UID
		if fu == "" {
			fu = slugID(fc.Label)
		}
		famByUID[fu] = fc
	}
	for _, e := range ms {
		uid := e.FamilyUID
		if uid == "" {
			uid = slugID(e.Family)
		}
		if uid == "" {
			uid = e.ID
		}
		g, ok := m[uid]
		if !ok {
			fc := famByUID[uid]
			label := fc.Label
			if label == "" {
				label = e.Family
			}
			if label == "" {
				label = e.Label
			}
			ctx, maxo := e.ContextWindow, e.MaxTokens
			if fc.ContextWindow > 0 {
				ctx = fc.ContextWindow
			}
			if fc.MaxTokens > 0 {
				maxo = fc.MaxTokens
			}
			base := firstNonEmpty(e.BaseURL, fc.BaseURL, f.Upstream.BaseURL)
			key := firstNonEmpty(e.APIKey, fc.APIKey, f.Upstream.APIKey)
			up := firstNonEmpty(e.UpstreamModel, fc.UpstreamModel, f.Upstream.Model)
			prov := NormalizeProvider(firstNonEmpty(e.Provider, fc.Provider, "openai"))
			to := e.TimeoutSec
			if to <= 0 {
				to = fc.TimeoutSec
			}
			if to <= 0 {
				to = f.Upstream.TimeoutSec
			}
			g = &FamilyGroup{
				UID: uid, Label: label, ContextWindow: ctx, MaxTokens: maxo,
				Provider: prov, BaseURL: base, APIKeyMasked: MaskAPIKey(key), APIKeySet: key != "",
				UpstreamModel: up, TimeoutSec: to,
			}
			m[uid] = g
			order = append(order, uid)
		}
		// hide secrets on variants for API
		ev := e
		if ev.APIKey != "" {
			ev.APIKey = MaskAPIKey(ev.APIKey)
		}
		g.Variants = append(g.Variants, ev)
	}
	out := make([]FamilyGroup, 0, len(order))
	for _, uid := range order {
		out = append(out, *m[uid])
	}
	return out
}

// FamilyUpsertInput GUI/API 创建更新 family+供应商+思考强度。
type FamilyUpsertInput struct {
	UID           string   `json:"uid"`
	Label         string   `json:"label"`
	UpstreamModel string   `json:"upstream_model"`
	Provider      string   `json:"provider"`
	BaseURL       string   `json:"base_url"`
	APIKey        string   `json:"api_key"`
	ContextWindow int      `json:"context_window"`
	MaxTokens     int      `json:"max_tokens"`
	TimeoutSec    int      `json:"timeout_sec"`
	Levels        []string `json:"levels"`
}

// UpsertFamilyPresets 创建/更新 family 的供应商与思考强度变体。
func (f *File) UpsertFamilyPresets(in FamilyUpsertInput) {
	uid := strings.TrimSpace(in.UID)
	label := strings.TrimSpace(in.Label)
	if uid == "" {
		uid = slugID(label)
	}
	if label == "" {
		label = uid
	}
	up := strings.TrimSpace(in.UpstreamModel)
	if up == "" {
		up = firstNonEmpty(f.Upstream.Model, label)
	}
	levels := in.Levels
	if len(levels) == 0 {
		levels = []string{"low", "medium", "high"}
	}
	ctxWin, maxOut := in.ContextWindow, in.MaxTokens
	if ctxWin <= 0 {
		ctxWin = 128000
	}
	if maxOut <= 0 {
		maxOut = 8192
	}
	prov := NormalizeProvider(in.Provider)
	base := strings.TrimSpace(in.BaseURL)
	key := strings.TrimSpace(in.APIKey)

	foundFam := false
	for i := range f.Upstream.Families {
		fu := f.Upstream.Families[i].UID
		if fu == "" {
			fu = slugID(f.Upstream.Families[i].Label)
		}
		if fu == uid {
			f.Upstream.Families[i].UID = uid
			f.Upstream.Families[i].Label = label
			f.Upstream.Families[i].ContextWindow = ctxWin
			f.Upstream.Families[i].MaxTokens = maxOut
			f.Upstream.Families[i].Provider = prov
			if base != "" {
				f.Upstream.Families[i].BaseURL = base
			}
			// 空 key 表示不改
			if key != "" {
				f.Upstream.Families[i].APIKey = key
			}
			f.Upstream.Families[i].UpstreamModel = up
			if in.TimeoutSec > 0 {
				f.Upstream.Families[i].TimeoutSec = in.TimeoutSec
			}
			foundFam = true
			break
		}
	}
	if !foundFam {
		if base == "" {
			base = f.Upstream.BaseURL
		}
		if key == "" {
			key = f.Upstream.APIKey
		}
		f.Upstream.Families = append(f.Upstream.Families, FamilyConfig{
			UID: uid, Label: label, ContextWindow: ctxWin, MaxTokens: maxOut,
			Provider: prov, BaseURL: base, APIKey: key, UpstreamModel: up,
			TimeoutSec: in.TimeoutSec,
		})
	}
	// rebuild variants
	kept := make([]ModelEntry, 0, len(f.Upstream.Models))
	for _, e := range f.Upstream.Models {
		eu := e.FamilyUID
		if eu == "" {
			eu = slugID(e.Family)
		}
		if eu == uid {
			continue
		}
		kept = append(kept, e)
	}
	for _, lv := range levels {
		lv = NormalizeThinkingLevel(lv)
		if lv == "" {
			continue
		}
		id := uid + "-" + lv
		kept = append(kept, ModelEntry{
			ID: id, Label: label + " " + capitalize(lv), UpstreamModel: up,
			Thinking: lv, Family: label, FamilyUID: uid, FamilyOrder: DevinFamilyOrder(lv),
			ContextWindow: ctxWin, MaxTokens: maxOut,
		})
	}
	f.Upstream.Models = kept
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}



// slugID public alias
func SlugID(s string) string { return slugID(s) }


func (f *File) CacheMode() string {
	m := strings.ToLower(strings.TrimSpace(f.Cache.Mode))
	if m == "" {
		return "prompt"
	}
	return m
}

func (f *File) PromptCacheEnabled() bool {
	if f == nil || !f.Cache.Enabled {
		return false
	}
	m := f.CacheMode()
	return m == "prompt" || m == "both"
}

func (f *File) ResponseCacheEnabled() bool {
	if f == nil || !f.Cache.Enabled {
		return false
	}
	m := f.CacheMode()
	return m == "response" || m == "both"
}


func NormalizeProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "anthropic", "claude":
		return "anthropic"
	default:
		return "openai"
	}
}

// migrateProviderToFamilies 把旧版全局 upstream.base_url/api_key 下沉到尚无供应商配置的 family。
func (f *File) migrateProviderToFamilies() {
	if f == nil {
		return
	}
	globalURL := strings.TrimSpace(f.Upstream.BaseURL)
	globalKey := strings.TrimSpace(f.Upstream.APIKey)
	globalModel := strings.TrimSpace(f.Upstream.Model)
	// ensure family rows exist for all models
	seen := map[string]bool{}
	for _, m := range f.Upstream.Models {
		uid := m.FamilyUID
		if uid == "" {
			uid = slugID(m.Family)
		}
		if uid == "" {
			uid = m.ID
		}
		if seen[uid] {
			continue
		}
		seen[uid] = true
		found := false
		for i := range f.Upstream.Families {
			fu := f.Upstream.Families[i].UID
			if fu == "" {
				fu = slugID(f.Upstream.Families[i].Label)
			}
			if fu == uid {
				found = true
				if f.Upstream.Families[i].BaseURL == "" {
					f.Upstream.Families[i].BaseURL = globalURL
				}
				if f.Upstream.Families[i].APIKey == "" {
					f.Upstream.Families[i].APIKey = globalKey
				}
				if f.Upstream.Families[i].UpstreamModel == "" {
					up := m.UpstreamModel
					if up == "" {
						up = globalModel
					}
					f.Upstream.Families[i].UpstreamModel = up
				}
				if f.Upstream.Families[i].Provider == "" {
					f.Upstream.Families[i].Provider = "openai"
				}
				if f.Upstream.Families[i].Label == "" {
					f.Upstream.Families[i].Label = m.Family
				}
				if f.Upstream.Families[i].UID == "" {
					f.Upstream.Families[i].UID = uid
				}
				break
			}
		}
		if !found {
			up := m.UpstreamModel
			if up == "" {
				up = globalModel
			}
			label := m.Family
			if label == "" {
				label = m.Label
			}
			f.Upstream.Families = append(f.Upstream.Families, FamilyConfig{
				UID: uid, Label: label, Provider: "openai",
				BaseURL: globalURL, APIKey: globalKey, UpstreamModel: up,
				ContextWindow: m.ContextWindow, MaxTokens: m.MaxTokens,
			})
		}
	}
}

// ProviderResolved 解析后的供应商连接信息。
type ProviderResolved struct {
	Provider      string
	BaseURL       string
	APIKey        string
	UpstreamModel string
	Headers       map[string]string
	TimeoutSec    int
	FamilyUID     string
	FamilyLabel   string
}

// ResolveProvider 仅从选中的 family/model 解析供应商；不再用“全局默认 model 名”覆盖 upstream_model。
func (f *File) ResolveProvider(selectedUID string) (ProviderResolved, bool) {
	var out ProviderResolved
	m, ok := f.FindModel(selectedUID)
	if !ok {
		return out, false
	}
	uid := m.FamilyUID
	if uid == "" {
		uid = slugID(m.Family)
	}
	var fam FamilyConfig
	for _, fc := range f.Upstream.Families {
		fu := fc.UID
		if fu == "" {
			fu = slugID(fc.Label)
		}
		if fu == uid {
			fam = fc
			break
		}
	}
	// model 级覆盖 family
	out.FamilyUID = uid
	out.FamilyLabel = fam.Label
	if out.FamilyLabel == "" {
		out.FamilyLabel = m.Family
	}
	out.Provider = NormalizeProvider(firstNonEmpty(m.Provider, fam.Provider, "openai"))
	out.BaseURL = firstNonEmpty(m.BaseURL, fam.BaseURL, f.Upstream.BaseURL)
	out.APIKey = firstNonEmpty(m.APIKey, fam.APIKey, f.Upstream.APIKey)
	out.UpstreamModel = firstNonEmpty(m.UpstreamModel, fam.UpstreamModel, m.ID)
	out.TimeoutSec = m.TimeoutSec
	if out.TimeoutSec <= 0 {
		out.TimeoutSec = fam.TimeoutSec
	}
	if out.TimeoutSec <= 0 {
		out.TimeoutSec = f.Upstream.TimeoutSec
	}
	out.Headers = map[string]string{}
	for k, v := range f.Upstream.Headers {
		out.Headers[k] = v
	}
	for k, v := range fam.Headers {
		out.Headers[k] = v
	}
	for k, v := range m.Headers {
		out.Headers[k] = v
	}
	return out, strings.TrimSpace(out.BaseURL) != "" && strings.TrimSpace(out.UpstreamModel) != ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
