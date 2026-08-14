package localapi

import (
	"strings"
	"unicode"

	"devin-byok/internal/config"
)

func modelDisplayName(cfg *config.File, modelID string) string {
	if model, ok := cfg.FindModel(modelID); ok {
		if label := strings.TrimSpace(model.Label); label != "" {
			return label
		}
		if family := strings.TrimSpace(model.Family); family != "" {
			level := config.DevinThinkingName(model.Thinking)
			if level != "" && !strings.EqualFold(level, "none") {
				return family + " " + level
			}
			return family
		}
	}
	return strings.TrimSpace(modelID)
}

func isModelIdentityQuestion(text string) bool {
	normalized := normalizeIdentityText(text)
	if normalized == "" {
		return false
	}
	// 结构完整的身份问句或强身份专有词：命中即拦截（这些模式本身无技术歧义）。
	// "真实底模/底层模型" 等专有词几乎只出现在身份提问中，单独出现也拦截；
	// 而 "模型/架构" 等弱词不在此列，避免误伤技术讨论。
	direct := []string{
		"你是谁", "您是谁", "你叫什么", "你的名字", "你是哪位",
		"谁训练的", "谁开发的", "谁创造了你", "谁做的你",
		"真实底模", "实际底模", "底模是什么", "底层模型",
		"whatpowersyou", "whoareyou", "what are you", "whomadeyou", "whobuiltyou", "youridentity",
		"que modelo", "qué modelo", "modelo eres", "quelmodele", "quel modèle", "welchesmodell",
	}
	for _, marker := range direct {
		if strings.Contains(normalized, normalizeIdentityText(marker)) {
			return true
		}
	}

	// 组合规则：身份词（模型/底模/架构…）必须同时出现自称代词（你/您/you/who），
	// 才判定为身份提问。宽泛的 "什么模型/模型名称" 等二义子串不再单独触发——
	// 例如 "我的模型为什么崩溃"（无"你"）是真实技术问题，不应被本地身份应答拦截。
	identityWords := []string{"model", "llm", "模型", "底模", "身份", "identity", "architecture", "架构"}
	selfRefWords := []string{"你", "您", "you", "who", "your", "你的", "您的"}
	return containsAny(normalized, identityWords) && containsAny(normalized, selfRefWords)
}

func normalizeIdentityText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r > unicode.MaxASCII {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsAny(text string, values []string) bool {
	for _, value := range values {
		if strings.Contains(text, normalizeIdentityText(value)) {
			return true
		}
	}
	return false
}

func modelIdentityAnswer(name, userText string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "当前选择的模型"
	}
	low := strings.ToLower(userText)
	if strings.Contains(low, "what") || strings.Contains(low, "which") || strings.Contains(low, "who") || strings.Contains(low, "model") {
		return "I am " + name + "."
	}
	return "我是 " + name + "。"
}
