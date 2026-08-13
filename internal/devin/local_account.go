package devin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/platform"
)

const localAccountFileName = "local-account.json"

// LocalAccount is a machine-local identity used only by the BYOK compatibility service.
type LocalAccount struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	APIKey    string `json:"api_key"`
	CreatedAt string `json:"created_at"`
}

func LocalAccountPath() string {
	return filepath.Join(platform.DataDir(), localAccountFileName)
}

func LoadLocalAccount() (*LocalAccount, error) {
	return loadLocalAccountAt(LocalAccountPath())
}

// EnsureLocalAccount creates a random local identity once and reuses it on later runs.
func EnsureLocalAccount() (*LocalAccount, bool, error) {
	return ensureLocalAccountAt(LocalAccountPath(), rand.Reader, time.Now())
}

func loadLocalAccountAt(path string) (*LocalAccount, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var account LocalAccount
	if err := json.Unmarshal(b, &account); err != nil {
		return nil, fmt.Errorf("解析本地账户失败: %w", err)
	}
	if err := validateLocalAccount(&account); err != nil {
		return nil, err
	}
	return &account, nil
}

func ensureLocalAccountAt(path string, random io.Reader, now time.Time) (*LocalAccount, bool, error) {
	account, err := loadLocalAccountAt(path)
	if err == nil {
		return account, false, nil
	}
	if !os.IsNotExist(err) {
		return nil, false, err
	}

	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil {
		return nil, false, fmt.Errorf("生成本地账户失败: %w", err)
	}
	idPart := hex.EncodeToString(raw[:8])
	account = &LocalAccount{
		ID:        "byok-local-" + idPart,
		Name:      "BYOK Local",
		Email:     "byok-" + idPart[:12] + "@local.invalid",
		APIKey:    "sk-ws-01-byok-" + hex.EncodeToString(raw[8:]),
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	b, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return nil, false, err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, false, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, false, err
	}
	return account, true, nil
}

func validateLocalAccount(account *LocalAccount) error {
	if account == nil || strings.TrimSpace(account.ID) == "" || strings.TrimSpace(account.Name) == "" ||
		strings.TrimSpace(account.Email) == "" || strings.TrimSpace(account.APIKey) == "" {
		return fmt.Errorf("本地账户文件缺少必要字段")
	}
	if !strings.HasSuffix(strings.ToLower(account.Email), ".invalid") {
		return fmt.Errorf("本地账户邮箱必须使用 .invalid 域名")
	}
	if !strings.HasPrefix(account.APIKey, "sk-ws-01-byok-") {
		return fmt.Errorf("本地账户 API key 格式无效")
	}
	return nil
}

// ApplyLocalAccountToConfig enables pure-local mode and stores the generated identity.
func ApplyLocalAccountToConfig(configPath string, account *LocalAccount) (*config.File, error) {
	if err := validateLocalAccount(account); err != nil {
		return nil, err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	cfg.Auth.FakeUserID = account.ID
	cfg.Auth.FakeName = account.Name
	cfg.Auth.FakeEmail = account.Email
	cfg.Auth.FakeAPIKey = account.APIKey
	cfg.Features.PureLocal = true
	if err := config.Save(configPath, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LocalAccountImported checks whether Devin currently points at this local service.
func LocalAccountImported(cfg *config.File, wrapperPath string) (bool, string) {
	paths, err := ResolvePaths()
	if err != nil {
		return false, err.Error()
	}
	settings, err := loadSettings(paths.SettingsJSON)
	if err != nil {
		return false, err.Error()
	}
	portal := strings.TrimRight(strings.TrimSpace(cfg.Server.PublicBase), "/")
	apiURL := portal + "/_route/api_server"
	expected := map[string]any{
		"devin.portalUrl":                      portal,
		"windsurf.portalUrl":                   portal,
		"codeium.apiServerUrl":                 apiURL,
		"codeium.inferenceApiServerUrl":        apiURL,
		"devin.multiTenantMode":                true,
		"devin.cascade.enabled":                true,
		"sync.enableSettings":                   false,
		"codeiumDev.languageServerBinaryPath": wrapperPath,
	}
	for key, value := range expected {
		if value == "" && key == "codeiumDev.languageServerBinaryPath" {
			continue
		}
		if settings[key] != value {
			return false, "Devin 本地配置尚未导入或已被覆盖"
		}
	}
	return true, "已导入，完全退出并重启 Devin 后生效"
}
