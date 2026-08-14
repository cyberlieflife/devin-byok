package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"devin-byok/internal/config"
	"devin-byok/internal/desktop"
	"devin-byok/internal/devin"
	"devin-byok/internal/extinstall"
	"devin-byok/internal/ideinject"
	"devin-byok/internal/localapi"
	"devin-byok/internal/logx"
	"devin-byok/internal/lsinstall"
	"devin-byok/internal/paths"
	"devin-byok/internal/platform"

	"github.com/getlantern/systray"
)

var (
	title = "Devin BYOK"

	embedMu   sync.Mutex
	embedHTTP *http.Server
)

func main() {
	setupFileLog()
	if !guiInit() {
		return
	}

	if path, err := paths.EnsureConfig(); err != nil {
		logx.Warnf("ensure config: %v", err)
	} else {
		logx.Infof("config: %s", path)
	}
	if wpath, err := lsinstall.MaterializeWrapper(); err != nil {
		logx.Warnf("materialize wrapper: %v", err)
	} else {
		logx.Infof("wrapper binary: %s", wpath)
	}

	// 3.7.16: 一次性还原历史对 bundle 的修改（旧方案的 ideinject 注入与 wrapper 备份残留），
	// 消除 Devin "installation appears to be corrupt" 误报。
	cfg0, err := config.Load(paths.FindConfig())
	installDir := ""
	if err == nil {
		installDir = cfg0.Devin.InstallDir
	}
	if err := ideinject.RestoreContextUsageDonut(); err != nil {
		logx.Warnf("restore ideinject: %v", err)
	} else {
		logx.Infof("ideinject restored (bundle untouched)")
	}
	lsinstall.CleanBundleArtifacts(installDir)
	logx.Infof("bundle artifacts cleaned")

	prefs := desktop.LoadPrefs()
	systray.Register(onTrayReady, onTrayExit)

	if err := startEmbedded(); err != nil {
		logx.Warnf("start management server: %v", err)
	}
	waitAPI(80)

	uiURL := "http://127.0.0.1:8787/ui/"

	w := guiCreateWindow(uiURL)
	if w == nil {
		openBrowser(uiURL)
		select {}
	}

	guiBind(w, "nativeStart", func() string {
		if err := startService(); err != nil {
			return "error: " + err.Error()
		}
		go autoInstallWrapper()
		if waitAPI(50) {
			return "ok"
		}
		return "started but api not ready"
	})
	guiBind(w, "nativeStop", func() string { return stopService() })
	guiBind(w, "nativeOnline", func() bool { return online() })
	guiBind(w, "nativeHideToTray", func() string { hideMainWindow(); return "ok" })
	guiBind(w, "nativeShowWindow", func() string { showMainWindow(); return "ok" })
	guiBind(w, "nativeInstallWrapper", func() string {
		cfg, err := config.Load(paths.FindConfig())
		if err != nil {
			return "error: " + err.Error()
		}
		meta, err := lsinstall.Install(cfg.Devin.InstallDir)
		if err != nil {
			return "error: " + err.Error()
		}
		return fmt.Sprintf("ok installed=%s", meta.Target)
	})
	guiBind(w, "nativeQuit", func() string {
		go func() {
			time.Sleep(400 * time.Millisecond)
			quitApp()
		}()
		return "ok"
	})
	guiBind(w, "nativeQuitForce", func() string {
		go func() {
			time.Sleep(200 * time.Millisecond)
			systray.Quit()
			time.Sleep(50 * time.Millisecond)
			os.Exit(0)
		}()
		return "ok"
	})

	guiSetSize(w, 1100, 780)
	guiNavigate(w, uiURL)
	guiSetIcon()

	go guiWatchMinimize()
	if prefs.StartMinimized {
		go func() {
			time.Sleep(700 * time.Millisecond)
			hideMainWindow()
		}()
	}
	guiRun(w)
	quitApp()
}

func autoInstallWrapper() {
	// 3.7.16 适配：不再向 Devin.app bundle 写入 wrapper（签名应用受系统保护，
	// 且会导致 "installation appears to be corrupt"）。
	// 改用 codeiumDev.languageServerBinaryPath 覆盖，从用户目录启动 wrapper，
	// 由 applyFromConfig 写入 settings.json 完成。
	logx.Infof("auto install wrapper: skipped (languageServerBinaryPath override in use)")
}

func onTrayReady() {
	if len(desktop.IconICO) > 0 {
		systray.SetIcon(desktop.IconICO)
	}
	systray.SetTitle(title)
	systray.SetTooltip("Devin BYOK")
	mShow := systray.AddMenuItem("显示窗口", "Show window")
	mHide := systray.AddMenuItem("隐藏到托盘", "Hide to tray")
	systray.AddSeparator()
	mStart := systray.AddMenuItem("启动服务", "start + apply")
	mStop := systray.AddMenuItem("停止服务", "stop + restore")
	mWrap := systray.AddMenuItem("安装 LS Wrapper", "install language_server wrapper")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出程序", "Quit GUI")
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				showMainWindow()
			case <-mHide.ClickedCh:
				hideMainWindow()
			case <-mStart.ClickedCh:
				_ = startService()
				go autoInstallWrapper()
			case <-mStop.ClickedCh:
				_ = stopService()
			case <-mWrap.ClickedCh:
				go autoInstallWrapper()
			case <-mQuit.ClickedCh:
				quitApp()
			}
		}
	}()
}

func onTrayExit() {}

func quitApp() {
	_ = stopService()
	closeEmbedded()
	systray.Quit()
	time.Sleep(80 * time.Millisecond)
	os.Exit(0)
}

func startService() error {
	if err := startEmbedded(); err != nil {
		return err
	}
	if !waitAPI(50) {
		return fmt.Errorf("management API not ready")
	}
	return postControl("start")
}

func stopService() string {
	if online() {
		if err := postControl("stop"); err == nil {
			return "ok"
		}
	}
	_ = extinstall.Disable()
	_, _ = devin.RestorePortal()
	_ = ideinject.RestoreContextUsageDonut()
	return "ok"
}

func startEmbedded() error {
	if online() {
		return nil
	}
	cfgPath := paths.FindConfig()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	captureDir := paths.CaptureDir()
	_ = os.MkdirAll(captureDir, 0o755)
	srv := localapi.New(cfg, captureDir)
	abs, _ := filepath.Abs(cfgPath)
	srv.SetConfigPath(abs)
	srv.StartConfigWatch(2 * time.Second)
	localapi.MetricsLoad()
	localapi.MetricsStartPeriodicSave(30 * time.Second)

	addr := cfg.Addr()
	hs := &http.Server{Addr: addr, Handler: srv.Handler()}
	embedMu.Lock()
	embedHTTP = hs
	embedMu.Unlock()
	go func() {
		logx.Infof("embedded local-api on http://%s", addr)
		if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Warnf("embedded serve: %v", err)
		}
	}()
	time.Sleep(250 * time.Millisecond)
	return nil
}

func closeEmbedded() {
	embedMu.Lock()
	hs := embedHTTP
	embedHTTP = nil
	embedMu.Unlock()
	if hs != nil {
		_ = hs.Close()
	}
}

func postControl(action string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/control/"+action, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("control %s failed: HTTP %d", action, resp.StatusCode)
	}
	return nil
}

func applyFromConfig() {
	cfg, err := config.Load(paths.FindConfig())
	if err != nil {
		return
	}
	if wpath, err := lsinstall.MaterializeWrapper(); err == nil {
		if _, err := devin.ApplyLocalRuntime(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys, wpath); err != nil {
			logx.Warnf("apply local runtime: %v", err)
		} else {
			logx.Infof("apply local runtime ok: %s", wpath)
		}
	} else {
		logx.Warnf("materialize wrapper: %v", err)
	}
	// 3.7.16: 不再向 bundle 注入任何内容（ideinject 已停用）
	if _, err := extinstall.InstallFromFS(extinstall.ExtFS, extinstall.ExtRoot); err != nil {
		logx.Warnf("extension install: %v", err)
	} else {
		_ = extinstall.Enable()
		logx.Infof("extension enabled: %s", extinstall.ExtID)
	}
}

func online() bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:8787/healthz")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func waitAPI(n int) bool {
	for i := 0; i < n; i++ {
		if online() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return online()
}

func setupFileLog() {
	dir := platform.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "gui.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	os.Stdout = f
	os.Stderr = f
	logx.Infof("gui log -> %s", path)
}
