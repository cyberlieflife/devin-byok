package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"devin-byok/internal/config"
	"devin-byok/internal/desktop"
	"devin-byok/internal/devin"
	"devin-byok/internal/localapi"
	"devin-byok/internal/logx"
	"devin-byok/internal/lsinstall"
	"devin-byok/internal/paths"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

var (
	title      = "Devin BYOK"
	user32     = syscall.NewLazyDLL("user32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	procFind   = user32.NewProc("FindWindowW")
	procShow   = user32.NewProc("ShowWindow")
	procIsIc   = user32.NewProc("IsIconic")
	procSetFG  = user32.NewProc("SetForegroundWindow")
	procMsgBox = user32.NewProc("MessageBoxW")

	embedMu   sync.Mutex
	embedHTTP *http.Server

	singletonMutex uintptr
)

const (
	swHide             = 0
	swRestore          = 9
	swShow             = 5
	errorAlreadyExists = 183
	mbIconError        = 0x00000010
)

func main() {
	runtime.LockOSThread()
	setupFileLog()
	hideConsole()

	if !ensureSingleInstance() {
		bringExistingToFront()
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

	prefs := desktop.LoadPrefs()
	systray.Register(onTrayReady, onTrayExit)

	if err := startService(); err != nil {
		logx.Warnf("start service: %v", err)
	} else {
		go autoInstallWrapper()
	}
	waitAPI(80)

	uiURL := "http://127.0.0.1:8787/ui/"
	dataPath := filepath.Join(os.Getenv("APPDATA"), "devin-byok", "webview2")
	_ = os.MkdirAll(dataPath, 0o755)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  1100,
			Height: 780,
			Center: true,
		},
	})
	if w == nil {
		logx.Errorf("webview2 init failed; dataPath=%s", dataPath)
		messageBox(title, "无法初始化 WebView2 窗口。\n\n请安装 Microsoft Edge WebView2 Runtime 后重试。\n将尝试用系统浏览器打开管理页。", mbIconError)
		_ = exec.Command("cmd", "/c", "start", "", uiURL).Start()
		select {}
	}

	_ = w.Bind("nativeStart", func() string {
		if err := startService(); err != nil {
			return "error: " + err.Error()
		}
		go autoInstallWrapper()
		if waitAPI(50) {
			return "ok"
		}
		return "started but api not ready"
	})
	_ = w.Bind("nativeStop", func() string { return stopService() })
	_ = w.Bind("nativeOnline", func() bool { return online() })
	_ = w.Bind("nativeHideToTray", func() string { hideMainWindow(); return "ok" })
	_ = w.Bind("nativeShowWindow", func() string { showMainWindow(); return "ok" })
	_ = w.Bind("nativeInstallWrapper", func() string {
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
	_ = w.Bind("nativeQuit", func() string {
		go func() {
			time.Sleep(400 * time.Millisecond)
			quitApp()
		}()
		return "ok"
	})

	w.SetSize(1100, 780, webview2.HintNone)
	w.Navigate(uiURL)

	go watchMinimize()
	if prefs.StartMinimized {
		go func() {
			time.Sleep(700 * time.Millisecond)
			hideMainWindow()
		}()
	}
	w.Run()
	quitApp()
}

func autoInstallWrapper() {
	cfg, err := config.Load(paths.FindConfig())
	if err != nil {
		return
	}
	meta, err := lsinstall.InstallIfNeeded(cfg.Devin.InstallDir)
	if err != nil {
		logx.Warnf("auto install wrapper: %v", err)
		return
	}
	logx.Infof("wrapper installed: %s", meta.Target)
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
	// stop service + restore portal on process exit
	_ = stopService()
	systray.Quit()
	time.Sleep(80 * time.Millisecond)
	os.Exit(0)
}

func watchMinimize() {
	for {
		time.Sleep(400 * time.Millisecond)
		if !desktop.LoadPrefs().MinimizeToTray {
			continue
		}
		if h := findMainHWND(); h != 0 {
			if r, _, _ := procIsIc.Call(h); r != 0 {
				procShow.Call(h, uintptr(swHide))
			}
		}
	}
}

func findMainHWND() uintptr {
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	h, _, _ := procFind.Call(0, uintptr(unsafe.Pointer(ptr)))
	return h
}

func hideMainWindow() {
	if h := findMainHWND(); h != 0 {
		procShow.Call(h, uintptr(swHide))
	}
}

func showMainWindow() {
	h := findMainHWND()
	if h == 0 {
		return
	}
	procShow.Call(h, uintptr(swRestore))
	procShow.Call(h, uintptr(swShow))
	procSetFG.Call(h)
}

func bringExistingToFront() {
	for i := 0; i < 15; i++ {
		if findMainHWND() != 0 {
			showMainWindow()
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
	showMainWindow()
}

func hideConsole() {
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	freeConsole := kernel32.NewProc("FreeConsole")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow := user32.NewProc("ShowWindow")
		showWindow.Call(hwnd, 0)
		freeConsole.Call()
	}
}

func messageBox(caption, text string, flags uintptr) {
	c, _ := syscall.UTF16PtrFromString(caption)
	t, _ := syscall.UTF16PtrFromString(text)
	procMsgBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
}

func setupFileLog() {
	dir := filepath.Join(os.Getenv("APPDATA"), "devin-byok")
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

func startService() error {
	return startEmbedded()
}

func stopService() string {
	embedMu.Lock()
	hasEmbed := embedHTTP != nil
	if hasEmbed {
		_ = embedHTTP.Close()
		embedHTTP = nil
	}
	embedMu.Unlock()
	if hasEmbed {
		_, _ = devin.RestorePortal()
		return "ok"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/control/stop", nil)
	if err == nil {
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			return "ok"
		}
	}
	_, _ = devin.RestorePortal()
	return "ok"
}

func startEmbedded() error {
	if online() {
		applyFromConfig()
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
	applyFromConfig()
	return nil
}

func applyFromConfig() {
	cfg, err := config.Load(paths.FindConfig())
	if err != nil {
		return
	}
	if _, err := devin.ApplyPortal(cfg.Server.PublicBase, cfg.Devin.PortalURLKeys); err != nil {
		logx.Warnf("apply: %v", err)
	} else {
		logx.Infof("apply ok")
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

func ensureSingleInstance() bool {
	createMutex := kernel32.NewProc("CreateMutexW")
	closeHandle := kernel32.NewProc("CloseHandle")
	name, err := syscall.UTF16PtrFromString(`Local\DevinBYOK_GUI_Singleton`)
	if err != nil {
		return true
	}
	handle, _, errno := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true
	}
	if errno == syscall.Errno(errorAlreadyExists) {
		_, _, _ = closeHandle.Call(handle)
		return false
	}
	singletonMutex = handle
	return true
}