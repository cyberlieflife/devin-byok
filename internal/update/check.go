package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config 在线更新配置。
type Config struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	// Repo GitHub owner/name，例如 cyberlieflife/devin-byok
	Repo string `yaml:"repo" json:"repo"`
	// AssetContains 用于匹配 release 资产名，默认 windows-amd64.zip
	AssetContains string `yaml:"asset_contains" json:"asset_contains"`
	// AutoApply 检查到新版本后是否自动下载并调度替换（默认 false，需用户确认更安全）
	AutoApply bool `yaml:"auto_apply" json:"auto_apply"`
	// CheckURL 可选：完整 releases/latest API；空则用 Repo 拼
	CheckURL string `yaml:"check_url" json:"check_url"`
}

// Result 检查结果。
type Result struct {
	OK            bool   `json:"ok"`
	Current       string `json:"current"`
	Latest        string `json:"latest,omitempty"`
	UpdateAvailable bool `json:"update_available"`
	ReleaseURL    string `json:"release_url,omitempty"`
	AssetName     string `json:"asset_name,omitempty"`
	AssetURL      string `json:"asset_url,omitempty"`
	SHA256URL     string `json:"sha256_url,omitempty"`
	Body          string `json:"body,omitempty"`
	Message       string `json:"message,omitempty"`
	CheckedAt     string `json:"checked_at"`
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
		res.Message = "update disabled"
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
	want := cfg.AssetContains
	if want == "" {
		want = "windows-amd64.zip"
	}
	var zipURL, shaURL, zipName string
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, strings.ToLower(want)) && strings.HasSuffix(n, ".zip") && !strings.HasSuffix(n, ".sha256") {
			zipURL = a.BrowserDownloadURL
			zipName = a.Name
		}
		if strings.HasSuffix(n, ".sha256") && strings.Contains(n, strings.ToLower(strings.TrimSuffix(want, ".zip"))) {
			shaURL = a.BrowserDownloadURL
		}
	}
	// 若没匹配到 sha，尝试 同名.zip.sha256
	if zipURL != "" && shaURL == "" {
		for _, a := range rel.Assets {
			if strings.EqualFold(a.Name, zipName+".sha256") {
				shaURL = a.BrowserDownloadURL
			}
		}
	}
	res.AssetName = zipName
	res.AssetURL = zipURL
	res.SHA256URL = shaURL
	if res.Latest == "" {
		res.OK = false
		res.Message = "release 无 tag"
		return res
	}
	cmp := compareSemver(res.Current, res.Latest)
	res.UpdateAvailable = cmp < 0
	if zipURL == "" {
		res.Message = "找到版本但无匹配资产 " + want
	} else if res.UpdateAvailable {
		res.Message = "有新版本 " + res.Latest
	} else {
		res.Message = "已是最新或更新"
	}
	return res
}

// DownloadAndSchedule 下载 zip，校验 sha256（若有），写更新脚本并启动后退出由调用方处理。
func DownloadAndSchedule(ctx context.Context, cfg Config, current string, installDir string) (ApplyResult, error) {
	check := Check(ctx, cfg, current)
	if !check.OK {
		return ApplyResult{OK: false, Message: check.Message}, fmt.Errorf("%s", check.Message)
	}
	if !check.UpdateAvailable {
		return ApplyResult{OK: true, Message: "无需更新: " + check.Message}, nil
	}
	if check.AssetURL == "" {
		return ApplyResult{OK: false, Message: check.Message}, fmt.Errorf("no asset")
	}
	if installDir == "" {
		exe, _ := os.Executable()
		installDir = filepath.Dir(exe)
	}
	tmp := filepath.Join(os.TempDir(), "devin-byok-update-"+strconv.FormatInt(time.Now().Unix(), 10))
	_ = os.MkdirAll(tmp, 0o755)
	zipPath := filepath.Join(tmp, check.AssetName)
	if err := downloadFile(ctx, check.AssetURL, zipPath); err != nil {
		return ApplyResult{OK: false, Message: err.Error()}, err
	}
	if check.SHA256URL != "" {
		sumPath := zipPath + ".sha256"
		if err := downloadFile(ctx, check.SHA256URL, sumPath); err == nil {
			want, _ := os.ReadFile(sumPath)
			got, err := fileSHA256(zipPath)
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
	extractDir := filepath.Join(tmp, "extract")
	_ = os.MkdirAll(extractDir, 0o755)
	if err := unzip(zipPath, extractDir); err != nil {
		return ApplyResult{OK: false, Message: "unzip: " + err.Error()}, err
	}
	script := filepath.Join(tmp, "apply-update.cmd")
	// 等主进程退出后覆盖 exe，再可选 start
	content := fmt.Sprintf(`@echo off
setlocal
echo [devin-byok] waiting for process exit...
timeout /t 2 /nobreak >nul
set SRC=%s
set DST=%s
copy /Y "%%SRC%%\devin-byok.exe" "%%DST%%\devin-byok.exe" >nul
copy /Y "%%SRC%%\devin-byok-gui.exe" "%%DST%%\devin-byok-gui.exe" >nul
if exist "%%SRC%%\devin-byok-ls-wrapper.exe" copy /Y "%%SRC%%\devin-byok-ls-wrapper.exe" "%%DST%%\devin-byok-ls-wrapper.exe" >nul
if exist "%%SRC%%\README.md" copy /Y "%%SRC%%\README.md" "%%DST%%\README.md" >nul
if exist "%%SRC%%\config.example.yaml" copy /Y "%%SRC%%\config.example.yaml" "%%DST%%\config.example.yaml" >nul
echo [devin-byok] update applied
start "" "%%DST%%\devin-byok.exe" start
start "" "%%DST%%\devin-byok-gui.exe"
endlocal
`, extractDir, installDir)
	// Fix the batch - I used wrong escaping. Write proper batch.
	content = "@echo off\r\n" +
		"setlocal\r\n" +
		"echo [devin-byok] applying update...\r\n" +
		"timeout /t 2 /nobreak >nul\r\n" +
		"set \"SRC=" + extractDir + "\"\r\n" +
		"set \"DST=" + installDir + "\"\r\n" +
		"copy /Y \"%SRC%\\devin-byok.exe\" \"%DST%\\devin-byok.exe\"\r\n" +
		"copy /Y \"%SRC%\\devin-byok-gui.exe\" \"%DST%\\devin-byok-gui.exe\"\r\n" +
		"if exist \"%SRC%\\devin-byok-ls-wrapper.exe\" copy /Y \"%SRC%\\devin-byok-ls-wrapper.exe\" \"%DST%\\devin-byok-ls-wrapper.exe\"\r\n" +
		"if exist \"%SRC%\\README.md\" copy /Y \"%SRC%\\README.md\" \"%DST%\\README.md\"\r\n" +
		"if exist \"%SRC%\\config.example.yaml\" copy /Y \"%SRC%\\config.example.yaml\" \"%DST%\\config.example.yaml\"\r\n" +
		"echo [devin-byok] update applied to %DST%\r\n" +
		"start \"\" \"%DST%\\devin-byok.exe\" start\r\n" +
		"start \"\" \"%DST%\\devin-byok-gui.exe\"\r\n" +
		"endlocal\r\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return ApplyResult{OK: false, Message: err.Error()}, err
	}
	cmd := exec.Command("cmd", "/c", "start", "", script)
	cmd.Dir = tmp
	if err := cmd.Start(); err != nil {
		return ApplyResult{OK: false, Message: err.Error()}, err
	}
	return ApplyResult{OK: true, Message: "已下载 " + check.Latest + "，即将替换并重启", Script: script, ZipPath: zipPath}, nil
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
	// 使用 PowerShell Expand-Archive，避免再引 archive/zip 依赖问题
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Expand-Archive -LiteralPath '%s' -DestinationPath '%s' -Force", zipPath, dest))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(out))
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
	return filepath.Dir(exe)
}

// GOOS helper for tests
func IsWindows() bool { return runtime.GOOS == "windows" }
