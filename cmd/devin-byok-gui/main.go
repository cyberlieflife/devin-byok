package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"devin-byok/internal/config"
	"devin-byok/internal/desktop"
	"devin-byok/internal/devin"
	"devin-byok/internal/localapi"
	"devin-byok/internal/logx"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

var (
	title    = "Devin BYOK"
	user32   = syscall.NewLazyDLL("user32.dll")
	procFind = user32.NewProc("FindWindowW")
	procShow = user32.NewProc("ShowWindow")
	procIsIc = user32.NewProc("IsIconic")

	embedMu   sync.Mutex
	embedHTTP *http.Server
)

const (
	swHide    = 0
	swRestore = 9
	swShow    = 5
)

func main() {
	hideConsole()
	if !ensureSingleInstance() {
		// 已有实例：尝试前置窗口后退出
		showMainWindow()
		return
	}
	prefs := desktop.LoadPrefs()

	go systray.Run(func() { onTrayReady() }, func() {})

	if err := startService(); err != nil {
		logx.Warnf("start service: %v", err)
	}
	waitAPI(50)

	uiURL := "http://127.0.0.1:8787/ui/"
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  1100,
			Height: 780,
			Center: true,
		},
	})
	if w == nil {
		_ = exec.Command("cmd", "/c", "start", "", uiURL).Start()
		select {}
	}

	_ = w.Bind("nativeStart", func() string {
		if err := startService(); err != nil {
			return "error: " + err.Error()
		}
		if waitAPI(50) {
			return "ok"
		}
		return "started but api not ready"
	})
	_ = w.Bind("nativeStop", func() string { return stopService() })
	_ = w.Bind("nativeOnline", func() bool { return online() })
	_ = w.Bind("nativeHideToTray", func() string { hideMainWindow(); return "ok" })
	_ = w.Bind("nativeShowWindow", func() string { showMainWindow(); return "ok" })
	_ = w.Bind("nativeQuit", func() string {
		// 给更新脚本时间启动
		go func() {
			time.Sleep(400 * time.Millisecond)
			os.Exit(0)
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
}

func onTrayReady() {
	if len(desktop.IconICO) > 0 {
		systray.SetIcon(desktop.IconICO)
	}
	systray.SetTitle(title)
	systray.SetTooltip("Devin BYOK")
	mShow := systray.AddMenuItem("Show", "Show window")
	mHide := systray.AddMenuItem("Hide to tray", "Hide window")
	systray.AddSeparator()
	mStart := systray.AddMenuItem("Start service", "start + apply")
	mStop := systray.AddMenuItem("Stop service", "stop + restore")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit GUI", "Quit UI")
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				showMainWindow()
			case <-mHide.ClickedCh:
				hideMainWindow()
			case <-mStart.ClickedCh:
				_ = startService()
			case <-mStop.ClickedCh:
				_ = stopService()
			case <-mQuit.ClickedCh:
				os.Exit(0)
			}
		}
	}()
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
	if h := findMainHWND(); h != 0 {
		procShow.Call(h, uintptr(swRestore))
		procShow.Call(h, uintptr(swShow))
	}
}

func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	freeConsole := kernel32.NewProc("FreeConsole")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		u := syscall.NewLazyDLL("user32.dll")
		showWindow := u.NewProc("ShowWindow")
		showWindow.Call(hwnd, 0)
		freeConsole.Call()
	}
}

func startService() error {
	if cli, ok := siblingCLI(); ok {
		cmd := exec.Command(cli, "start")
		cmd.Dir = filepath.Dir(cli)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return cmd.Start()
	}
	return startEmbedded()
}

func stopService() string {
	// 内嵌服务优先本地关闭，避免 /api/control/stop 的 os.Exit 杀掉 GUI
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
			for i := 0; i < 20; i++ {
				time.Sleep(150 * time.Millisecond)
				if !online() {
					return "ok"
				}
			}
			return "ok"
		}
	}
	if cli, ok := siblingCLI(); ok {
		cmd := exec.Command(cli, "stop")
		cmd.Dir = filepath.Dir(cli)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
		return "ok"
	}
	_, _ = devin.RestorePortal()
	return "ok"
}


func startEmbedded() error {
	if online() {
		applyFromConfig()
		return nil
	}
	cfgPath := findConfig()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	root := projectRoot()
	captureDir := filepath.Join(root, "work", "capture")
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
	// apply portal
	time.Sleep(200 * time.Millisecond)
	applyFromConfig()
	return nil
}

func applyFromConfig() {
	cfg, err := config.Load(findConfig())
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
	resp, err := http.Get("http://127.0.0.1:8787/healthz")
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
		time.Sleep(150 * time.Millisecond)
	}
	return online()
}

func siblingCLI() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	self := filepath.Base(exe)
	dir := filepath.Dir(exe)
	for _, name := range []string{"devin-byok.exe"} {
		if self == name {
			continue
		}
		cand := filepath.Join(dir, name)
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, true
		}
	}
	return "", false
}

func findConfig() string {
	var cands []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands, filepath.Join(dir, "config.yaml"))
	}
	cands = append(cands, "config.yaml")
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		cands = append(cands, filepath.Join(appdata, "devin-byok", "config.yaml"))
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// 首次从 example 生成
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		ex := filepath.Join(dir, "config.example.yaml")
		dst := filepath.Join(dir, "config.yaml")
		if b, err := os.ReadFile(ex); err == nil {
			_ = os.WriteFile(dst, b, 0o644)
			return dst
		}
	}
	return "config.yaml"
}

func projectRoot() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}


// ensureSingleInstance 禁止多个 GUI 同时运行（命名互斥量）。
func ensureSingleInstance() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	getLastError := kernel32.NewProc("GetLastError")
	name, err := syscall.UTF16PtrFromString("Local\\DevinBYOK_GUI_Singleton")
	if err != nil {
		return true
	}
	handle, _, _ := createMutex.Call(0, 1, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true
	}
	// ERROR_ALREADY_EXISTS = 183
	errCode, _, _ := getLastError.Call()
	if errCode == 183 {
		return false
	}
	// 保持互斥量直到进程退出（故意不 CloseHandle）
	_ = handle
	return true
}
