package paths

import (
	"os"
	"path/filepath"

	"devin-byok/internal/payload"
	"devin-byok/internal/platform"
)

const DirName = ".devin-byok"

func Dir() string {
	return platform.DataDir()
}

// EnsureDir creates the data directory.
func EnsureDir() (string, error) {
	d := Dir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// ConfigPath returns config.yaml path.
func ConfigPath() string {
	return filepath.Join(Dir(), "config.yaml")
}

// WorkDir returns work directory under data dir.
func WorkDir() string {
	return filepath.Join(Dir(), "work")
}

// CaptureDir returns RPC capture directory and ensures it exists.
func CaptureDir() string {
	d := filepath.Join(WorkDir(), "capture")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// EnsureConfig ensures config.yaml exists.
// Prefer existing non-template file; migrate once from legacy locations when the
// target is missing or still an untouched embedded template; else write template.
func EnsureConfig() (string, error) {
	if _, err := EnsureDir(); err != nil {
		return "", err
	}
	dst := ConfigPath()
	dstIsTemplate := false
	if st, err := os.Stat(dst); err == nil && !st.IsDir() && st.Size() > 0 {
		if b, rerr := os.ReadFile(dst); rerr == nil && len(payload.ConfigExample) > 0 && string(b) == string(payload.ConfigExample) {
			// 目标位置是未修改的内置模板：可能是升级时生成的示例，
			// 下面仍尝试从历史位置迁移用户真实配置。
			dstIsTemplate = true
		} else {
			// 已存在且非模板：用户配置，直接使用。
			return dst, nil
		}
	}
	for _, old := range legacyConfigCandidates() {
		if old == "" || old == dst {
			continue
		}
		b, err := os.ReadFile(old)
		if err != nil || len(b) == 0 {
			continue
		}
		// 跳过同样是内置模板的候选（避免把示例配置当用户配置迁移）。
		if len(payload.ConfigExample) > 0 && string(b) == string(payload.ConfigExample) {
			continue
		}
		if err := os.WriteFile(dst, b, 0o644); err == nil {
			return dst, nil
		}
	}
	if dstIsTemplate {
		return dst, nil
	}
	if len(payload.ConfigExample) == 0 {
		return "", os.ErrNotExist
	}
	if err := os.WriteFile(dst, payload.ConfigExample, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// FindConfig returns config path, creating/migrating when needed.
func FindConfig() string {
	dst := ConfigPath()
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		return dst
	}
	if p, err := EnsureConfig(); err == nil {
		return p
	}
	return dst
}

func legacyConfigCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "config.yaml"))
	}
	out = append(out, "config.yaml")
	// 历史数据目录：master(v1.2.x) 在 Windows 用 ~/.devin-byok，
	// v1.0.0 起改为 platform.DataDir()（%APPDATA%\devin-byok）。
	// 升级用户的数据目录不迁移会导致 config/capture/system-prompts 静默丢失，
	// 这里把旧默认位置加入候选，EnsureConfig 会优先迁移已存在的旧配置。
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".devin-byok", "config.yaml"))
	}
	if app := os.Getenv("APPDATA"); app != "" {
		out = append(out, filepath.Join(app, "devin-byok", "config.yaml"))
	}
	out = append(out, filepath.Join(platform.DataDir(), "config.yaml"))
	return out
}
