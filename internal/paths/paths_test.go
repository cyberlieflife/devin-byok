package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"devin-byok/internal/paths"
	"devin-byok/internal/payload"
)

// 隔离数据目录：把 APPDATA/XDG_DATA_HOME/USERPROFILE/HOME 指到临时目录，
// 保证三平台 DataDir 与 legacy 位置都落在 tmp 内，不触碰真实环境。
func isolateDataDirs(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(tmp, "appdata"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "appdata"))
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	return tmp
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 目标位置已是示例模板（精确等于内嵌模板，或含占位域名 api.example.com 的旧示例）
// 时，应迁移历史位置的用户配置（升级路径修复）。
func TestEnsureConfigMigratesLegacyWhenTargetIsTemplate(t *testing.T) {
	tmp := isolateDataDirs(t)
	legacy := filepath.Join(tmp, ".devin-byok", "config.yaml")
	writeFile(t, legacy, "upstream:\n  base_url: http://user.example/v1\n")
	// 目标位置写入"旧版本示例模板"（与当前内嵌模板不同，但含占位域名 api.example.com）
	dst := paths.ConfigPath()
	oldExample := "server:\n  host: 127.0.0.1\n  port: 8787\nupstream:\n  base_url: https://api.example.com/v1\n  api_key: \"\"\n"
	writeFile(t, dst, oldExample)

	p, err := paths.EnsureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if p != dst {
		t.Fatalf("path = %q, want %q", p, dst)
	}
	if got := readFile(t, dst); got != "upstream:\n  base_url: http://user.example/v1\n" {
		t.Fatalf("example target was not replaced with legacy config: %q", got)
	}
}

// 目标为当前内嵌模板（精确相等）时同样迁移 legacy 配置。
func TestEnsureConfigMigratesLegacyWhenTargetIsExactTemplate(t *testing.T) {
	tmp := isolateDataDirs(t)
	legacy := filepath.Join(tmp, ".devin-byok", "config.yaml")
	writeFile(t, legacy, "legacy: true\n")
	dst := paths.ConfigPath()
	writeFile(t, dst, string(payload.ConfigExample))

	p, err := paths.EnsureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "legacy: true\n" {
		t.Fatalf("exact template target was not migrated: %q", got)
	}
}

// 目标位置不存在时，legacy 用户配置应被迁移（原有行为回归保护）。
func TestEnsureConfigMigratesLegacyWhenTargetMissing(t *testing.T) {
	tmp := isolateDataDirs(t)
	legacy := filepath.Join(tmp, ".devin-byok", "config.yaml")
	writeFile(t, legacy, "legacy: true\n")

	p, err := paths.EnsureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "legacy: true\n" {
		t.Fatalf("missing target was not migrated: %q", got)
	}
}

// 目标位置已是用户配置（非模板）时不得覆盖。
func TestEnsureConfigKeepsExistingUserConfig(t *testing.T) {
	tmp := isolateDataDirs(t)
	legacy := filepath.Join(tmp, ".devin-byok", "config.yaml")
	writeFile(t, legacy, "legacy: true\n")
	dst := paths.ConfigPath()
	writeFile(t, dst, "user: kept\n")

	p, err := paths.EnsureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != "user: kept\n" {
		t.Fatalf("existing user config was overwritten: %q", got)
	}
}

// 目标为模板且无 legacy 配置时保留模板。
func TestEnsureConfigKeepsTemplateWithoutLegacy(t *testing.T) {
	isolateDataDirs(t)
	dst := paths.ConfigPath()
	writeFile(t, dst, string(payload.ConfigExample))

	p, err := paths.EnsureConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, p); got != string(payload.ConfigExample) {
		t.Fatalf("template was not kept: %q", got)
	}
}
