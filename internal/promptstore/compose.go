package promptstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"devin-byok/internal/upstream/openai"
)

// ReliabilityPrompt is intentionally short and operational. It asks for
// evidence and verification without requesting hidden chain-of-thought.
const ReliabilityPrompt = `# Reliability Contract

你的目标是交付正确、可执行、可验证的结果，而不是快速生成看似合理的答案。

## Before acting

识别用户真正目标、已知事实与未知信息、约束条件和完成标准。
复杂任务先形成简短计划，然后执行，不要只停留在建议。

## Evidence

优先检查真实文件、代码、日志、工具结果和官方资料。
区分已验证事实、基于证据的推断和未验证假设。
不得虚构文件内容、命令结果、测试结果、引用或工具调用。

## Execution

代码任务先读取相关代码和项目约定，只做最小必要修改。
有工具时，修改后运行适当的格式、语法、类型或测试检查。
没有实际运行时，不得声称测试通过。

## Verification

交付前逐项检查完成标准、边界条件、反例和未声明副作用。
无法完成时，明确说明阻塞点、已验证内容和剩余不确定性。`

const codingExecutionPrompt = `# Coding Execution

对代码、配置和构建任务按以下顺序工作：
1. 先提取目标、约束、输入输出契约和验收标准；复杂任务列出简短计划。
2. 读取相关文件和项目约定，定位根因后做最小修改，不擅自改变未要求的行为。
3. 修改后先执行格式、语法、类型或构建检查，再运行针对性测试。
4. 测试失败时根据真实错误修复，最多重复三轮；不要用跳过测试或静默兜底掩盖失败。
5. 交付时列出实际改动、实际执行的命令和仍未验证的风险。`

const debuggingPrompt = `# Debugging Workflow

先稳定复现问题并记录最小输入、实际输出和错误位置。
区分症状、触发条件和根因；每次只验证一个关键假设。
修复后覆盖原始复现、正常路径、空值/边界值和回归场景。
如果无法复现，明确说明已检查的证据，不要把猜测写成根因。`

const researchPrompt = `# Evidence-Based Research

先明确问题的时间范围、比较维度和结论标准。
优先一手资料和官方文档，核对发布日期与适用版本。
区分来源原文、基于来源的推断和未确认信息；冲突时说明取舍依据。
不要编造链接、引用、版本号或“最新”结论。`

const reviewPrompt = `# Review Contract

先独立理解目标和变更影响，再按严重性检查正确性、回归、边界条件、安全性和可维护性。
每个问题都给出文件位置、触发条件和实际影响；没有证据支持的疑点标为风险而不是缺陷。
优先报告会导致错误行为或数据损失的问题，最后说明测试缺口和剩余风险。`

const explainPrompt = `# Explanation Contract

先给结论，再用当前代码、数据或官方资料解释原因。
区分事实、推断和示例；用最小必要例子说明边界条件。
不要把不确定的实现细节说成确定事实。`

const outputContractPrompt = `# Output Contract

严格遵守用户要求的输出格式和范围：如果用户要求 only、单个值、YES/NO 或 JSON，就只输出该格式，不加 Markdown 围栏、前后解释或免责声明。
不要把常见默认行为替换成用户明确给出的规则；有歧义时采用最小假设并在允许的格式内回答。
用户列出多个验收条件时，必须逐项覆盖，不能只回答最容易的一项。`

const constraintAuditPrompt = `# Constraint Audit

把用户明确写出的限定词当作硬约束，不用常见默认行为替换它们。特别核对：only/仅、exact/恰好、inclusive/包含端点、overlap/重叠、adjacent/相邻、nil/empty/空值、invalid/非法输入和错误处理方式。
回答前用最小反例检查边界、类型和条件组合；如果题目定义了语义，严格按题目定义作答，不擅自补充未要求的规则。`

const fastContextPrompt = `# Fast Context Contract

你是快速检索代理。只搜索、读取和整理真实工作区证据，不修改文件。
优先使用检索工具，再读取最相关的文件和片段。
最终只列出实际观察到的路径、行号或明确的“未找到”；不得虚构路径或代码。`

const deepWikiPrompt = `# DeepWiki Contract

根据实际代码证据生成准确、紧凑的符号或模块说明。
只描述已观察到的职责、调用关系和边界；不生成代码修改方案，不编造路径或依赖。`

const codeMapPrompt = `# CodeMap Contract

输出必须严格符合调用方要求的 JSON 结构，不要 Markdown 围栏或额外说明。
只使用实际观察到的文件路径和模块关系；未知位置留空，不要猜测行号。`

// ComposeContext describes the request at the prompt boundary. Task may be
// left empty; DetectTask(UserText) is then used deterministically.
type ComposeContext struct {
	Route        string
	ModelID      string
	Family       string
	UserText     string
	Task         string
	HasTools     bool
	HasWorkspace bool
	QualityMode  string // fast|balanced|verified
	// QualityEnabled is nil for legacy callers and means enabled. A non-nil
	// false value disables all prompt enhancement, including custom profiles.
	QualityEnabled *bool
}

type ComposeResult struct {
	Messages    []openai.ChatMessage
	ProfileIDs  []string
	Warnings    []string
	Hash        string
	Route       string
	Task        string
	QualityMode string
}

// ComposeMessages combines original messages and matching profiles. It never
// emits hidden reasoning; only the operational contract and final response are
// sent to the client.
func ComposeMessages(msgs []openai.ChatMessage, ctx ComposeContext) ComposeResult {
	profiles, _ := Load()
	return composeWithProfiles(msgs, ctx, profiles)
}

func composeWithProfiles(msgs []openai.ChatMessage, ctx ComposeContext, custom []Prompt) ComposeResult {
	ctx.Route = normalizeRoute(ctx.Route)
	if ctx.Route == "fast_context" {
		ctx.QualityMode = "fast"
	}
	ctx.QualityMode = normalizeQualityMode(ctx.QualityMode)
	if ctx.Task == "" {
		ctx.Task = DetectTask(ctx.UserText)
	} else {
		ctx.Task = normalizeTask(ctx.Task)
	}
	if ctx.QualityEnabled != nil && !*ctx.QualityEnabled {
		base := cloneMessages(msgs)
		return ComposeResult{Messages: base, Hash: hashMessages(base), Route: ctx.Route, Task: ctx.Task, QualityMode: ctx.QualityMode}
	}

	profiles := builtinProfiles(ctx)
	profiles = append(profiles, matchingCustomProfiles(custom, ctx)...)
	profiles = dedupeProfiles(profiles)
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].Priority != profiles[j].Priority {
			return profiles[i].Priority < profiles[j].Priority
		}
		return profiles[i].ID < profiles[j].ID
	})

	var replace *Prompt
	var prepend, appendProfiles []Prompt
	for i := range profiles {
		p := profiles[i]
		switch p.Mode {
		case ModeReplace:
			if replace == nil || p.Priority >= replace.Priority {
				copyP := p
				replace = &copyP
			}
		case ModePrepend:
			prepend = append(prepend, p)
		default:
			appendProfiles = append(appendProfiles, p)
		}
	}

	base := cloneMessages(msgs)
	warnings := make([]string, 0, 2)
	if replace != nil {
		warnings = append(warnings, fmt.Sprintf("profile %q uses replace and may override the upstream system prompt", replace.ID))
		if strings.Contains(strings.ToLower(replace.Body), "tool") {
			warnings = append(warnings, fmt.Sprintf("profile %q replace body may remove the tool contract", replace.ID))
		}
		base = replaceFirstSystem(base, replace.Body)
	}

	// Keep the original system prompt first. Explicit prepend profiles retain
	// their legacy meaning by appearing before the generated profile section.
	for i := len(prepend) - 1; i >= 0; i-- {
		base = injectNote(base, prepend[i].Body, true)
	}
	for _, p := range appendProfiles {
		if strings.TrimSpace(p.Body) == "" {
			continue
		}
		base = injectNote(base, p.Body, false)
	}

	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if strings.TrimSpace(p.Body) != "" {
			ids = append(ids, p.ID)
		}
	}
	return ComposeResult{Messages: base, ProfileIDs: ids, Warnings: uniqueStrings(warnings), Hash: hashMessages(base), Route: ctx.Route, Task: ctx.Task, QualityMode: ctx.QualityMode}
}

func builtinProfiles(ctx ComposeContext) []Prompt {
	if ctx.Route == "fast_context" {
		return []Prompt{{ID: "fast-context", Title: "Fast Context", Body: fastContextPrompt, Enabled: true, Mode: ModeAppend, Priority: 60, Builtin: true}}
	}
	profiles := make([]Prompt, 0, 6)
	if ctx.Route == "chat" {
		if ctx.HasTools {
			// Keep the tool contract ahead of general guidance so later profiles
			// cannot accidentally make file edits through shell commands.
			profiles = append(profiles, Prompt{ID: "file-tools", Title: "File Tools", Body: BuiltinFileToolsPrompt, Enabled: true, Mode: ModeAppend, Priority: 5, Builtin: true})
		}
		if ctx.QualityMode != "fast" {
			profiles = append(profiles, Prompt{ID: "core-reliability", Title: "Core Reliability", Body: ReliabilityPrompt, Enabled: true, Mode: ModeAppend, Priority: 10, Builtin: true})
			profiles = append(profiles, Prompt{ID: "constraint-audit", Title: "Constraint Audit", Body: constraintAuditPrompt, Enabled: true, Mode: ModeAppend, Priority: 35, Builtin: true})
		}
		// This short contract remains active in fast mode because format errors
		// break Devin's structured routes even when deep reasoning is disabled.
		profiles = append(profiles, Prompt{ID: "output-contract", Title: "Output Contract", Body: outputContractPrompt, Enabled: true, Mode: ModeAppend, Priority: 90, Builtin: true})
		if ctx.QualityMode == "verified" {
			profiles = append(profiles, Prompt{ID: "verification", Title: "Verification", Body: `# Verification Gate

重要修改完成前，必须获得机械检查或测试证据。若检查失败，先修复再交付；若无法执行，明确标记未验证。`, Enabled: true, Mode: ModeAppend, Priority: 25, Builtin: true})
		}
		if ctx.QualityMode != "fast" {
			switch ctx.Task {
			case "coding":
				profiles = append(profiles, Prompt{ID: "coding-execution", Title: "Coding Execution", Body: codingExecutionPrompt, Enabled: true, Mode: ModeAppend, Priority: 40, Builtin: true})
			case "debug":
				profiles = append(profiles, Prompt{ID: "debugging", Title: "Debugging", Body: debuggingPrompt, Enabled: true, Mode: ModeAppend, Priority: 40, Builtin: true})
			case "research":
				profiles = append(profiles, Prompt{ID: "research-evidence", Title: "Research Evidence", Body: researchPrompt, Enabled: true, Mode: ModeAppend, Priority: 40, Builtin: true})
			case "review":
				profiles = append(profiles, Prompt{ID: "review-risk", Title: "Review Risk", Body: reviewPrompt, Enabled: true, Mode: ModeAppend, Priority: 40, Builtin: true})
			case "explain":
				profiles = append(profiles, Prompt{ID: "explain", Title: "Explanation", Body: explainPrompt, Enabled: true, Mode: ModeAppend, Priority: 40, Builtin: true})
			}
		}
	}
	if ctx.Route == "fast_context" {
		profiles = append(profiles, Prompt{ID: "fast-context", Title: "Fast Context", Body: fastContextPrompt, Enabled: true, Mode: ModeAppend, Priority: 60, Builtin: true})
	} else if ctx.Route == "deepwiki" {
		profiles = append(profiles, Prompt{ID: "deepwiki", Title: "DeepWiki", Body: deepWikiPrompt, Enabled: true, Mode: ModeAppend, Priority: 60, Builtin: true})
	} else if ctx.Route == "codemap" {
		profiles = append(profiles, Prompt{ID: "codemap", Title: "CodeMap", Body: codeMapPrompt, Enabled: true, Mode: ModeAppend, Priority: 60, Builtin: true})
	}
	if ctx.HasWorkspace && ctx.Route == "chat" {
		profiles = append(profiles, Prompt{ID: "workspace-evidence", Title: "Workspace Evidence", Body: "工作区上下文可用时，只引用实际读取到的路径、内容和命令结果。", Enabled: true, Mode: ModeAppend, Priority: 70, Builtin: true})
	}
	return profiles
}

func matchingCustomProfiles(custom []Prompt, ctx ComposeContext) []Prompt {
	out := make([]Prompt, 0, len(custom))
	for _, p := range custom {
		if !p.Enabled || strings.TrimSpace(p.Body) == "" || p.Builtin {
			continue
		}
		if profileMatches(p, ctx) {
			if p.Mode == "" {
				p.Mode = ModeAppend
			}
			out = append(out, p)
		}
	}
	return out
}

func profileMatches(p Prompt, ctx ComposeContext) bool {
	scope := strings.ToLower(strings.TrimSpace(p.Scope))
	if scope == "" && len(p.Routes) == 0 && len(p.Models) == 0 && len(p.Tasks) == 0 {
		return true
	}
	if scope == "global" {
		return true
	}
	if len(p.Routes) > 0 && !containsFold(p.Routes, ctx.Route) {
		return false
	}
	if len(p.Models) > 0 && !containsFold(p.Models, ctx.ModelID) && !containsFold(p.Models, ctx.Family) {
		return false
	}
	if len(p.Tasks) > 0 && !containsFold(p.Tasks, ctx.Task) {
		return false
	}
	switch scope {
	case "model":
		return containsFold(p.Models, ctx.ModelID) || containsFold(p.Models, ctx.Family)
	case "route":
		return containsFold(p.Routes, ctx.Route)
	case "task":
		return containsFold(p.Tasks, ctx.Task)
	default:
		return scope == "" || scope == "global"
	}
}

func dedupeProfiles(in []Prompt) []Prompt {
	out := make([]Prompt, 0, len(in))
	seenID, seenBody := map[string]bool{}, map[string]bool{}
	for _, p := range in {
		id := strings.TrimSpace(p.ID)
		body := strings.TrimSpace(p.Body)
		if id != "" && seenID[id] || body != "" && seenBody[body] {
			continue
		}
		if id != "" {
			seenID[id] = true
		}
		if body != "" {
			seenBody[body] = true
		}
		out = append(out, p)
	}
	return out
}

func replaceFirstSystem(msgs []openai.ChatMessage, body string) []openai.ChatMessage {
	out := cloneMessages(msgs)
	for i := range out {
		if out[i].Role == "system" {
			out[i].Content = strings.TrimSpace(body)
			return out
		}
	}
	return append([]openai.ChatMessage{{Role: "system", Content: strings.TrimSpace(body)}}, out...)
}

func cloneMessages(in []openai.ChatMessage) []openai.ChatMessage {
	out := make([]openai.ChatMessage, len(in))
	copy(out, in)
	return out
}

func injectNote(msgs []openai.ChatMessage, note string, front bool) []openai.ChatMessage {
	note = strings.TrimSpace(note)
	if note == "" {
		return msgs
	}
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(openai.TextContent(m.Content), note) {
			return msgs
		}
	}
	sys := openai.ChatMessage{Role: "system", Content: note}
	if front {
		return append([]openai.ChatMessage{sys}, msgs...)
	}
	if len(msgs) > 0 && msgs[0].Role == "system" {
		out := make([]openai.ChatMessage, 0, len(msgs)+1)
		insertAt := 1
		for insertAt < len(msgs) && msgs[insertAt].Role == "system" {
			insertAt++
		}
		out = append(out, msgs[:insertAt]...)
		out = append(out, sys)
		out = append(out, msgs[insertAt:]...)
		return out
	}
	return append([]openai.ChatMessage{sys}, msgs...)
}

func hashMessages(msgs []openai.ChatMessage) string {
	b, _ := json.Marshal(msgs)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func containsFold(values []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, v := range values {
		if strings.ToLower(strings.TrimSpace(v)) == wanted && wanted != "" {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func normalizeRoute(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "fast", "fast-context", "instant_context", "instant-context":
		return "fast_context"
	case "wiki":
		return "deepwiki"
	case "map":
		return "codemap"
	case "":
		return "chat"
	default:
		return v
	}
}

func normalizeQualityMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "fast":
		return "fast"
	case "verified", "verify", "strict":
		return "verified"
	default:
		return "balanced"
	}
}

func normalizeTask(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "coding", "debug", "research", "review", "explain", "general":
		return v
	default:
		return DetectTask(v)
	}
}

// NormalizeTask exposes the Composer's task vocabulary to local callers.
func NormalizeTask(v string) string { return normalizeTask(v) }

// DetectTask is deterministic and intentionally conservative. It is a router,
// not a model call, so an unknown request falls back to general.
func DetectTask(text string) string {
	low := strings.ToLower(strings.TrimSpace(text))
	if low == "" {
		return "general"
	}
	groups := []struct {
		name string
		keys []string
	}{
		{"review", []string{"审查", "检查风险", "找 bug", "review", "code review"}},
		{"debug", []string{"报错", "错误", "崩溃", "失败", "异常", "bug", "debug", "crash", "cannot start", "无法启动"}},
		{"research", []string{"查询", "对比", "最新", "文档", "调查", "research", "compare"}},
		{"coding", []string{"实现", "添加", "修改", "重构", "接入", "编写", "implement", "refactor", "fix"}},
		{"explain", []string{"解释", "原理", "为什么", "说明", "explain", "why"}},
	}
	for _, group := range groups {
		for _, key := range group.keys {
			if strings.Contains(low, key) {
				return group.name
			}
		}
	}
	return "general"
}

// ApplyToMessages keeps the legacy API while using the new default composer.
func ApplyToMessages(msgs []openai.ChatMessage) []openai.ChatMessage {
	return ComposeMessages(msgs, ComposeContext{Route: "chat", HasTools: true, QualityMode: "balanced"}).Messages
}
