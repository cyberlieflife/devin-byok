package ideinject

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devin-byok/internal/logx"
	"devin-byok/internal/platform"
)

//go:embed byok-context-usage.js
var assetFS embed.FS

const (
	scriptName  = "byok-context-usage.js"
	markerBegin = "<!-- devin-byok-context-usage:begin -->"
	markerEnd   = "<!-- devin-byok-context-usage:end -->"
	metaFileName = "ideinject-context-usage.json"
)

func sessionsHTML(installDir string) string {
	return platform.SessionsHTMLPath(installDir)
}

func sessionsDir(installDir string) string {
	return filepath.Dir(platform.SessionsHTMLPath(installDir))
}

// dataDir 返回元数据目录；测试中可覆盖以隔离真实用户目录。
var dataDir = platform.DataDir

func metaPath() string {
	return filepath.Join(dataDir(), metaFileName)
}

// ApplyContextUsageDonut 向 Devin sessions.html 注入悬停四色环脚本。
func ApplyContextUsageDonut(installDir string) error {
	installDir = strings.TrimSpace(installDir)
	if installDir == "" {
		installDir = detectInstallDir()
	}
	if installDir == "" {
		return fmt.Errorf("未找到 Devin 安装目录（请在配置中填写 devin.install_dir）")
	}
	htmlPath := sessionsHTML(installDir)
	dir := sessionsDir(installDir)
	if st, err := os.Stat(htmlPath); err != nil || st.IsDir() {
		return fmt.Errorf("sessions.html 不存在: %s", htmlPath)
	}

	jsBytes, err := assetFS.ReadFile(scriptName)
	if err != nil {
		return err
	}
	jsPath := filepath.Join(dir, scriptName)
	if err := os.WriteFile(jsPath, jsBytes, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", jsPath, err)
	}

	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		return err
	}
	html := string(raw)

	if strings.Contains(html, markerBegin) {
		_ = writeMeta(installDir, htmlPath, jsPath, false)
		logx.Infof("ideinject context-usage: already present, js refreshed")
		return nil
	}

	bak := htmlPath + ".bak_byok_ctx_" + time.Now().Format("20060102_150405")
	if err := os.WriteFile(bak, raw, 0o644); err != nil {
		return fmt.Errorf("备份 sessions.html 失败: %w", err)
	}

	// 注意转义：Go 字符串中 src="./file" 需写成 src=\"./file\"
	snippet := "\n\t" + markerBegin + "\n\t<script src=\"./" + scriptName + "\" defer></script>\n\t" + markerEnd + "\n"

	out := html
	lower := strings.ToLower(html)
	if idx := strings.LastIndex(lower, "</html>"); idx >= 0 {
		out = html[:idx] + snippet + html[idx:]
	} else {
		out = html + snippet
	}
	if err := os.WriteFile(htmlPath, []byte(out), 0o644); err != nil {
		return err
	}
	_ = writeMeta(installDir, htmlPath, jsPath, true)
	logx.Infof("ideinject context-usage: injected into %s", htmlPath)
	return nil
}

// RestoreContextUsageDonut 移除注入。
func RestoreContextUsageDonut() error {
	meta, err := readMeta()
	if err != nil {
		dir := detectInstallDir()
		if dir == "" {
			return nil
		}
		return restoreAt(dir)
	}
	return restoreAt(meta.InstallDir)
}

func restoreAt(installDir string) error {
	if installDir == "" {
		return nil
	}
	htmlPath := sessionsHTML(installDir)
	dir := sessionsDir(installDir)
	jsPath := filepath.Join(dir, scriptName)
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil
	}
	html := string(raw)
	if strings.Contains(html, markerBegin) {
		start := strings.Index(html, markerBegin)
		end := strings.Index(html, markerEnd)
		if start >= 0 && end > start {
			end += len(markerEnd)
			for start > 0 && (html[start-1] == '\n' || html[start-1] == '\r' || html[start-1] == '\t') {
				start--
			}
			html = html[:start] + html[end:]
			_ = os.WriteFile(htmlPath, []byte(html), 0o644)
		}
	}
	_ = os.Remove(jsPath)
	// 清理历史备份，恢复 bundle 原貌（避免安装完整性校验误报）
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, "sessions.html.bak_byok_ctx_") {
				_ = os.Remove(filepath.Join(dir, n))
			}
		}
	}
	_ = os.Remove(metaPath())
	logx.Infof("ideinject context-usage: restored %s", htmlPath)
	return nil
}

type injectMeta struct {
	InstallDir string
	HTMLPath   string
	JSPath     string
}

func writeMeta(installDir, htmlPath, jsPath string, fresh bool) error {
	dir := dataDir()
	_ = os.MkdirAll(dir, 0o755)
	b := []byte(fmt.Sprintf(
		"{\n  \"install_dir\": %q,\n  \"html_path\": %q,\n  \"js_path\": %q,\n  \"applied_at\": %q,\n  \"fresh_html_inject\": %v\n}\n",
		installDir, htmlPath, jsPath, time.Now().Format(time.RFC3339), fresh,
	))
	return os.WriteFile(metaPath(), b, 0o644)
}

func readMeta() (*injectMeta, error) {
	b, err := os.ReadFile(metaPath())
	if err != nil {
		return nil, err
	}
	s := string(b)
	m := &injectMeta{
		InstallDir: jsonStringField(s, "install_dir"),
		HTMLPath:   jsonStringField(s, "html_path"),
		JSPath:     jsonStringField(s, "js_path"),
	}
	if m.InstallDir == "" {
		return nil, fmt.Errorf("meta empty")
	}
	return m, nil
}

func jsonStringField(s, key string) string {
	k := "\"" + key + "\""
	i := strings.Index(s, k)
	if i < 0 {
		return ""
	}
	rest := s[i+len(k):]
	j := strings.Index(rest, "\"")
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	k2 := strings.Index(rest, "\"")
	if k2 < 0 {
		return ""
	}
	return rest[:k2]
}

func detectInstallDir() string {
	cands := platform.DevinInstallCandidates()
	for _, c := range cands {
		if st, err := os.Stat(sessionsHTML(c)); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}