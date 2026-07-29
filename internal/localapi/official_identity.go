package localapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"devin-byok/internal/logx"
	"devin-byok/internal/pbwire"
)

// officialIdentity 混合模式下缓存官方 GetUserJwt 解析出的真实账号。
// GetUserStatus 仍本地伪装 Pro + 注入 BYOK 模型列表，但展示用真实 name/email。
type officialIdentity struct {
	Name   string
	Email  string
	UserID string
	APIKey string
	RawJWT string
}

var (
	officialIdentMu sync.RWMutex
	officialIdent   officialIdentity
)

func getOfficialIdentity() officialIdentity {
	officialIdentMu.RLock()
	defer officialIdentMu.RUnlock()
	return officialIdent
}

func setOfficialIdentity(id officialIdentity) {
	officialIdentMu.Lock()
	officialIdent = id
	officialIdentMu.Unlock()
	logx.Infof("official identity cached name=%q email=%q user=%q", id.Name, id.Email, id.UserID)
}

// rememberOfficialJWTFromProxyBody 解析官方 GetUserJwt 响应并缓存 claims。
func rememberOfficialJWTFromProxyBody(respBody []byte) {
	plain := maybeGunzip(respBody)
	jwt := extractJWTFromBytes(plain)
	if jwt == "" {
		logx.Warnf("official GetUserJwt: no JWT in %d-byte response", len(respBody))
		return
	}
	claims, err := decodeJWTPayload(jwt)
	if err != nil {
		logx.Warnf("official GetUserJwt: decode claims: %v", err)
		return
	}
	id := officialIdentity{RawJWT: jwt}
	id.Name = claimString(claims, "name", "display_name", "preferred_username")
	id.Email = claimString(claims, "email")
	id.APIKey = claimString(claims, "api_key", "apiKey")
	id.UserID = claimString(claims, "user_id", "userId", "sub", "id")
	if id.Name == "" && id.Email == "" && id.UserID == "" {
		logx.Warnf("official GetUserJwt: claims missing identity fields keys=%v", mapKeys(claims))
		return
	}
	setOfficialIdentity(id)
}

func claimString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				if strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func extractJWTFromBytes(plain []byte) string {
	for _, msg := range protoPayloadCandidates(plain) {
		for _, f := range pbwire.ParseFields(msg) {
			if f.Number == 1 && f.Wire == 2 && looksLikeJWT(string(f.Bytes)) {
				return string(f.Bytes)
			}
		}
	}
	return findJWTString(plain)
}

func protoPayloadCandidates(plain []byte) [][]byte {
	out := [][]byte{plain}
	i := 0
	for i+5 <= len(plain) {
		n := int(plain[i+1])<<24 | int(plain[i+2])<<16 | int(plain[i+3])<<8 | int(plain[i+4])
		if n <= 0 || i+5+n > len(plain) {
			break
		}
		out = append(out, plain[i+5:i+5+n])
		i += 5 + n
		if len(out) > 8 {
			break
		}
	}
	return out
}

func findJWTString(b []byte) string {
	s := string(b)
	start := strings.Index(s, "eyJ")
	for start >= 0 {
		rest := s[start:]
		end := 0
		for end < len(rest) {
			c := rest[end]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
				end++
				continue
			}
			break
		}
		tok := rest[:end]
		if looksLikeJWT(tok) {
			return tok
		}
		next := strings.Index(rest[1:], "eyJ")
		if next < 0 {
			break
		}
		start += 1 + next
	}
	return ""
}

func looksLikeJWT(s string) bool {
	parts := strings.Split(s, ".")
	return len(parts) >= 2 && strings.HasPrefix(s, "eyJ") && len(s) > 40
}

func decodeJWTPayload(jwt string) (map[string]any, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid jwt")
	}
	payload := parts[1]
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		pad := payload + strings.Repeat("=", (4-len(payload)%4)%4)
		raw, err = base64.URLEncoding.DecodeString(pad)
		if err != nil {
			return nil, err
		}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// chatUsesLocalBYOKModel 请求体是否点名了配置中的 BYOK 模型。
func chatUsesLocalBYOKModel(plain []byte, modelIDs []string) bool {
	raw := string(plain)
	if strings.Contains(raw, "-byok-") || strings.Contains(raw, "BYOK") {
		return true
	}
	ids := append([]string(nil), modelIDs...)
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if len(ids[j]) > len(ids[i]) {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && strings.Contains(raw, id) {
			return true
		}
	}
	return false
}


// looksLikeOfficialModelEnum 请求是否携带 Codeium 官方模型枚举名。
func looksLikeOfficialModelEnum(plain []byte) bool {
	raw := string(plain)
	keys := []string{
		"MODEL_CHAT_", "MODEL_GPT_", "MODEL_CLAUDE_", "MODEL_GOOGLE_",
		"MODEL_PRIVATE_", "MODEL_LLAMA_", "MODEL_GEMINI_", "MODEL_COMMAND_",
		"MODEL_CASCADE_", "MODEL_SONNET_", "MODEL_OPUS_",
	}
	for _, k := range keys {
		if strings.Contains(raw, k) {
			return true
		}
	}
	return false
}
