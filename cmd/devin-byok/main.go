package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/desktop"
	"devin-byok/internal/devin"
	"devin-byok/internal/extinstall"
	"devin-byok/internal/localapi"
	"devin-byok/internal/logx"
	"devin-byok/internal/lsinstall"
	"devin-byok/internal/paths"
	"devin-byok/internal/platform"
	"devin-byok/internal/update"
	"devin-byok/internal/upstream/openai"
	"devin-byok/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}
	cmd := os.Args[1]
	cfgPath := findConfig()
	switch cmd {
	case "help", "-h", "--help":
		printHelp()
	case "serve":
		mustServe(cfgPath)
	case "start":
		mustStartBackground(cfgPath)
	case "stop":
		mustStopBackground()
	case "autostart":
		if len(os.Args) < 3 {
			fmt.Println("usage: devin-byok autostart on|off|status")
			os.Exit(1)
		}
		mustAutostart(os.Args[2])
	case "test-upstream":
		mustTestUpstream(cfgPath)
	case "detect":
		mustDetect()
	case "apply":
		mustApply(cfgPath)
	case "restore":
		mustRestore()
	case "status", "doctor":
		mustStatus(cfgPath)
	case "checklist":
		printToolChecklist()
	case "gui":
		mustGUI()
	case "install":
		mustInstall()
	case "uninstall":
		mustUninstall()
	case "reload":
		mustReloadHint()
	case "update":
		mustUpdate(cfgPath, os.Args[2:])
	case "capture-hint":
		printCaptureHint()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`devin-byok - Devin 免登录 OpenAI 兼容 BYOK

用法:
  devin-byok install           安装 language_server wrapper
  devin-byok uninstall         卸载 wrapper
  devin-byok serve             前台启动本地 API（支持配置热重载）
  devin-byok start             后台启动 serve（写 PID，可配合开机自启）
  devin-byok stop              停止后台 serve
  devin-byok autostart on|off|status
                               开机自启开关
  devin-byok doctor|status     健康检查
  devin-byok test-upstream     测试上游
  devin-byok apply|restore     portalUrl 辅助
  devin-byok help

推荐:
  1) 编辑配置文件 ` + paths.ConfigPath() + `
  2) devin-byok install
  3) devin-byok start
  4) devin-byok autostart on   # 可选
  5) 重启 Devin，选择 BYOK 模型
`)
}

func findConfig() string {
	return paths.FindConfig()
}

func projectRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := platform.ReleaseInstallDir(exe)
		if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "scripts")); err == nil {
			return dir
		}
		if _, err := os.Stat(platform.GUIPath(dir)); err == nil {
			return dir
		}
	}
	return platform.DataDir()
}

func pidFile() string {
	dir := platform.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "serve.pid")
}

func mustServe(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Errorf("load config: %v", err)
		os.Exit(1)
	}
	captureDir := paths.CaptureDir()
	srv := localapi.New(cfg, captureDir)
	abs, _ := filepath.Abs(cfgPath)
	srv.SetConfigPath(abs)
	srv.StartConfigWatch(2 * time.Second)
	if _, err := extinstall.InstallFromFS(extinstall.ExtFS, extinstall.ExtRoot); err != nil {
		logx.Warnf("extension install: %v", err)
	} else {
		_ = extinstall.Enable()
		logx.Infof("extension enabled")
	}
	localapi.MetricsLoad()
	localapi.MetricsStartPeriodicSave(30 * time.Second)
	defer localapi.MetricsSave()

	addr := cfg.Addr()
	logx.Infof("local-api listening on http://%s", addr)
	logx.Infof("config hot-reload: %s", abs)
	logx.Infof("pure_local=%v stream=%v cascade_tools=%v deepwiki=%v codemap=%v model=%s models=%d",
		cfg.Features.PureLocal, cfg.Features.EnableStream, cfg.Features.EnableCascadeTools, cfg.Features.EnableDeepWiki, cfg.Features.EnableCodeMap,
		cfg.DefaultModelID(), len(cfg.ModelList()))
	logx.Infof("rpc log: %s", filepath.Join(captureDir, "localapi-rpc.jsonl"))
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		logx.Errorf("serve: %v", err)
		os.Exit(1)
	}
}

func mustStartBackground(cfgPath string) {
	// 若已在跑 healthz，直接提示
	if resp, err := http.Get("http://127.0.0.1:8787/healthz"); err == nil {
		resp.Body.Close()
		logx.Infof("local-api already UP")
		applyOnStart(cfgPath)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		logx.Errorf("executable: %v", err)
		os.Exit(1)
	}
	// 用同一 exe serve
	cmd := exec.Command(exe, "serve")
	cmd.Dir = projectRoot()
	// Windows 下隐藏窗口
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		logx.Errorf("start: %v", err)
		os.Exit(1)
	}
	_ = os.WriteFile(pidFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	// 等待就绪
	ok := false
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		if resp, err := http.Get("http://127.0.0.1:8787/healthz"); err == nil {
			resp.Body.Close()
			ok = true
			break
		}
	}
	if !ok {
		logx.Warnf("started pid=%d but healthz not ready yet", cmd.Process.Pid)
	} else {
		logx.Infof("started background serve pid=%d", cmd.Process.Pid)
	}
	applyOnStart(cfgPath)
}

func mustStopBackground() {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/control/stop", nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			for i := 0; i < 25; i++ {
				time.Sleep(150 * time.Millisecond)
				if r, e := http.Get("http://127.0.0.1:8787/healthz"); e != nil {
					_ = os.Remove(pidFile())
					restoreOnStop()
					logx.Infof("stop done (graceful)")
					return
				} else if r != nil {
					r.Body.Close()
				}
			}
		}
	}
	if b, err := os.ReadFile(pidFile()); err == nil {
		pid := strings.TrimSpace(string(b))
		killArgs := platform.KillCommand(pid)
		_ = exec.Command(killArgs[0], killArgs[1:]...).Run()
		_ = os.Remove(pidFile())
	}
	restoreOnStop()
	logx.Infof("stop done")
}

func mustAutostart(mode string) {
	exe, _ := os.Executable()
	_ = desktop.SetAutostart(strings.ToLower(mode) == "on", projectRoot(), exe)
	switch strings.ToLower(mode) {
	case "on":
		if desktop.AutostartEnabled() {
			logx.Infof("autostart ON")
		} else {
			logx.Errorf("autostart on failed")
			os.Exit(1)
		}
	case "off":
		logx.Infof("autostart OFF")
	case "status":
		if desktop.AutostartEnabled() {
			logx.Infof("autostart: ON")
		} else {
			logx.Infof("autostart: OFF")
		}
	default:
		fmt.Println("usage: devin-byok autostart on|off|status")
		os.Exit(1)
	}
}

func mustReloadHint() {
	logx.Infof("serve 已支持配置热重载：保存 config.yaml 后约 2 秒自动生效，无需重启 serve")
}

func mustTestUpstream(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Errorf("load config: %v", err)
		os.Exit(1)
	}
	c := openai.New(cfg.Upstream)
	logx.Infof("endpoint: %s", c.Endpoint())
	logx.Infof("model: %s", cfg.DefaultModelID())
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Upstream.TimeoutSec)*time.Second)
	defer cancel()
	text, err := c.Ping(ctx)
	if err != nil {
		logx.Errorf("test-upstream failed: %v", err)
		os.Exit(1)
	}
	logx.Infof("test-upstream ok: %q", text)
}

func mustDetect() {
	p, err := devin.ResolvePaths()
	if err != nil {
		logx.Errorf("detect: %v", err)
		os.Exit(1)
	}
	logx.Infof("user data: %s", p.UserDataDir)
	logx.Infof("settings: %s", p.SettingsJSON)
}

func applyOnStart(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Warnf("start apply skip (load config): %v", err)
		return
	}
	res, err := devin.ApplyPortal(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys)
	if err != nil {
		logx.Warnf("start apply failed: %v", err)
		return
	}
	logx.Infof("start apply ok portal=%s settings=%s", res.PortalURL, res.SettingsPath)
	if err := applyDevKeys(); err != nil {
		logx.Warnf("start apply dev keys failed: %v", err)
	}
}

func applyDevKeys() error {
	wpath, err := lsinstall.MaterializeWrapper()
	if err != nil {
		return err
	}
	return devin.ApplyDevKeys(wpath)
}

func restoreOnStop() {
	res, err := devin.RestorePortal()
	if err != nil {
		logx.Warnf("stop restore: %v (maybe not applied yet)", err)
		return
	}
	logx.Infof("stop restore ok: %v", res.Restored)
}

func mustUpdate(cfgPath string, args []string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Errorf("load: %v", err)
		os.Exit(1)
	}
	ucfg := update.Config{Enabled: cfg.Update.Enabled, Repo: cfg.Update.Repo, AssetContains: cfg.Update.AssetContains, AutoApply: cfg.Update.AutoApply, CheckURL: cfg.Update.CheckURL}
	sub := "check"
	if len(args) > 0 {
		sub = args[0]
	}
	ctx := context.Background()
	switch sub {
	case "check", "status":
		res := update.Check(ctx, ucfg, version.Version)
		logx.Infof("current=%s latest=%s available=%v msg=%s", res.Current, res.Latest, res.UpdateAvailable, res.Message)
		if res.ReleaseURL != "" {
			logx.Infof("release: %s", res.ReleaseURL)
		}
		if !res.OK {
			os.Exit(1)
		}
	case "apply", "install":
		res, err := update.DownloadAndSchedule(ctx, ucfg, version.Version, update.DefaultInstallDir())
		if err != nil {
			logx.Errorf("update apply: %v", err)
			os.Exit(1)
		}
		logx.Infof("%s", res.Message)
		if res.OK && res.Script != "" {
			logx.Infof("updater script: %s (exiting so files can be replaced)", res.Script)
			os.Exit(0)
		}
	default:
		fmt.Println("usage: devin-byok update check|apply")
		os.Exit(1)
	}
}

func mustApply(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Errorf("load config: %v", err)
		os.Exit(1)
	}
	res, err := devin.ApplyPortal(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys)
	if err != nil {
		logx.Errorf("apply: %v", err)
		os.Exit(1)
	}
	logx.Infof("apply ok -> %s (%+v)", cfg.Server.PublicBase, res)
	if err := applyDevKeys(); err != nil {
		logx.Errorf("apply dev keys: %v", err)
		os.Exit(1)
	}
}

func mustRestore() {
	res, err := devin.RestorePortal()
	if err != nil {
		logx.Errorf("restore: %v", err)
		os.Exit(1)
	}
	logx.Infof("restore ok (%+v)", res)
}

func mustInstall() {
	cfgPath := findConfig()
	installDir := platform.DefaultInstallDir()
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Devin.InstallDir != "" {
		installDir = cfg.Devin.InstallDir
	}
	meta, err := lsinstall.Install(installDir)
	if err != nil {
		logx.Errorf("install wrapper: %v", err)
		os.Exit(1)
	}
	logx.Infof("install done target=%s real=%s", meta.Target, meta.Real)
	logx.Infof("Next: devin-byok start && restart Devin")
}

func mustUninstall() {
	cfgPath := findConfig()
	installDir := platform.DefaultInstallDir()
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Devin.InstallDir != "" {
		installDir = cfg.Devin.InstallDir
	}
	if err := lsinstall.Uninstall(installDir); err != nil {
		logx.Errorf("uninstall wrapper: %v", err)
		os.Exit(1)
	}
	logx.Infof("uninstall done")
}

func mustStatus(cfgPath string) {
	root := projectRoot()
	logx.Infof("project: %s", root)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logx.Warnf("config: %v", err)
	} else {
		logx.Infof("config: %s", cfgPath)
		logx.Infof("portal: %s", cfg.Server.PublicBase)
		logx.Infof("upstream: %s", openai.New(cfg.Upstream).Endpoint())
		logx.Infof("default model: %s", cfg.DefaultModelID())
		logx.Infof("features pure_local=%v stream=%v cascade_tools=%v deepwiki=%v codemap=%v", cfg.Features.PureLocal, cfg.Features.EnableStream, cfg.Features.EnableCascadeTools, cfg.Features.EnableDeepWiki, cfg.Features.EnableCodeMap)
		logx.Infof("thinking param=%s default=%s", cfg.Upstream.Thinking.Param, cfg.Upstream.Thinking.Default)
		if cfg.Upstream.Sampling.Temperature != nil {
			logx.Infof("sampling temperature=%v max_tokens=%d top_p=%v", *cfg.Upstream.Sampling.Temperature, cfg.Upstream.Sampling.MaxTokens, cfg.Upstream.Sampling.TopP)
		} else {
			logx.Infof("sampling max_tokens=%d (temperature/top_p unset)", cfg.Upstream.Sampling.MaxTokens)
		}
		for _, m := range cfg.ModelList() {
			logx.Infof("  model: %s (%s) upstream=%s thinking=%s family=%s family_uid=%s ctx=%d max_out=%d", m.ID, m.Label, m.ResolveUpstream(), m.Thinking, m.Family, m.FamilyUID, m.ContextWindow, m.MaxTokens)
		}
		logx.Infof("tools mode=%s timeout_sec=%d workspace_hint=%v", cfg.ToolsMode(), cfg.ResolveChatTimeoutSec(true), cfg.ToolsWorkspaceHint())
		if len(cfg.Tools.Allow) > 0 || len(cfg.Tools.Deny) > 0 {
			logx.Infof("tools allow=%v deny=%v", cfg.Tools.Allow, cfg.Tools.Deny)
		}
		if cfg.Features.EnableCascadeTools && cfg.ToolsMode() == "off" {
			logx.Warnf("cascade_tools=true but tools.mode=off")
		}
	}
	real := filepath.Join(platform.ExtensionsBinDir(platform.DefaultInstallDir()), platform.RealLanguageServerName())
	if cfg2, err := config.Load(cfgPath); err == nil && cfg2.Devin.InstallDir != "" {
		cand := filepath.Join(platform.ExtensionsBinDir(cfg2.Devin.InstallDir), platform.RealLanguageServerName())
		if _, err := os.Stat(cand); err == nil {
			real = cand
		}
	}
	if _, err := os.Stat(real); err == nil {
		logx.Infof("wrapper: INSTALLED")
	} else {
		logx.Warnf("wrapper: NOT installed (missing %s)", platform.RealLanguageServerName())
	}

	host := "http://127.0.0.1:8787"
	if cfg2, err := config.Load(cfgPath); err == nil && cfg2.Server.PublicBase != "" {
		host = strings.TrimRight(cfg2.Server.PublicBase, "/")
	}
	if resp, err := http.Get(host + "/healthz"); err != nil {
		logx.Warnf("local-api: DOWN (%v) - run: devin-byok start", err)
	} else {
		defer resp.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&m)
		logx.Infof("local-api: UP ok=%v tools_mode=%v cascade_tools=%v stream=%v", m["ok"], m["tools_mode"], m["cascade_tools"], m["stream"])
	}

	if cfg2, err := config.Load(cfgPath); err == nil {
		c := openai.New(cfg2.Upstream)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		textPing, err := c.Ping(ctx)
		cancel()
		if err != nil {
			logx.Warnf("upstream: FAIL %v", err)
		} else {
			logx.Infof("upstream: OK %q", truncateStr(textPing, 60))
		}
	}

	rpcLog := filepath.Join(projectRoot(), "work", "capture", "localapi-rpc.jsonl")
	if b, err := os.ReadFile(rpcLog); err == nil {
		lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		logx.Infof("rpc log: %s (%d lines)", rpcLog, len(lines))
		startLn := 0
		if len(lines) > 40 {
			startLn = len(lines) - 40
		}
		errN := 0
		for _, ln := range lines[startLn:] {
			low := strings.ToLower(ln)
			if strings.Contains(low, "\"error\"") {
				errN++
			}
		}
		if errN > 0 {
			logx.Warnf("rpc recent: ~%d error-ish lines in last %d", errN, len(lines)-startLn)
		} else {
			logx.Infof("rpc recent: clean-ish (last %d lines)", len(lines)-startLn)
		}
	} else {
		logx.Infof("rpc log: (none yet)")
	}

	if b, err := os.ReadFile(pidFile()); err == nil {
		logx.Infof("pid file: %s (%s)", pidFile(), strings.TrimSpace(string(b)))
	}
	startup := desktop.AutostartPath()
	if _, err := os.Stat(startup); err == nil {
		logx.Infof("autostart: ON")
	} else {
		logx.Infof("autostart: OFF")
	}
	logx.Infof("next: devin-byok gui  or checklist")
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func printToolChecklist() {
	fmt.Print("Devin BYOK tool checklist\n\n" +
		"Pre:\n" +
		"  1) enable_cascade_tools=true\n" +
		"  2) tools.mode=standard (full for shell)\n" +
		"  3) devin-byok start + restart Devin\n\n" +
		"Cases:\n" +
		"  [ ] read config.yaml\n" +
		"  [ ] list workdir\n" +
		"  [ ] search ToolsMode\n" +
		"  [ ] write byok-p0-test.txt\n" +
		"  [ ] edit append line\n" +
		"  [ ] (full) echo byok-ok\n\n" +
		"OK logs: toolsOut>0 ; tool_calls=N names=[...]\n" +
		"Fail: doctor + localapi-rpc.jsonl + Devin.log\n")
}

func printCaptureHint() {
	fmt.Print(`抓包: serve 后重启 Devin，看 work/capture/localapi-rpc.jsonl\n`)
}

func mustGUI() {
	root := projectRoot()
	gui := platform.GUIPath(root)
	if _, err := os.Stat(gui); err != nil {
		logx.Errorf("missing %s - build: go build -o %s ./cmd/devin-byok-gui", gui, gui)
		os.Exit(1)
	}
	cmd := exec.Command(gui)
	if platform.IsDarwin() && filepath.Ext(gui) == ".app" {
		cmd = exec.Command("open", gui)
	}
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		logx.Errorf("start gui: %v", err)
		os.Exit(1)
	}
	logx.Infof("GUI started: %s", gui)
}
