package main

import (
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

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

var (
	title    = "Devin BYOK"
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procFind = user32.NewProc("FindWindowW")
	procShow = user32.NewProc("ShowWindow")
	procIsIc = user32.NewProc("IsIconic")
	procSetFG = user32.NewProc("SetForegroundWindow")

	embedMu   sync.Mutex
	embedHTTP *http.Server

	// singletonMutex 保持命名互斥句柄，进程退出前不得 Close（否则可被第二实例抢占）
	singletonMutex uintptr
)

const (
	swHide    = 0
	swRestore = 9
	swShow    = 5
	errorAlreadyExists = 183
)

func main() {
	// systray 的 init 会 LockOSThread；这里再锁一次，确保主线程跑 UI 消息泵
	runtime.LockOSThread()
	hideConsole()

	if !ensureSingleInstance() {
		// 已有实例：尝试前置已有窗口后退出，避免双托盘
		bringExistingToFront()
		return
	}

	prefs := desktop.LoadPrefs()

	// 与 webview 共用主线程消息循环：只能用 Register，不能 go systray.Run
	// go systray.Run 会在子线程自建 GetMessage 泵，导致托盘图标在、菜单无响应
	systray.Register(onTrayReady, onTrayExit)

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
		// 无 webview 时由托盘保活；Register 需要宿主泵消息——此处退化轮询
		for {
			time.Sleep(time.Hour)
		}
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
		// 给更新脚本一点启动时间；先清托盘再退
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
	// webview 退出时清理托盘
	quitApp()
}

func onTrayReady() {
	if len(desktop.IconICO) > 0 {
		systray.SetIcon(desktop.IconICO)
	}
	systray.SetTitle(title)
	systray.SetTooltip("Devin BYOK")

	// 中文菜单，左右键点击托盘都会弹出
	mShow := systray.AddMenuItem("显示窗口", "Show window")
	mHide := systray.AddMenuItem("隐藏到托盘", "Hide to tray")
	systray.AddSeparator()
	mStart := systray.AddMenuItem("启动服务", "start + apply")
	mStop := systray.AddMenuItem("停止服务", "stop + restore")
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
			case <-mStop.ClickedCh:
				_ = stopService()
			case <-mQuit.ClickedCh:
				quitApp()
			}
		}
	}()
}

func onTrayExit() {
	// systray 退出回调：预留清理位
}

func quitApp() {
	// 仅退出 GUI：不在这里 stop/restore（更新流程依赖 nativeQuit 不抢 restore）
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
	// 第二实例启动时多试几次，等首实例窗口就绪
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
// 注意：必须用 CreateMutex 的 errno 返回值判断 ERROR_ALREADY_EXISTS，
// 不能在 Call 之后再单独 GetLastError（Go runtime 可能已清掉 last error）。
func ensureSingleInstance() bool {
	createMutex := kernel32.NewProc("CreateMutexW")
	closeHandle := kernel32.NewProc("CloseHandle")
	name, err := syscall.UTF16PtrFromString(`Local\DevinBYOK_GUI_Singleton`)
	if err != nil {
		return true
	}
	// lpMutexAttributes=nil, bInitialOwner=FALSE, lpName=...
	handle, _, errno := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		// 创建失败时放行，避免误伤启动
		return true
	}
	if errno == syscall.Errno(errorAlreadyExists) {
		_, _, _ = closeHandle.Call(handle)
		return false
	}
	// 故意不 CloseHandle：句柄活到进程退出，互斥才一直占着
	singletonMutex = handle
	return true
}
