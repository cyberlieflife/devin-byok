package lsinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"devin-byok/internal/payload"
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
	return filepath.Join(os.Getenv("APPDATA"), "devin-byok", "ls-wrapper-install.json")
}

// MaterializeWrapper 将内置 wrapper 释放到 %APPDATA%\devin-byok\bin\。
func MaterializeWrapper() (string, error) {
	if len(payload.LSWrapper) == 0 {
		return "", fmt.Errorf("embedded ls-wrapper is empty")
	}
	dir := filepath.Join(os.Getenv("APPDATA"), "devin-byok", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "devin-byok-ls-wrapper.exe")
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
	return filepath.Join(installDir, "resources", "app", "extensions", "windsurf", "bin")
}

// IsInstalled 是否已植入 wrapper（存在 .real）。
func IsInstalled(installDir string) bool {
	real := filepath.Join(BinDir(installDir), "language_server_windows_x64.real.exe")
	_, err := os.Stat(real)
	return err == nil
}

// Install 将内置 wrapper 安装到 Devin language_server 路径。
func Install(installDir string) (*Meta, error) {
	if installDir == "" {
		installDir = `D:\Devin`
	}
	bin := BinDir(installDir)
	target := filepath.Join(bin, "language_server_windows_x64.exe")
	real := filepath.Join(bin, "language_server_windows_x64.real.exe")
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
		bak := filepath.Join(bin, "language_server_windows_x64.exe.bak_"+ts)
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
		bak := filepath.Join(bin, "language_server_windows_x64.exe.wrapperbak_"+ts)
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
		installDir = `D:\Devin`
	}
	bin := BinDir(installDir)
	target := filepath.Join(bin, "language_server_windows_x64.exe")
	real := filepath.Join(bin, "language_server_windows_x64.real.exe")
	if _, err := os.Stat(real); err != nil {
		return fmt.Errorf("real binary missing; nothing to uninstall")
	}
	ts := time.Now().Format("20060102_150405")
	if _, err := os.Stat(target); err == nil {
		bak := filepath.Join(bin, "language_server_windows_x64.exe.wrapper_before_restore_"+ts)
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
