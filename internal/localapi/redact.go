package localapi

import "regexp"

// secretPatterns 用于在开启抓包时对提取出的可读字符串做尽力脱敏。
// 仅覆盖常见形态（authorization / Bearer / api_key / sk-*），不是完整安全边界。
var secretPatterns = []*regexp.Regexp{
	// authorization 头优先整体脱敏到行尾，避免与 bearer 重复脱敏
	regexp.MustCompile(`(?i)\b(authorization\s*[:=]\s*)[^\r\n]*`),
	regexp.MustCompile(`(?i)\b(bearer\s+)[a-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)\b(api[_-]?key\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`\b(sk-)[a-z0-9_-]{6,}\b`),
}

func redactSecrets(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "${1}[REDACTED]")
	}
	return s
}

func redactStrings(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = redactSecrets(s)
	}
	return out
}
