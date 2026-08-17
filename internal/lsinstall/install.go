package lsinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/payload"
	"devin-byok/internal/platform"
)

// Meta 安装元数据。
type Meta struct {
	InstalledAt  string   `json:"installed_at"`
	Target       string   `json:"target"`
	Real         string   `json:"real"`
	WrapperSHA   string   `json:"wrapper_sha256"`
	RealSHA      string   `json:"real_sha256"`
	Backups      []string `json:"backups"`
	API          string   `json:"api"`
	WrapperPath  string   `json:"wrapper_path"`
}

func metaPath() string {
	return filepath.Join(platform.DataDir(), "ls-wrapper-install.json")
}

// MaterializeWrapper 将内置 wrapper 释放到平台数据目录。
func MaterializeWrapper() (string, error) {
	if len(payload.LSWrapper) == 0 {
		return "", fmt.Errorf("embedded ls-wrapper is empty")
	}
	dir := filepath.Join(platform.DataDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, platform.WrapperExeName())
	// 内容变化才覆盖，避免无谓写盘
	if b, err := os.ReadFile(dst); err == nil {
		if sha256Hex(b) == sha256Hex(payload.LSWrapper) {
			return dst, nil
		}
	}
	if err := os.WriteFile(dst, payload.LSWrapper, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

// BinDir 返回 Devin language_server 目录。
func BinDir(installDir string) string {
	return platform.ExtensionsBinDir(installDir)
}

// IsInstalled 是否已植入 wrapper（存在 .real）。
func IsInstalled(installDir string) bool {
	real := filepath.Join(BinDir(installDir), platform.RealLanguageServerName())
	_, err := os.Stat(real)
	return err == nil
}

// CleanBundleArtifacts 清理历史安装留下的备份/临时文件，恢复 bundle 原貌。
// 3.7.16 起不再修改 bundle，遗留文件会导致 Devin 安装完整性校验误报 corrupt。
func CleanBundleArtifacts(installDir string) {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	if installDir == "" {
		return
	}
	bin := BinDir(installDir)
	lsName := platform.LanguageServerName()
	entries, err := os.ReadDir(bin)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if n == lsName || n == platform.RealLanguageServerName() {
			continue
		}
		if strings.HasPrefix(n, lsName+".bak_") ||
			strings.HasPrefix(n, lsName+".wrapperbak_") ||
			strings.HasPrefix(n, lsName+".wrapper_before_restore_") {
			_ = os.Remove(filepath.Join(bin, n))
		}
	}
}

// Install 将内置 wrapper 安装到 Devin language_server 路径。
func Install(installDir string) (*Meta, error) {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	bin := BinDir(installDir)
	lsName := platform.LanguageServerName()
	target := filepath.Join(bin, lsName)
	real := filepath.Join(bin, platform.RealLanguageServerName())
	if _, err := os.Stat(target); err != nil {
		return nil, fmt.Errorf("missing language_server: %s", target)
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
		bak := filepath.Join(bin, lsName+".bak_"+ts)
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
		bak := filepath.Join(bin, lsName+".wrapperbak_"+ts)
		if err := copyFile(target, bak); err == nil {
			backups = append(backups, bak)
		}
	}

	if err := copyFile(wrapperSrc, target); err != nil {
		return nil, fmt.Errorf("install wrapper: %w", err)
	}

	meta := &Meta{
		InstalledAt: time.Now().Format(time.RFC3339),
		Target:      target,
		Real:        real,
		WrapperSHA:  fileSHA(target),
		RealSHA:     fileSHA(real),
		Backups:     backups,
		API:         "http://127.0.0.1:8787/_route/api_server",
		WrapperPath: wrapperSrc,
	}
	mp := metaPath()
	_ = os.MkdirAll(filepath.Dir(mp), 0o755)
	b, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(mp, b, 0o644)
	return meta, nil
}

// InstallIfNeeded 未安装时自动安装；已安装则刷新 wrapper 二进制。
func InstallIfNeeded(installDir string) (*Meta, error) {
	// 刷新安装（含已安装时覆盖 wrapper）
	return Install(installDir)
}

// Uninstall 还原原始 language_server。
func Uninstall(installDir string) error {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	bin := BinDir(installDir)
	lsName := platform.LanguageServerName()
	target := filepath.Join(bin, lsName)
	real := filepath.Join(bin, platform.RealLanguageServerName())
	if _, err := os.Stat(real); err != nil {
		return fmt.Errorf("real binary missing; nothing to uninstall")
	}
	ts := time.Now().Format("20060102_150405")
	if _, err := os.Stat(target); err == nil {
		bak := filepath.Join(bin, lsName+".wrapper_before_restore_"+ts)
		_ = copyFile(target, bak)
		_ = os.Remove(target)
	}
	if err := os.Rename(real, target); err != nil {
		return err
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

// realCopyDir 是 settings 覆盖链路（codeiumDev.languageServerBinaryPath）下
// wrapper 同目录的 .real 存放目录。包级变量便于测试注入临时目录。
var realCopyDir = func() string {
	return filepath.Join(platform.DataDir(), "bin")
}

// RealCopyDirForTest 测试钩子：读取 realCopyDir 当前实现。
func RealCopyDirForTest() func() string { return realCopyDir }

// SetRealCopyDirForTest 测试钩子：替换 realCopyDir（测试注入临时目录，避免
// 写入真实数据目录）。仅测试使用。
func SetRealCopyDirForTest(f func() string) { realCopyDir = f }

// EnsureRealCopy 从 Devin bundle 拷贝真实语言服务器到数据目录 bin，
// 供 settings 覆盖链路下的 wrapper 启动。macOS 签名 bundle 受系统保护
// 不可替换，这是唯一可行的本地启动路径；Windows 由 Install() 在 bundle 内
// 同目录放置 .real，无需本函数。内容一致时跳过，避免无谓写盘。
func EnsureRealCopy(installDir string) (string, error) {
	if installDir == "" {
		installDir = platform.DefaultInstallDir()
	}
	if installDir == "" {
		return "", fmt.Errorf("cannot resolve Devin install dir")
	}
	bin := BinDir(installDir)
	realName := platform.RealLanguageServerName()
	// 源优先 bundle 内已有的 .real（历史替换残留），否则用原版语言服务器。
	src := filepath.Join(bin, realName)
	if _, err := os.Stat(src); err != nil {
		src = filepath.Join(bin, platform.LanguageServerName())
	}
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("missing language server in bundle: %s", src)
	}
	dstDir := realCopyDir()
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dstDir, realName)
	if b, err := os.ReadFile(dst); err == nil {
		if s, err2 := os.ReadFile(src); err2 == nil && sha256Hex(b) == sha256Hex(s) {
			return dst, nil
		}
	}
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// RemoveRealCopy 删除数据目录 bin 下的 .real 副本（uninstall 时还原现场）。
func RemoveRealCopy() error {
	dst := filepath.Join(realCopyDir(), platform.RealLanguageServerName())
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
