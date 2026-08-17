package devinwrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"devin-byok/internal/logx"
	"devin-byok/internal/payload"
	"devin-byok/internal/platform"
)

// Devin ACP agent（devin.exe）接管：
//
// Devin 的 Agent/ACP 通道由扩展 spawn `devin.exe acp`（stdio ACP），认证时
// 扩展把官方 api_server_url 放在 authenticate meta 里传给 devin.exe，本地
// settings 注入无法改变它（codeium.apiServerUrl 不是 Devin 注册的配置键）。
// 实测 devin.exe 会优先使用环境变量 WINDSURF_API_SERVER_URL 覆盖该 URL，
// 因此：
//   - Windows：bundle 无签名保护，直接替换 devin/bin/devin.exe 为包装器
//     （注入 WINDSURF_API_SERVER_URL 后 exec 真身 devin.exe.real）。
//   - macOS：签名 bundle 不可写，改用 launchctl setenv 设置用户级环境变量
//     （Devin.app 由 launchd 启动时继承）。
//
// devin.exe 的模型列表/账号/聊天全部走与 language_server 相同的
// exa.* Connect RPC（GetUserStatus/GetCliModelConfigs/GetChatMessage），
// 本地兼容层已有对应 stub，因此接管 URL 后 ACP 通道即走本地模型。

// Meta 安装元数据（Windows 替换 bundle 时写入）。
type Meta struct {
	InstalledAt string   `json:"installed_at"`
	Target      string   `json:"target"`
	Real        string   `json:"real"`
	WrapperSHA  string   `json:"wrapper_sha256"`
	RealSHA     string   `json:"real_sha256"`
	Backups     []string `json:"backups"`
	API         string   `json:"api"`
}

const localAPIBase = "http://127.0.0.1:8787"

func metaPath() string {
	return filepath.Join(platform.DataDir(), "devin-wrapper-install.json")
}

// wrapperFileName 数据目录中 wrapper 副本的文件名（平台相关后缀）。
func wrapperFileName() string {
	if runtime.GOOS == "windows" {
		return "devin-byok-acp-wrapper.exe"
	}
	return "devin-byok-acp-wrapper"
}

// MaterializeWrapper 将内置 devin wrapper 释放到数据目录 bin。
func MaterializeWrapper() (string, error) {
	if len(payload.DevinWrapper) == 0 {
		return "", fmt.Errorf("embedded devin wrapper is empty")
	}
	dir := filepath.Join(platform.DataDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, wrapperFileName())
	if b, err := os.ReadFile(dst); err == nil {
		if sha256Hex(b) == sha256Hex(payload.DevinWrapper) {
			return dst, nil
		}
	}
	if err := os.WriteFile(dst, payload.DevinWrapper, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

// Install 将 devin wrapper 安装到 bundle（Windows：替换 devin/bin/devin.exe）。
func Install(installDir string) (*Meta, error) {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	bin := platform.DevinBinDir(installDir)
	exeName := platform.DevinExeName()
	target := filepath.Join(bin, exeName)
	real := filepath.Join(bin, platform.RealDevinExeName())
	if _, err := os.Stat(target); err != nil {
		return nil, fmt.Errorf("missing devin agent: %s", target)
	}
	wrapperSrc, err := MaterializeWrapper()
	if err != nil {
		return nil, err
	}

	ts := time.Now().Format("20060102_150405")
	var backups []string
	already := false
	if _, err := os.Stat(real); err == nil {
		already = true
	}

	if !already {
		bak := filepath.Join(bin, exeName+".bak_"+ts)
		if err := copyFile(target, bak); err != nil {
			return nil, fmt.Errorf("backup: %w", err)
		}
		backups = append(backups, bak)
		st1, _ := os.Stat(target)
		st2, _ := os.Stat(bak)
		if st1 != nil && st2 != nil && st1.Size() != st2.Size() {
			return nil, fmt.Errorf("backup size mismatch")
		}
		if err := os.Rename(target, real); err != nil {
			return nil, fmt.Errorf("rename to real: %w", err)
		}
	} else {
		bak := filepath.Join(bin, exeName+".wrapperbak_"+ts)
		if err := copyFile(target, bak); err == nil {
			backups = append(backups, bak)
		}
	}

	if err := copyFile(wrapperSrc, target); err != nil {
		return nil, fmt.Errorf("install devin wrapper: %w", err)
	}

	meta := &Meta{
		InstalledAt: time.Now().Format(time.RFC3339),
		Target:      target,
		Real:        real,
		WrapperSHA:  fileSHA(target),
		RealSHA:     fileSHA(real),
		Backups:     backups,
		API:         localAPIBase,
	}
	mp := metaPath()
	_ = os.MkdirAll(filepath.Dir(mp), 0o755)
	b, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(mp, b, 0o644)
	return meta, nil
}

// Uninstall 还原 bundle 内的 devin.exe（Windows）。
func Uninstall(installDir string) error {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	bin := platform.DevinBinDir(installDir)
	exeName := platform.DevinExeName()
	target := filepath.Join(bin, exeName)
	real := filepath.Join(bin, platform.RealDevinExeName())
	if _, err := os.Stat(real); err != nil {
		return fmt.Errorf("real devin binary missing; nothing to uninstall")
	}
	ts := time.Now().Format("20060102_150405")
	if _, err := os.Stat(target); err == nil {
		bak := filepath.Join(bin, exeName+".wrapper_before_restore_"+ts)
		_ = copyFile(target, bak)
		_ = os.Remove(target)
	}
	if err := os.Rename(real, target); err != nil {
		return err
	}
	return nil
}

// IsInstalled bundle 内是否存在真身（Windows 判定）。
func IsInstalled(installDir string) bool {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	_, err := os.Stat(filepath.Join(platform.DevinBinDir(installDir), platform.RealDevinExeName()))
	return err == nil
}

// Ensure 按平台装配 ACP 接管：
//   - Windows：替换 bundle devin.exe 为包装器
//   - macOS：launchctl setenv 用户级环境变量（签名 bundle 不可写）
func Ensure(installDir string) error {
	switch runtime.GOOS {
	case "windows":
		meta, err := Install(installDir)
		if err != nil {
			return err
		}
		logx.Infof("devin wrapper installed target=%s real=%s", meta.Target, meta.Real)
		return nil
	case "darwin":
		return SetUserEnv()
	default:
		logx.Infof("devin wrapper: skipped on %s (unsupported platform)", runtime.GOOS)
		return nil
	}
}

// Remove 撤销 ACP 接管（与 Ensure 对应）。
func Remove(installDir string) error {
	switch runtime.GOOS {
	case "windows":
		if err := Uninstall(installDir); err != nil {
			return err
		}
		logx.Infof("devin wrapper uninstalled")
		return nil
	case "darwin":
		return UnsetUserEnv()
	default:
		return nil
	}
}

// SetUserEnv 通过 launchctl 设置用户级环境变量（macOS，Devin.app 由
// launchd 启动时继承 WINDSURF_API_SERVER_URL）。
func SetUserEnv() error {
	cmd := exec.Command("launchctl", "setenv", "WINDSURF_API_SERVER_URL", localAPIBase)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl setenv: %v: %s", err, string(out))
	}
	return nil
}

// UnsetUserEnv 移除 launchctl 用户级环境变量。
func UnsetUserEnv() error {
	cmd := exec.Command("launchctl", "unsetenv", "WINDSURF_API_SERVER_URL")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl unsetenv: %v: %s", err, string(out))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fileSHA(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha256Hex(b)
}
