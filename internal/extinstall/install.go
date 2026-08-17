package extinstall

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/logx"
)

const (
	Publisher = "devin-byok"
	Name      = "prompt-editor"
	Version   = "1.0.0"
	// ExtID VS Code/Devin 扩展 id
	ExtID = Publisher + "." + Name
)

// FolderName 磁盘目录名 publisher.name-version
func FolderName() string {
	return fmt.Sprintf("%s.%s-%s", Publisher, Name, Version)
}

// UserExtensionsDir Devin 用户扩展目录。
func UserExtensionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".devin", "extensions")
}

// InstallFromFS 将嵌入的扩展文件写入用户扩展目录并注册 extensions.json。
func InstallFromFS(src fs.FS, root string) (string, error) {
	base := UserExtensionsDir()
	if base == "" {
		return "", fmt.Errorf("无法解析用户主目录")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(base, FolderName())
	// 清理旧版同 id 目录
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), Publisher+"."+Name+"-") && e.Name() != FolderName() {
			_ = os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
	if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	err := fs.WalkDir(src, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// embed 使用正斜杠
		rel = filepath.FromSlash(rel)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		// 去掉 UTF-8 BOM，避免 package.json 被 Devin 判为非法 JSON
		if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
			b = b[3:]
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		return "", err
	}
	// 确保未标记 obsolete
	_ = clearObsolete(base, ExtID)
	if err := registerExtensionsJSON(base, dst, ExtID, Version, FolderName()); err != nil {
		logx.Warnf("ext register json: %v", err)
	}
	logx.Infof("extension installed: %s", dst)
	return dst, nil
}

// ---- Codeium.codeium-dev 扩展壳 ----
// Devin 扩展的 hasDevExtension()（extensions.getExtension("Codeium.codeium-dev")
// 是否存在）决定 getConfig() 是否读取 codeiumDev.* 开发配置。macOS 因签名
// bundle 不可替换，必须走 codeiumDev.languageServerBinaryPath 覆盖链路，
// 因此需要这个最小扩展壳（仅 package.json 声明 publisher/name，无需代码）。

const (
	DevShellPublisher = "Codeium"
	DevShellName     = "codeium-dev"
	DevShellVersion  = "0.0.1"
	// DevShellExtID 扩展 id（Codeium.codeium-dev）。
	DevShellExtID = DevShellPublisher + "." + DevShellName
)

// DevShellFolderName 磁盘目录名 publisher.name-version。
func DevShellFolderName() string {
	return fmt.Sprintf("%s.%s-%s", DevShellPublisher, DevShellName, DevShellVersion)
}

func devShellPackageJSON() []byte {
	return []byte(`{
  "name": "codeium-dev",
  "displayName": "Codeium Dev",
  "publisher": "Codeium",
  "version": "0.0.1",
  "description": "Development shell enabling codeiumDev.* settings for the Devin BYOK local service.",
  "engines": { "vscode": "^1.60.0" }
}
`)
}

// InstallDevShell 安装 Codeium.codeium-dev 扩展壳并注册 extensions.json。
func InstallDevShell() (string, error) {
	base := UserExtensionsDir()
	if base == "" {
		return "", fmt.Errorf("无法解析用户主目录")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(base, DevShellFolderName())
	if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dst, "package.json"), devShellPackageJSON(), 0o644); err != nil {
		return "", err
	}
	_ = clearObsolete(base, DevShellExtID)
	if err := registerExtensionsJSON(base, dst, DevShellExtID, DevShellVersion, DevShellFolderName()); err != nil {
		logx.Warnf("dev shell register json: %v", err)
	}
	logx.Infof("dev shell installed: %s", dst)
	return dst, nil
}

// EnableDevShell 启用扩展壳（清 .obsolete 标记）。
func EnableDevShell() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	return clearObsolete(base, DevShellExtID)
}

// DisableDevShell 禁用扩展壳（写 .obsolete，不删文件便于再启）。
func DisableDevShell() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	return setObsolete(base, DevShellExtID, true)
}

// UninstallDevShell 完全移除扩展壳。
func UninstallDevShell() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	_ = setObsolete(base, DevShellExtID, true)
	_ = os.RemoveAll(filepath.Join(base, DevShellFolderName()))
	_ = unregisterExtensionsJSON(base, DevShellExtID)
	return nil
}

// IsDevShellInstalled 扩展壳目录是否存在。
func IsDevShellInstalled() bool {
	base := UserExtensionsDir()
	if base == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(base, DevShellFolderName()))
	return err == nil && st.IsDir()
}

// Disable 停止服务时禁用扩展（写入 .obsolete，不删文件便于再启）。
func Disable() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	return setObsolete(base, ExtID, true)
}

// Enable 启动服务时启用（清 obsolete；若未安装由调用方 Install）。
func Enable() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	return clearObsolete(base, ExtID)
}

// Uninstall 完全移除。
func Uninstall() error {
	base := UserExtensionsDir()
	if base == "" {
		return nil
	}
	_ = setObsolete(base, ExtID, true)
	dst := filepath.Join(base, FolderName())
	_ = os.RemoveAll(dst)
	_ = unregisterExtensionsJSON(base, ExtID)
	return nil
}

// IsInstalled 目录是否存在。
func IsInstalled() bool {
	base := UserExtensionsDir()
	if base == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(base, FolderName()))
	return err == nil && st.IsDir()
}

// IsDisabled 是否在 .obsolete 中标记禁用。
func IsDisabled() bool {
	base := UserExtensionsDir()
	if base == "" {
		return false
	}
	b, err := os.ReadFile(obsoletePath(base))
	if err != nil || len(b) == 0 {
		return false
	}
	m := map[string]bool{}
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	return m[ExtID]
}

func obsoletePath(base string) string {
	return filepath.Join(base, ".obsolete")
}

func setObsolete(base, id string, on bool) error {
	p := obsoletePath(base)
	m := map[string]bool{}
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &m)
	}
	if on {
		m[id] = true
	} else {
		delete(m, id)
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(p, raw, 0o644)
}

func clearObsolete(base, id string) error {
	return setObsolete(base, id, false)
}

type extJSONEntry struct {
	Identifier struct {
		ID string `json:"id"`
	} `json:"identifier"`
	Version  string `json:"version"`
	Location struct {
		Mid    int    `json:"$mid"`
		Path   string `json:"path"`
		Scheme string `json:"scheme"`
	} `json:"location"`
	RelativeLocation string         `json:"relativeLocation"`
	Metadata         map[string]any `json:"metadata"`
}

func registerExtensionsJSON(base, absDir, id, version, folder string) error {
	p := filepath.Join(base, "extensions.json")
	var list []extJSONEntry
	if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &list)
	}
	// 规范化 path：/c:/Users/...
	loc := strings.ReplaceAll(absDir, `\`, `/`)
	if len(loc) >= 2 && loc[1] == ':' {
		loc = "/" + strings.ToLower(loc[:1]) + loc[1:]
	}
	entry := extJSONEntry{
		Version:          version,
		RelativeLocation: folder,
		Metadata: map[string]any{
			"installedTimestamp": time.Now().UnixMilli(),
			"pinned":             true,
			"source":             "vsix",
		},
	}
	entry.Identifier.ID = id
	entry.Location.Mid = 1
	entry.Location.Path = loc
	entry.Location.Scheme = "file"

	out := make([]extJSONEntry, 0, len(list)+1)
	for _, e := range list {
		if strings.EqualFold(e.Identifier.ID, id) {
			continue
		}
		out = append(out, e)
	}
	out = append(out, entry)
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o644)
}

func unregisterExtensionsJSON(base, id string) error {
	p := filepath.Join(base, "extensions.json")
	var list []extJSONEntry
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	_ = json.Unmarshal(b, &list)
	out := list[:0]
	for _, e := range list {
		if !strings.EqualFold(e.Identifier.ID, id) {
			out = append(out, e)
		}
	}
	raw, _ := json.Marshal(out)
	return os.WriteFile(p, raw, 0o644)
}
