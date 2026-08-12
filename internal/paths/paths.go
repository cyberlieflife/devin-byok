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
// Prefer existing file; migrate once from legacy locations; else write embedded template.
func EnsureConfig() (string, error) {
	if _, err := EnsureDir(); err != nil {
		return "", err
	}
	dst := ConfigPath()
	if st, err := os.Stat(dst); err == nil && !st.IsDir() && st.Size() > 0 {
		return dst, nil
	}
	for _, old := range legacyConfigCandidates() {
		if old == "" || old == dst {
			continue
		}
		b, err := os.ReadFile(old)
		if err != nil || len(b) == 0 {
			continue
		}
		if err := os.WriteFile(dst, b, 0o644); err == nil {
			return dst, nil
		}
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
	out = append(out, filepath.Join(platform.DataDir(), "config.yaml"))
	return out
}
