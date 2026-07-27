package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"devin-byok/internal/desktop"

	"github.com/getlantern/systray"
	"github.com/jchv/go-webview2"
)

var (
	title  = "Devin BYOK"
	user32 = syscall.NewLazyDLL("user32.dll")
	procFind = user32.NewProc("FindWindowW")
	procShow = user32.NewProc("ShowWindow")
	procIsIc = user32.NewProc("IsIconic")
)

const (
	swHide    = 0
	swRestore = 9
	swShow    = 5
)

func main() {
	hideConsole()
	prefs := desktop.LoadPrefs()

	// 托盘放后台 goroutine；WebView2 占主线程
	go systray.Run(func() { onTrayReady() }, func() {})

	_ = ensureAPI()
	waitAPI(40)

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
		if err := startCLI(); err != nil {
			return "error: " + err.Error()
		}
		if waitAPI(40) {
			return "ok"
		}
		return "started but api not ready"
	})
	_ = w.Bind("nativeStop", func() string {
		client := &http.Client{Timeout: 3 * time.Second}
		req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/control/stop", nil)
		if req != nil {
			if resp, err := client.Do(req); err == nil {
				resp.Body.Close()
				for i := 0; i < 25; i++ {
					time.Sleep(150 * time.Millisecond)
					if _, e := http.Get("http://127.0.0.1:8787/healthz"); e != nil {
						return "ok"
					}
				}
				return "ok"
			}
		}
		cli := siblingCLI()
		cmd := exec.Command(cli, "stop")
		cmd.Dir = filepath.Dir(cli)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
		return "ok"
	})
	_ = w.Bind("nativeOnline", func() bool { return online() })
	_ = w.Bind("nativeHideToTray", func() string { hideMainWindow(); return "ok" })
	_ = w.Bind("nativeShowWindow", func() string { showMainWindow(); return "ok" })

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

	mShow := systray.AddMenuItem("显示窗口", "显示主窗口")
	mHide := systray.AddMenuItem("隐藏到托盘", "隐藏主窗口")
	systray.AddSeparator()
	mStart := systray.AddMenuItem("启动服务", "start + apply")
	mStop := systray.AddMenuItem("停止服务", "stop + restore")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出 GUI", "仅退出界面")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				showMainWindow()
			case <-mHide.ClickedCh:
				hideMainWindow()
			case <-mStart.ClickedCh:
				_ = startCLI()
			case <-mStop.ClickedCh:
				cli := siblingCLI()
				cmd := exec.Command(cli, "stop")
				cmd.Dir = filepath.Dir(cli)
				cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
				_ = cmd.Run()
			case <-mQuit.ClickedCh:
				os.Exit(0)
			}
		}
	}()
}

func watchMinimize() {
	for {
		time.Sleep(400 * time.Millisecond)
		p := desktop.LoadPrefs()
		if !p.MinimizeToTray {
			continue
		}
		hwnd := findMainHWND()
		if hwnd == 0 {
			continue
		}
		r, _, _ := procIsIc.Call(hwnd)
		if r != 0 {
			procShow.Call(hwnd, uintptr(swHide))
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

func ensureAPI() error {
	// 无论是否已在线，都调用 start（幂等 + 确保 apply）
	return startCLI()
}

func startCLI() error {
	cli := siblingCLI()
	cmd := exec.Command(cli, "start")
	cmd.Dir = filepath.Dir(cli)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
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

func siblingCLI() string {
	exe, err := os.Executable()
	if err != nil {
		return "devin-byok.exe"
	}
	cand := filepath.Join(filepath.Dir(exe), "devin-byok.exe")
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	if _, err := os.Stat(`D:\Devin-byok\devin-byok.exe`); err == nil {
		return `D:\Devin-byok\devin-byok.exe`
	}
	return cand
}