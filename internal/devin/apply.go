package devin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/platform"
)

// Paths 描述 Devin 用户数据路径。
type Paths struct {
	UserDataDir  string
	SettingsJSON string
	StateDB      string
	StorageJSON  string
}

func ResolvePaths() (*Paths, error) {
	dataDirs := platform.DevinDataDirs()
	var userData string
	for _, d := range dataDirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			userData = d
			break
		}
	}
	if userData == "" {
		return nil, fmt.Errorf("未找到 Devin 用户目录（请先启动过一次 Devin）")
	}
	user := filepath.Join(userData, "User")
	_ = os.MkdirAll(user, 0o755)
	return &Paths{
		UserDataDir:  userData,
		SettingsJSON: filepath.Join(user, "settings.json"),
		StateDB:      filepath.Join(user, "globalStorage", "state.vscdb"),
		StorageJSON:  filepath.Join(user, "globalStorage", "storage.json"),
	}, nil
}

// ApplyResult 是 apply 的结果摘要。
type ApplyResult struct {
	PortalURL    string   `json:"portal_url"`
	APIServerURL string   `json:"api_server_url"`
	SettingsPath string   `json:"settings_path"`
	Backups      []string `json:"backups"`
	SettingsKeys []string `json:"settings_keys"`
	NeedReload   bool     `json:"need_reload"`
	Notes        []string `json:"notes"`
}

// RestoreResult 是 restore 结果。
type RestoreResult struct {
	Restored []string `json:"restored"`
	Notes    []string `json:"notes"`
}

func backupFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("不能备份目录: %s", path)
	}
	ts := time.Now().Format("20060102_150405")
	bak := path + ".bak_" + ts
	in, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(bak, in, 0o644); err != nil {
		return "", err
	}
	out, err := os.ReadFile(bak)
	if err != nil {
		return "", err
	}
	if len(out) != len(in) {
		return "", fmt.Errorf("备份校验失败: %s", bak)
	}
	return bak, nil
}

func removeBackup(bak string) {
	if bak == "" {
		return
	}
	_ = os.Remove(bak)
}

func loadSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var m map[string]any
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("解析 settings.json 失败: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func saveSettings(path string, m map[string]any) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// ApplyPortal 写入 portalUrl + codeium.apiServerUrl，强制 LS/ACP 指向本地兼容层。
func ApplyPortal(portalBase string, settingKeys []string) (*ApplyResult, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	portalBase = strings.TrimRight(strings.TrimSpace(portalBase), "/")
	if portalBase == "" {
		return nil, fmt.Errorf("portalBase 为空")
	}
	apiURL := portalBase + "/_route/api_server"

	// S1 portal keys + S2 直接 API URL keys（扩展内部映射到 Config.API_SERVER_URL）
	keys := []string{
		"devin.portalUrl",
		"windsurf.portalUrl",
		"codeium.apiServerUrl",
		"codeium.inferenceApiServerUrl",
	}
	if len(settingKeys) > 0 {
		// 保留用户额外 key，但不丢掉强制 key
		seen := map[string]bool{}
		merged := []string{}
		for _, k := range append(keys, settingKeys...) {
			if !seen[k] {
				seen[k] = true
				merged = append(merged, k)
			}
		}
		keys = merged
	}

	res := &ApplyResult{
		PortalURL:    portalBase,
		APIServerURL: apiURL,
		SettingsPath: paths.SettingsJSON,
		SettingsKeys: keys,
		NeedReload:   true,
	}

	bak, err := backupFile(paths.SettingsJSON)
	if err != nil {
		return nil, fmt.Errorf("备份 settings.json 失败: %w", err)
	}
	if bak != "" {
		res.Backups = append(res.Backups, bak)
	}

	m, err := loadSettings(paths.SettingsJSON)
	if err != nil {
		return nil, err
	}
	old := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			old[k] = v
		} else {
			old[k] = nil
		}
		switch k {
		case "codeium.apiServerUrl", "codeium.inferenceApiServerUrl":
			m[k] = apiURL
		default:
			m[k] = portalBase
		}
	}
	if err := saveSettings(paths.SettingsJSON, m); err != nil {
		return nil, err
	}

	metaDir := platform.DataDir()
	_ = os.MkdirAll(metaDir, 0o755)
	meta := map[string]any{
		"applied_at":     time.Now().Format(time.RFC3339),
		"portal_url":     portalBase,
		"api_server_url": apiURL,
		"settings_path":  paths.SettingsJSON,
		"settings_keys":  keys,
		"old_settings":   old,
		"settings_bak":   bak,
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	metaPath := filepath.Join(metaDir, "last-apply.json")
	_ = os.WriteFile(metaPath, mb, 0o644)
	res.Notes = append(res.Notes,
		"已写入 portalUrl + codeium.apiServerUrl/inferenceApiServerUrl",
		"请完全退出并重启 Devin",
		"重启后检查 language_server 是否包含 --api_server_url "+apiURL,
		"若仍指向 server.codeium.com，将启用 LS 二进制包装器",
		"元数据: "+metaPath,
	)
	return res, nil
}

// RestorePortal 恢复 apply 前设置。
func RestorePortal() (*RestoreResult, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	metaPath := filepath.Join(platform.DataDir(), "last-apply.json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("未找到 last-apply.json，无法自动恢复: %w", err)
	}
	var meta struct {
		SettingsPath string         `json:"settings_path"`
		SettingsKeys []string       `json:"settings_keys"`
		OldSettings  map[string]any `json:"old_settings"`
		SettingsBak  string         `json:"settings_bak"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	if meta.SettingsPath == "" {
		meta.SettingsPath = paths.SettingsJSON
	}
	res := &RestoreResult{}
	curBak, err := backupFile(meta.SettingsPath)
	if err != nil {
		return nil, err
	}
	if curBak != "" {
		res.Restored = append(res.Restored, "current-backup:"+curBak)
	}
	m, err := loadSettings(meta.SettingsPath)
	if err != nil {
		return nil, err
	}
	for _, k := range meta.SettingsKeys {
		old, ok := meta.OldSettings[k]
		if !ok || old == nil {
			delete(m, k)
		} else {
			m[k] = old
		}
	}
	if err := saveSettings(meta.SettingsPath, m); err != nil {
		return nil, err
	}
	res.Restored = append(res.Restored, meta.SettingsPath)
	if meta.SettingsBak != "" {
		removeBackup(meta.SettingsBak)
		res.Notes = append(res.Notes, "已删除 apply 备份: "+meta.SettingsBak)
	}
	if curBak != "" {
		removeBackup(curBak)
	}
	res.Notes = append(res.Notes, "请重启 Devin 使 language_server 重新拉起")
	return res, nil
}
