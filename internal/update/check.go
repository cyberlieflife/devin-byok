package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"devin-byok/internal/platform"
)

// Config 在线更新配置。
type Config struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Repo GitHub owner/name，例如 cyberlieflife/devin-byok
	Repo string `yaml:"repo" json:"repo"`
	// AssetContains 用于匹配 release 资产名，默认使用当前平台资产
	AssetContains string `yaml:"asset_contains" json:"asset_contains"`
	// AutoApply 检查到新版本后是否自动下载并调度替换（默认 false，需用户确认更安全）
	AutoApply bool `yaml:"auto_apply" json:"auto_apply"`
	// CheckURL 可选：完整 releases/latest API；空则用 Repo 拼
	CheckURL string `yaml:"check_url" json:"check_url"`
}

// Result 检查结果。
type Result struct {
	OK              bool   `json:"ok"`
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	AssetURL        string `json:"asset_url,omitempty"`
	SHA256URL       string `json:"sha256_url,omitempty"`
	Body            string `json:"body,omitempty"`
	ChineseNotes    string `json:"chinese_notes,omitempty"`
	Message         string `json:"message,omitempty"`
	CheckedAt       string `json:"checked_at"`
}

// ApplyResult 应用更新结果。
type ApplyResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Script  string `json:"script,omitempty"`
	ZipPath string `json:"zip_path,omitempty"`
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Check 查询 GitHub Releases latest。
func Check(ctx context.Context, cfg Config, current string) Result {
	res := Result{OK: true, Current: normalizeVer(current), CheckedAt: time.Now().Format(time.RFC3339)}
	if !cfg.Enabled {
		res.Message = "已关闭在线更新"
		return res
	}
	url := strings.TrimSpace(cfg.CheckURL)
	if url == "" {
		repo := strings.TrimSpace(cfg.Repo)
		if repo == "" {
			res.OK = false
			res.Message = "update.repo 未配置（例如 owner/devin-byok）"
			return res
		}
		url = "https://api.github.com/repos/" + repo + "/releases/latest"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.OK = false
		res.Message = err.Error()
		return res
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "devin-byok-updater")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		res.OK = false
		res.Message = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		res.OK = false
		res.Message = fmt.Sprintf("github HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		return res
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		res.OK = false
		res.Message = "parse release: " + err.Error()
		return res
	}
	res.Latest = normalizeVer(rel.TagName)
	res.ReleaseURL = rel.HTMLURL
	res.Body = truncate(rel.Body, 2000)
	res.ChineseNotes = buildChineseNotes(res.Current, res.Latest, rel.Body)
	want := cfg.AssetContains
	if want == "" {
		want = platform.AssetSuffix()
	}
	var assetURL, shaURL, assetName string
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, strings.ToLower(want)) && isInstallAsset(n) {
			assetURL = a.BrowserDownloadURL
			assetName = a.Name
		}
		stem := strings.TrimSuffix(strings.ToLower(want), filepath.Ext(strings.ToLower(want)))
		if strings.HasSuffix(n, ".sha256") && strings.Contains(n, stem) {
			shaURL = a.BrowserDownloadURL
		}
	}
	// 若没匹配到 sha，尝试 同名.zip.sha256
	if assetURL != "" && shaURL == "" {
		for _, a := range rel.Assets {
			if strings.EqualFold(a.Name, assetName+".sha256") {
				shaURL = a.BrowserDownloadURL
			}
		}
	}
	res.AssetName = assetName
	res.AssetURL = assetURL
	res.SHA256URL = shaURL
	if res.Latest == "" {
		res.OK = false
		res.Message = "Release 缺少版本标签"
		return res
	}
	cmp := compareSemver(res.Current, res.Latest)
	res.UpdateAvailable = cmp < 0
	if assetURL == "" {
		res.Message = "找到版本但未匹配安装包资产：" + want
	} else if res.UpdateAvailable {
		res.Message = "发现新版本 v" + res.Latest
	} else {
		res.Message = "已是最新版本"
	}
	return res
}

func isInstallAsset(name string) bool {
	name = strings.ToLower(name)
	if strings.HasSuffix(name, ".sha256") {
		return false
	}
	return strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".dmg") || strings.HasSuffix(name, ".exe")
}

// DownloadAndSchedule 下载当前平台安装包，校验 sha256（若有），写更新脚本并启动后退出由调用方处理。
func DownloadAndSchedule(ctx context.Context, cfg Config, current string, installDir string) (ApplyResult, error) {
	setProgress("checking", 0, 0, 0, "正在检查更新…")
	check := Check(ctx, cfg, current)
	if !check.OK {
		setProgress("error", 0, 0, 0, check.Message)
		return ApplyResult{OK: false, Message: check.Message}, fmt.Errorf("%s", check.Message)
	}
	if !check.UpdateAvailable {
		// 无需更新：回到 idle，避免 GUI 误显示进度条
		ResetProgress()
		return ApplyResult{OK: true, Message: "无需更新: " + check.Message}, nil
	}
	if check.AssetURL == "" {
		return ApplyResult{OK: false, Message: check.Message}, fmt.Errorf("no asset")
	}
	if installDir == "" {
		exe, _ := os.Executable()
		installDir = platform.ReleaseInstallDir(exe)
	}
	if abs, err := filepath.Abs(installDir); err == nil {
		installDir = abs
	}
	tmp := filepath.Join(os.TempDir(), "devin-byok-update-"+strconv.FormatInt(time.Now().Unix(), 10))
	_ = os.MkdirAll(tmp, 0o755)
	artifactPath := filepath.Join(tmp, check.AssetName)
	setProgress("downloading", 0, 0, 0, "正在下载 "+check.Latest+"…")
	if err := downloadFileProgress(ctx, check.AssetURL, artifactPath); err != nil {
		setProgress("error", 0, 0, 0, "下载失败: "+err.Error())
		return ApplyResult{OK: false, Message: err.Error()}, err
	}
	if check.SHA256URL != "" {
		sumPath := artifactPath + ".sha256"
		setProgress("verifying", 95, 0, 0, "正在校验 SHA256…")
		if err := downloadFile(ctx, check.SHA256URL, sumPath); err == nil {
			want, _ := os.ReadFile(sumPath)
			got, err := fileSHA256(artifactPath)
			if err != nil {
				return ApplyResult{OK: false, Message: err.Error()}, err
			}
			w := strings.ToLower(strings.TrimSpace(string(want)))
			// 允许 "HASH" 或 "HASH  filename"
			if i := strings.IndexAny(w, " \t"); i > 0 {
				w = w[:i]
			}
			if w != "" && w != strings.ToLower(got) {
				return ApplyResult{OK: false, Message: "sha256 mismatch"}, fmt.Errorf("sha256 mismatch want=%s got=%s", w, got)
			}
		}
	}
	guiName := platform.GUIName()
	var script string
	var err error
	if strings.HasSuffix(strings.ToLower(check.AssetName), ".zip") {
		extractDir := filepath.Join(tmp, "extract")
		_ = os.MkdirAll(extractDir, 0o755)
		if err := unzip(artifactPath, extractDir); err != nil {
			return ApplyResult{OK: false, Message: "unzip: " + err.Error()}, err
		}
		script, err = scheduleApply(extractDir, installDir, guiName, tmp)
	} else {
		script, err = scheduleApplyArtifact(artifactPath, installDir, guiName, tmp)
	}
	if err != nil {
		setProgress("error", 0, 0, 0, err.Error())
		return ApplyResult{OK: false, Message: err.Error()}, err
	}
	setProgress("scheduling", 100, 0, 0, "即将退出并安装 "+check.Latest)
	return ApplyResult{OK: true, Message: "已下载 " + check.Latest + "，即将替换并重启", Script: script, ZipPath: artifactPath}, nil
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "devin-byok-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func unzip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		cleanDest := filepath.Clean(dest)
		target := filepath.Join(cleanDest, filepath.FromSlash(f.Name))
		rel, relErr := filepath.Rel(cleanDest, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("illegal file path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizeVer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	return s
}

// compareSemver a<b => -1; a==b => 0; a>b => 1
func compareSemver(a, b string) int {
	pa := parseVer(a)
	pb := parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseVer(s string) [3]int {
	s = normalizeVer(s)
	var out [3]int
	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n := parts[i]
		// strip pre-release
		if j := strings.IndexAny(n, "-+"); j >= 0 {
			n = n[:j]
		}
		v, _ := strconv.Atoi(n)
		out[i] = v
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// DefaultInstallDir 当前可执行文件目录。
func DefaultInstallDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := platform.ReleaseInstallDir(exe)
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// GOOS helper for tests
func IsWindows() bool { return runtime.GOOS == "windows" }

func buildChineseNotes(current, latest, body string) string {
	var b strings.Builder
	b.WriteString("发现新版本，建议立即更新以获得修复与新功能。\n\n")
	b.WriteString("当前版本：v" + normalizeVer(current) + "\n")
	b.WriteString("最新版本：v" + normalizeVer(latest) + "\n\n")
	b.WriteString("更新说明：\n")
	body = strings.TrimSpace(body)
	if body == "" {
		b.WriteString("（发布者未填写详细说明，请查看 GitHub Release 页面）\n")
	} else {
		// 直接展示正文；发布时请用中文撰写 notes
		b.WriteString(body)
		b.WriteString("\n")
	}
	b.WriteString("\n更新过程将：下载安装包 → 关闭当前程序 → 替换文件 → 自动重新打开 GUI。\n")
	b.WriteString("请先保存未保存的配置，然后点击「立即更新」。")
	return b.String()
}

type progressReader struct {
	r     io.Reader
	total int64
	read  int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
		pct := 0.0
		if p.total > 0 {
			pct = float64(p.read) * 90.0 / float64(p.total) // 0-90% for download
		} else {
			pct = 10
		}
		setProgress("downloading", pct, p.read, p.total, fmt.Sprintf("下载中 %d / %d 字节", p.read, p.total))
	}
	return n, err
}

func downloadFileProgress(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "devin-byok-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength}
	_, err = io.Copy(f, pr)
	if err == nil {
		setProgress("downloading", 90, pr.read, pr.total, "下载完成，准备校验…")
	}
	return err
}
