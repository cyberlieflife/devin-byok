package localapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/pbwire"
)

const (
	// ChatMessageSource
	chatSourceUser    = 1
	chatSourceSystem  = 2
	chatSourceUnknown = 3 // 助手输出兜底

	// StopReason (api_server GetChatMessageResponse)
	stopReasonUnspecified  = 0
	stopReasonIncomplete   = 1
	stopReasonStopPattern  = 2 // 正常结束
	stopReasonMaxTokens    = 3
	stopReasonFunctionCall = 10
	stopReasonError        = 13

	// ModelPricingType
	// 注意：MODEL_PRICING_TYPE_BYOK(3) 在官方实现里会要求 legacy cascade，
	// 当前 Devin 默认非 legacy，会报 "can only be used in legacy mode"。
	// 因此本地模型改用 STATIC_CREDIT，走正常 Cascade 路径，聊天仍由本地上游承接。
	pricingStaticCredit = 1
	pricingBYOK         = 3

	// APIProvider / ModelType
	apiProviderOpenAICompatibleExternal = 10
	modelTypeChat                       = 2

	// TeamsTier
	teamsTierPro       = 2
	teamsTierDevinPro  = 16
	teamsTierDevinMax  = 17
	teamsTierDevinFree = 19
)

func modelUID(cfg *config.File) string {
	return cfg.DefaultModelID()
}

func modelOrAliasUID(uid string) []byte {
	// ModelOrAlias oneof choice: model_uid = field 3
	var moa []byte
	moa = pbwire.AppendString(moa, 3, uid)
	return moa
}

func buildAllowedModelConfig(cfg *config.File) []byte {
	// AllowedModelConfig: model_or_alias=1, credit_multiplier=2
	var b []byte
	b = pbwire.AppendMessage(b, 1, modelOrAliasUID(modelUID(cfg)))
	b = pbwire.AppendFloat32(b, 2, 0)
	return b
}

// buildTeamConfig 官方 TeamConfig：显式关闭 disable_fast_context 等门禁，伪装 Pro 团队。
func buildTeamConfig(cfg *config.File) []byte {
	_ = cfg
	var b []byte
	b = pbwire.AppendString(b, 1, "byok-local-team") // team_id
	b = pbwire.AppendBool(b, 5, true)                // allow_mcp_servers
	b = pbwire.AppendBool(b, 15, false)              // disable_tool_calls
	b = pbwire.AppendBool(b, 26, false)              // disable_tool_call_execution_outside_workspace
	b = pbwire.AppendBool(b, 28, false)              // disable_deepwiki
	b = pbwire.AppendBool(b, 31, false)              // disable_codemaps
	b = pbwire.AppendString(b, 32, "all")            // allow_codemap_sharing
	b = pbwire.AppendBool(b, 33, false)              // disable_fast_context ★
	b = pbwire.AppendBool(b, 34, false)              // disable_lifeguard
	return b
}

func buildPlanInfo(cfg *config.File) []byte {
	var b []byte
	// teams_tier = DEVIN_PRO，避免 FREE 额度门禁
	b = pbwire.AppendEnum(b, 1, teamsTierDevinPro)
	b = pbwire.AppendString(b, 2, "Devin BYOK Pro")
	b = pbwire.AppendBool(b, 3, true)    // has_autocomplete_fast_mode
	b = pbwire.AppendBool(b, 4, true)    // allow_sticky_premium_models
	b = pbwire.AppendBool(b, 5, true)    // has_forge_access
	b = pbwire.AppendInt64(b, 6, 999999) // max_num_premium_chat_messages
	b = pbwire.AppendInt64(b, 7, 200000) // max_num_chat_input_tokens
	b = pbwire.AppendInt64(b, 8, 100000)
	b = pbwire.AppendInt64(b, 9, 100)
	b = pbwire.AppendInt64(b, 10, 1000000)
	b = pbwire.AppendBool(b, 15, true) // allow_premium_command_models
	b = pbwire.AppendBool(b, 16, false)
	b = pbwire.AppendBool(b, 17, true)
	b = pbwire.AppendBool(b, 18, true) // can_buy_more_credits
	b = pbwire.AppendBool(b, 19, true) // cascade_web_search_enabled
	b = pbwire.AppendBool(b, 23, true) // has_tab_to_jump
	b = pbwire.AppendBool(b, 25, true)
	b = pbwire.AppendBool(b, 27, true)
	b = pbwire.AppendBool(b, 28, true)
	b = pbwire.AppendBool(b, 29, true)
	b = pbwire.AppendBool(b, 31, true)    // browser_enabled
	b = pbwire.AppendBool(b, 32, true)    // has_paid_features
	b = pbwire.AppendBool(b, 34, true)    // is_devin
	b = pbwire.AppendInt32(b, 12, 999999) // monthly_prompt_credits
	b = pbwire.AppendInt32(b, 13, 999999) // monthly_flow_credits
	// cascade_allowed_models_config repeated = 21
	b = pbwire.AppendMessage(b, 21, buildAllowedModelConfig(cfg))
	// devin_info = 33
	var di []byte
	di = pbwire.AppendBool(di, 1, true) // can_use_cascade
	di = pbwire.AppendBool(di, 2, true) // can_use_cli
	di = pbwire.AppendBool(di, 3, true) // is_admin
	di = pbwire.AppendString(di, 8, "BYOK Local")
	b = pbwire.AppendMessage(b, 33, di)
	// default_team_config = 24（含 disable_fast_context=false）
	b = pbwire.AppendMessage(b, 24, buildTeamConfig(cfg))
	return b
}

func buildPlanStatus(cfg *config.File) []byte {
	var b []byte
	b = pbwire.AppendMessage(b, 1, buildPlanInfo(cfg))
	// available_* credits — 用 int64 更贴近 schema
	b = pbwire.AppendInt64(b, 8, 999999)
	b = pbwire.AppendInt64(b, 9, 999999)
	b = pbwire.AppendInt64(b, 4, 999999)
	b = pbwire.AppendInt64(b, 7, 0)
	b = pbwire.AppendInt64(b, 5, 0)
	b = pbwire.AppendInt64(b, 6, 0)
	b = pbwire.AppendInt32(b, 14, 100) // daily_quota_remaining_percent
	b = pbwire.AppendInt32(b, 15, 100) // weekly_quota_remaining_percent
	return b
}

// modelEntries …
func modelEntries(cfg *config.File) []config.ModelEntry {
	return cfg.ModelList()
}

// ClientModelConfig for model picker
func buildClientModelConfig(cfg *config.File) []byte {
	id := modelUID(cfg)
	if m, ok := cfg.FindModel(id); ok {
		return buildClientModelConfigEntry(cfg, m.ID, m.Label, &m)
	}
	return buildClientModelConfigFor(cfg, id, "BYOK - "+id)
}

// buildClientModelConfigFor 按指定 id/label 构造模型项。
// entry 可为 nil（仅用 uid/label）。
func buildClientModelConfigFor(cfg *config.File, uid, label string) []byte {
	var entry *config.ModelEntry
	for i := range cfg.ModelList() {
		if cfg.ModelList()[i].ID == uid {
			e := cfg.ModelList()[i]
			entry = &e
			break
		}
	}
	return buildClientModelConfigEntry(cfg, uid, label, entry)
}

func buildClientModelConfigEntry(cfg *config.File, uid, label string, entry *config.ModelEntry) []byte {
	if label == "" {
		label = "BYOK - " + uid
	}
	thinking := config.NormalizeThinkingLevel(cfg.Upstream.Thinking.Default)
	if thinking == "" {
		thinking = "medium"
	}
	family := uid
	familyUID := uid
	order := config.DevinFamilyOrder(thinking)
	ctxWin := 128000
	maxOut := 8192
	if entry != nil {
		if entry.Thinking != "" {
			thinking = entry.Thinking
		}
		if entry.Family != "" {
			family = entry.Family
		}
		if entry.FamilyUID != "" {
			familyUID = entry.FamilyUID
		} else if entry.Family != "" {
			// 与 config.slug 一致的后备：用 upstream 或 family
			familyUID = entry.ResolveUpstream()
			if entry.FamilyUID == "" && entry.Family != "" {
				// FamilyUID 应在 normalize 时填好；这里再兜底
				familyUID = entry.FamilyUID
				if familyUID == "" {
					familyUID = entry.ResolveUpstream()
				}
			}
		}
		if entry.FamilyUID != "" {
			familyUID = entry.FamilyUID
		}
		order = entry.FamilyOrder
		if entry.ContextWindow > 0 {
			ctxWin = entry.ContextWindow
		}
		if entry.MaxTokens > 0 {
			maxOut = entry.MaxTokens
		}
		if entry.Label != "" {
			label = entry.Label
		}
	}
	// 官方同族：model_info.model_family_uid 相同；label 形如 "Family Effort"
	// 若 label 仍是 BYOK 长名且有 family+thinking，生成更接近官方的短 label
	if entry != nil && entry.Family != "" && entry.Thinking != "" {
		// 保留用户自定义 label；用户已写完整 label 时不改
	}

	var b []byte
	b = pbwire.AppendString(b, 1, label)
	b = pbwire.AppendMessage(b, 2, modelOrAliasUID(uid))
	b = pbwire.AppendString(b, 22, uid)
	b = pbwire.AppendFloat32(b, 3, 0.5)
	b = pbwire.AppendBool(b, 4, false) // enabled
	b = pbwire.AppendBool(b, 5, true)
	b = pbwire.AppendBool(b, 6, false)
	b = pbwire.AppendBool(b, 7, false)
	b = pbwire.AppendEnum(b, 10, 1)
	b = pbwire.AppendBool(b, 11, thinking == "medium")
	for _, tier := range []int32{teamsTierDevinPro, teamsTierDevinMax, teamsTierDevinFree, teamsTierPro} {
		b = pbwire.AppendEnum(b, 12, tier)
	}
	b = pbwire.AppendEnum(b, 13, pricingStaticCredit)
	b = pbwire.AppendEnum(b, 14, apiProviderOpenAICompatibleExternal)
	// ClientModelConfig.max_tokens?UI ? contextLimit …
	b = pbwire.AppendInt32(b, 18, int32(ctxWin))
	b = pbwire.AppendBool(b, 20, false)
	b = pbwire.AppendEnum(b, 24, 1)
	b = pbwire.AppendString(b, 27, fmt.Sprintf("BYOK family=%s thinking=%s ctx=%d max_out=%d", family, thinking, ctxWin, maxOut))

	var feat []byte
	feat = pbwire.AppendBool(feat, 8, true)
	feat = pbwire.AppendBool(feat, 11, true)
	feat = pbwire.AppendBool(feat, 12, true)
	if thinking != "none" {
		feat = pbwire.AppendBool(feat, 15, true)
	}
	feat = pbwire.AppendBool(feat, 20, true)
	feat = pbwire.AppendBool(feat, 21, true)
	feat = pbwire.AppendBool(feat, 13, true)

	var mi []byte
	mi = pbwire.AppendEnum(mi, 3, modelTypeChat)
	// model_info.max_tokens = 上下文窗口
	mi = pbwire.AppendInt32(mi, 4, int32(ctxWin)) // model_info.max_tokens = 上下文窗口
	mi = pbwire.AppendString(mi, 5, "CL100K_WITH_SPECIAL")
	mi = pbwire.AppendMessage(mi, 6, feat)
	mi = pbwire.AppendEnum(mi, 7, apiProviderOpenAICompatibleExternal)
	mi = pbwire.AppendString(mi, 8, uid)
	mi = pbwire.AppendBool(mi, 9, true)
	if cfg.Upstream.BaseURL != "" {
		mi = pbwire.AppendString(mi, 11, cfg.Upstream.BaseURL)
	}
	mi = pbwire.AppendString(mi, 12, uid)
	// max_output_tokens
	mi = pbwire.AppendInt32(mi, 13, int32(maxOut)) // model_info.max_output_tokens = 最大输出
	mi = pbwire.AppendString(mi, 17, uid)
	mi = pbwire.AppendString(mi, 20, "strawberry-pancake")
	mi = pbwire.AppendString(mi, 20, "swe-1p6")
	// 关键：同族共享 model_family_uid
	mi = pbwire.AppendString(mi, 23, familyUID)
	b = pbwire.AppendMessage(b, 23, mi)

	// model_family_metadata = 30
	var fam []byte
	fam = pbwire.AppendString(fam, 1, family)
	var val []byte
	// 官方 Low 可省略 order；我们始终写 order 更稳
	val = pbwire.AppendInt32(val, 1, int32(order))
	val = pbwire.AppendString(val, 2, config.DevinThinkingName(thinking))
	var ent []byte
	ent = pbwire.AppendString(ent, 1, "Reasoning Effort")
	ent = pbwire.AppendMessage(ent, 2, val)
	fam = pbwire.AppendMessage(fam, 2, ent)
	if thinking == "medium" {
		fam = pbwire.AppendBool(fam, 3, true) // is_default on metadata
	}
	b = pbwire.AppendMessage(b, 30, fam)
	// is_default_model_in_family = 31
	b = pbwire.AppendBool(b, 31, thinking == "medium")
	return b
}

func buildClientModelSort(cfg *config.File) []byte {
	// 官方 sort 放的是“族默认档”label，不是所有 variant
	var group []byte
	group = pbwire.AppendString(group, 1, "BYOK")
	seenFam := map[string]bool{}
	for _, m := range modelEntries(cfg) {
		// 每个 family 只放 medium（或第一个）
		key := m.FamilyUID
		if key == "" {
			key = m.Family
		}
		if seenFam[key] {
			continue
		}
		if m.Thinking != "medium" {
			// 先跳过非 medium；若某族没有 medium，后面补
			continue
		}
		seenFam[key] = true
		group = pbwire.AppendString(group, 2, m.Label)
	}
	// 没有 medium 的族补第一个
	for _, m := range modelEntries(cfg) {
		key := m.FamilyUID
		if key == "" {
			key = m.Family
		}
		if seenFam[key] {
			continue
		}
		seenFam[key] = true
		group = pbwire.AppendString(group, 2, m.Label)
	}
	var sort []byte
	sort = pbwire.AppendString(sort, 1, "Default")
	sort = pbwire.AppendMessage(sort, 2, group)
	return sort
}

func buildCascadeModelConfigData(cfg *config.File) []byte {
	var b []byte
	for _, m := range modelEntries(cfg) {
		mm := m
		b = pbwire.AppendMessage(b, 1, buildClientModelConfigEntry(cfg, m.ID, m.Label, &mm))
	}
	b = pbwire.AppendMessage(b, 2, buildClientModelSort(cfg))
	var def []byte
	def = pbwire.AppendString(def, 3, cfg.DefaultModelID())
	b = pbwire.AppendMessage(b, 3, def)
	return b
}

func buildUserStatus(cfg *config.File) []byte {
	var b []byte
	b = pbwire.AppendBool(b, 1, true) // pro
	b = pbwire.AppendBool(b, 2, true) // disable_telemetry
	// 混合模式：展示官方登录身份；纯本地用 Fake*
	name := cfg.Auth.FakeName
	email := cfg.Auth.FakeEmail
	userID := cfg.Auth.FakeUserID
	if userID == "" {
		userID = "byok-local-user"
	}
	if !cfg.Features.PureLocal {
		oid := getOfficialIdentity()
		if oid.Name != "" {
			name = oid.Name
		}
		if oid.Email != "" {
			email = oid.Email
		}
		if oid.UserID != "" {
			userID = oid.UserID
		}
	}
	if name == "" {
		name = "BYOK Local"
	}
	if email == "" {
		email = "byok@local"
	}
	b = pbwire.AppendString(b, 3, name)
	b = pbwire.AppendString(b, 7, email)
	b = pbwire.AppendEnum(b, 10, teamsTierDevinPro) // teams_tier
	b = pbwire.AppendMessage(b, 13, buildPlanStatus(cfg))
	b = pbwire.AppendBool(b, 31, true) // has_used_windsurf
	// team_config = 32 (disable_fast_context=false)
	b = pbwire.AppendMessage(b, 32, buildTeamConfig(cfg))
	b = pbwire.AppendMessage(b, 33, buildCascadeModelConfigData(cfg))
	b = pbwire.AppendInt64(b, 35, 999999) // max_num_premium_chat_messages
	b = pbwire.AppendString(b, 36, userID)
	return b
}

func buildGetUserStatusResponse(cfg *config.File) []byte {
	var b []byte
	b = pbwire.AppendMessage(b, 1, buildUserStatus(cfg))
	b = pbwire.AppendMessage(b, 2, buildPlanInfo(cfg))
	return b
}

// buildRegisterUserResponse 伪造 SeatManagementService/RegisterUser 响应（3.7.16 登录链路）。
// RegisterUserResponse: api_key=1, name=2, api_server_url=3, redirect_url=4, team_options=5(repeated)
func buildRegisterUserResponse(cfg *config.File) []byte {
	apiKey := cfg.Auth.FakeAPIKey
	if apiKey == "" || !strings.HasPrefix(apiKey, "sk-ws-01-") {
		apiKey = "sk-ws-01-byoklocal-fake-key-0000"
	}
	name := cfg.Auth.FakeName
	if name == "" {
		name = "BYOK Local"
	}
	var b []byte
	b = pbwire.AppendString(b, 1, apiKey)
	b = pbwire.AppendString(b, 2, name)
	b = pbwire.AppendString(b, 3, cfg.Server.PublicBase+cfg.APIBasePath())
	return b
}

// buildLocalUserJWT 构造本地 JWT claims，避免官方 FREE JWT 把发消息门禁卡死。
// 客户端通常只 base64 解码 payload，不强制验签。
func buildLocalUserJWT(cfg *config.File) string {
	name := cfg.Auth.FakeName
	if name == "" {
		name = "BYOK Local"
	}
	email := cfg.Auth.FakeEmail
	if email == "" {
		email = "byok@local"
	}
	apiKey := cfg.Auth.FakeAPIKey
	if apiKey == "" {
		apiKey = "devin-byok-local-key"
	}
	header := map[string]any{"alg": "none", "typ": "JWT"}
	payload := map[string]any{
		"api_key":                       apiKey,
		"name":                          name,
		"email":                         email,
		"pro":                           true,
		"max_num_premium_chat_messages": 999999,
		"teams_tier":                    "TEAMS_TIER_DEVIN_PRO",
		"team_status":                   "USER_TEAM_STATUS_APPROVED",
		"team_id":                       "byok-local-team",
		"disable_codeium":               false,
		"disable_cli":                   false,
		"ignore_chat_telemetry_setting": true,
		"exp":                           time.Now().Add(24 * time.Hour * 365).Unix(),
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	enc := func(b []byte) string {
		return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "=")
	}
	// 无签名 JWT：header.payload.
	return fmt.Sprintf("%s.%s.", enc(hb), enc(pb))
}

func buildGetUserJwtResponse(cfg *config.File) []byte {
	var b []byte
	b = pbwire.AppendString(b, 1, buildLocalUserJWT(cfg))
	return b
}

func buildCheckChatCapacityResponse() []byte {
	var b []byte
	b = pbwire.AppendBool(b, 1, true) // has_capacity
	b = pbwire.AppendString(b, 2, "")
	b = pbwire.AppendInt32(b, 3, 0) // active_sessions
	return b
}

func buildCheckUserMessageRateLimitResponse() []byte {
	// has_capacity=true + 充足 remaining/max + resets
	var b []byte
	b = pbwire.AppendBool(b, 1, true)
	b = pbwire.AppendString(b, 2, "")
	b = pbwire.AppendInt32(b, 3, 999999) // messages_remaining
	b = pbwire.AppendInt32(b, 4, 999999) // max_messages
	b = pbwire.AppendInt64(b, 5, 86400)  // resets_in_seconds
	return b
}

func buildGetModelStatusesResponse(cfg *config.File) []byte {
	var b []byte
	for _, m := range modelEntries(cfg) {
		var info []byte
		info = pbwire.AppendString(info, 4, m.ID)
		info = pbwire.AppendString(info, 2, "ok")
		info = pbwire.AppendEnum(info, 3, 1) // SUCCESS
		b = pbwire.AppendMessage(b, 1, info)
	}
	return b
}

func buildGetCommandModelConfigsResponse(cfg *config.File) []byte {
	// available_command_models=1, default_command_model=2
	// 供 Generate Git Commit Message / Review Working Changes 等 Command 功能选模型
	var b []byte
	for _, m := range modelEntries(cfg) {
		mm := m
		b = pbwire.AppendMessage(b, 1, buildClientModelConfigEntry(cfg, m.ID, m.Label, &mm))
	}
	def := strings.TrimSpace(cfg.FeatureModelID("command"))
	if def == "" {
		def = cfg.DefaultModelID()
	}
	if def != "" {
		b = pbwire.AppendString(b, 2, def)
	}
	return b
}

func buildGetCascadeModelConfigsResponse(cfg *config.File) []byte {
	var b []byte
	for _, m := range modelEntries(cfg) {
		mm := m
		b = pbwire.AppendMessage(b, 1, buildClientModelConfigEntry(cfg, m.ID, m.Label, &mm))
	}
	b = pbwire.AppendMessage(b, 2, buildClientModelSort(cfg))
	var def []byte
	def = pbwire.AppendString(def, 3, cfg.DefaultModelID())
	b = pbwire.AppendMessage(b, 3, def)
	return b
}

func buildPingResponse() []byte { return []byte{} }

// GetChatMessageResponse with assistant action text
// buildGetChatMessageResponse 构造 api_server_pb.GetChatMessageResponse（流式 delta）。
//
//	1 message_id
//	2 timestamp
//	3 delta_text        最终可见回复
//	4 delta_tokens
//	5 stop_reason
//	9 delta_thinking    思考链（reasoning_content）
func buildGetChatMessageResponse(messageID, convID, text string, inProgress bool) []byte {
	return buildGetChatMessageDelta(messageID, text, "", inProgress)
}

// buildGetChatMessageDelta 可同时带正文与思考链。
func buildGetChatMessageDelta(messageID, text, thinking string, inProgress bool) []byte {
	var b []byte
	if messageID != "" {
		b = pbwire.AppendString(b, 1, messageID)
	}
	// google.protobuf.Timestamp { seconds=1 }
	var ts []byte
	ts = pbwire.AppendInt64(ts, 1, time.Now().Unix())
	b = pbwire.AppendMessage(b, 2, ts)

	if text != "" {
		b = pbwire.AppendString(b, 3, text) // delta_text
		tok := int32(len([]rune(text)))
		if tok < 1 {
			tok = 1
		}
		b = pbwire.AppendInt32(b, 4, tok)
	}
	if thinking != "" {
		b = pbwire.AppendString(b, 9, thinking) // delta_thinking
	}

	if inProgress {
		b = pbwire.AppendEnum(b, 5, stopReasonIncomplete)
	} else {
		b = pbwire.AppendEnum(b, 5, stopReasonStopPattern)
	}
	return b
}

// buildGetChatMessageErrorDelta 用 stop_reason=ERROR 返回可见错误文本。
// buildGetChatMessageToolDelta 将上游 tool_calls 映射为 api_server delta_tool_calls。
func buildGetChatMessageToolDelta(messageID string, calls []openaiToolCallView) []byte {
	var b []byte
	if messageID != "" {
		b = pbwire.AppendString(b, 1, messageID)
	}
	var ts []byte
	ts = pbwire.AppendInt64(ts, 1, time.Now().Unix())
	b = pbwire.AppendMessage(b, 2, ts)
	for _, c := range calls {
		var tc []byte
		if c.ID != "" {
			tc = pbwire.AppendString(tc, 1, c.ID)
		}
		if c.Name != "" {
			tc = pbwire.AppendString(tc, 2, c.Name)
		}
		if c.Arguments != "" {
			tc = pbwire.AppendString(tc, 3, c.Arguments)
		}
		b = pbwire.AppendMessage(b, 6, tc) // delta_tool_calls
	}
	// 工具增量过程中 incomplete；最终工具帧由调用方决定 stop
	b = pbwire.AppendEnum(b, 5, stopReasonIncomplete)
	return b
}

func buildGetChatMessageToolFinal(messageID string, calls []openaiToolCallView) []byte {
	var b []byte
	if messageID != "" {
		b = pbwire.AppendString(b, 1, messageID)
	}
	var ts []byte
	ts = pbwire.AppendInt64(ts, 1, time.Now().Unix())
	b = pbwire.AppendMessage(b, 2, ts)
	for _, c := range calls {
		var tc []byte
		if c.ID != "" {
			tc = pbwire.AppendString(tc, 1, c.ID)
		}
		if c.Name != "" {
			tc = pbwire.AppendString(tc, 2, c.Name)
		}
		if c.Arguments != "" {
			tc = pbwire.AppendString(tc, 3, c.Arguments)
		}
		b = pbwire.AppendMessage(b, 6, tc)
	}
	b = pbwire.AppendEnum(b, 5, stopReasonFunctionCall)
	return b
}

// openaiToolCallView 避免 stubs 直接依赖 openai 包循环；由 server 转换。
type openaiToolCallView struct {
	ID, Name, Arguments string
}

func buildGetChatMessageErrorDelta(messageID, text string) []byte {
	var b []byte
	if messageID != "" {
		b = pbwire.AppendString(b, 1, messageID)
	}
	var ts []byte
	ts = pbwire.AppendInt64(ts, 1, time.Now().Unix())
	b = pbwire.AppendMessage(b, 2, ts)
	if text == "" {
		text = "BYOK error"
	}
	b = pbwire.AppendString(b, 3, text)
	b = pbwire.AppendEnum(b, 5, stopReasonError)
	return b
}

// buildGetProfileDataResponse 返回空的可选用户资料。
// Devin 3.7.16 将 profile_data.field 1 解释为头像 URL，并会把它交给
// Jimp 读取；写入显示名等普通文本会触发 "Could not load Buffer from URL"。
func buildGetProfileDataResponse() []byte {
	return nil
}

func buildGetStatusResponse() []byte                   { return []byte{} }
func buildGetDefaultWorkflowTemplatesResponse() []byte { return []byte{} }

// buildGetAllAcpRegistriesResponse mirrors the Devin air-gapped builtin
// registry: a local "devin-cli" agent started via `devin acp`. Without a
// registry entry the agent selector stays empty and ACP sessions cannot be
// created, so an empty response effectively breaks agent mode.
func buildGetAllAcpRegistriesResponse() []byte {
	registry := `{"version":"1.0.0","agents":[{"id":"devin-cli","name":"Devin","version":"1.0.0","description":"Devin AI coding agent","authors":["Cognition AI"],"license":"proprietary","cognition.ai/featured":true,"cognition.ai/bundled":true,"distribution":{"binary":{` +
		`"darwin-aarch64":{"archive":"","cmd":"devin","args":["acp"]},` +
		`"darwin-x86_64":{"archive":"","cmd":"devin","args":["acp"]},` +
		`"linux-aarch64":{"archive":"","cmd":"devin","args":["acp"]},` +
		`"linux-x86_64":{"archive":"","cmd":"devin","args":["acp"]},` +
		`"windows-aarch64":{"archive":"","cmd":"devin.exe","args":["acp"]},` +
		`"windows-x86_64":{"archive":"","cmd":"devin.exe","args":["acp"]}` +
		`}}}]}`
	var b []byte
	b = pbwire.AppendString(b, 1, registry)
	return b
}

// buildGetUnleashDataResponse 强制开启 cascade-find-code-context，使 Fast Context 门控通过。
func buildGetUnleashDataResponse() []byte {
	var exp []byte
	// ExperimentConfig.force_enable_experiment_strings = 4 (repeated string)
	for _, flag := range []string{
		"cascade-find-code-context",
		"cascade-enable-conversation-search",
		"cascade-tool-calling-section-content",
	} {
		exp = pbwire.AppendString(exp, 4, flag)
	}
	exp = pbwire.AppendBool(exp, 7, true) // dev_mode
	var b []byte
	// context optional
	var ctx []byte
	ctx = pbwire.AppendString(ctx, 1, "byok-local-user")
	b = pbwire.AppendMessage(b, 1, ctx)
	b = pbwire.AppendMessage(b, 2, exp)
	return b
}

func buildShouldEnableUnleashResponse() []byte {
	var b []byte
	b = pbwire.AppendBool(b, 1, true) // should_enable
	return b
}
