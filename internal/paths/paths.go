package paths

import (
	"os"
	"path/filepath"

	"devin-byok/internal/payload"
)

// DirName is the user data directory name managed by GUI.
const DirName = ".devin-byok"

// Dir returns data root: %USERPROFILE%\.devin-byok
// Falls back to %APPDATA%\devin-byok when home is unavailable.
func Dir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, DirName)
	}
	if app := os.Getenv("APPDATA"); app != "" {
		return filepath.Join(app, "devin-byok")
	}
	return DirName
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
	if app := os.Getenv("APPDATA"); app != "" {
		out = append(out, filepath.Join(app, "devin-byok", "config.yaml"))
	}
	out = append(out, `D:\Devin-byok\config.yaml`)
	return out
}
