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
	direct := []string{
		"你是谁", "您是谁", "你叫什么", "你的名字", "你是什么", "你是哪位",
		"什么模型", "哪个模型", "哪种模型", "模型名称", "模型名字", "模型版本",
		"真实模型", "实际模型", "底层模型", "底模", "基座模型", "大模型", "语言模型",
		"谁训练的", "谁开发的", "哪家公司", "哪个公司", "谁创造了你", "谁做的你",
		"whatmodel", "whichmodel", "modelname", "modelversion", "underlyingmodel",
		"basemodel", "actualmodel", "realmodel", "whatpowersyou", "whatllm",
		"whoareyou", "what are you", "whomadeyou", "whobuiltyou", "youridentity",
		"que modelo", "qué modelo", "quelmodele", "quel modèle", "welchesmodell",
	}
	for _, marker := range direct {
		if strings.Contains(normalized, normalizeIdentityText(marker)) {
			return true
		}
	}

	identityWords := []string{"model", "llm", "模型", "底模", "身份", "identity", "architecture", "架构"}
	questionWords := []string{"what", "which", "name", "version", "real", "actual", "under", "谁", "什么", "哪个", "名称", "版本", "真实", "实际", "底层"}
	return containsAny(normalized, identityWords) && containsAny(normalized, questionWords)
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
