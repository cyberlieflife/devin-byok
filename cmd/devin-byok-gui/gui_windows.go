//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"devin-byok/internal/desktop"
	"devin-byok/internal/logx"
	"devin-byok/internal/version"

	"github.com/jchv/go-webview2"
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")
	procFind   = user32.NewProc("FindWindowW")
	procShow   = user32.NewProc("ShowWindow")
	procIsIc   = user32.NewProc("IsIconic")
	procSetFG  = user32.NewProc("SetForegroundWindow")
	procMsgBox = user32.NewProc("MessageBoxW")

	singletonMutex uintptr
)

const (
	swHide             = 0
	swRestore          = 9
	swShow             = 5
	errorAlreadyExists = 183
	mbIconError        = 0x00000010
)

func guiInit() bool {
	hideConsole()
	cleanWebView2CacheOnUpgrade()
	if !ensureSingleInstance() {
		bringExistingToFront()
		return false
	}
	return true
}

// cleanWebView2CacheOnUpgrade 在版本升级后清空 WebView2 持久缓存：
// WebView2 数据目录跨版本复用，可能缓存旧版 index.html/app.js 等静态资源，
// 导致界面停留在旧版（版本号/按钮/模型列表异常）。用 webview2.version 标记
// 记录最近一次清理时的版本，不一致即清空缓存目录，保证升级后全新加载。
func cleanWebView2CacheOnUpgrade() {
	base := filepath.Join(os.Getenv("APPDATA"), "devin-byok")
	dataPath := filepath.Join(base, "webview2")
	verFile := filepath.Join(base, "webview2.version")
	cur := version.Version
	if b, err := os.ReadFile(verFile); err == nil && strings.TrimSpace(string(b)) == cur {
		return // 版本一致：无需清理
	}
	_ = os.RemoveAll(dataPath)
	_ = os.MkdirAll(dataPath, 0o755)
	_ = os.WriteFile(verFile, []byte(cur), 0o644)
}

func guiCreateWindow(uiURL string) interface{} {
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
		cmd := exec.Command("cmd", "/c", "start", "", uiURL)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Start()
		return nil
	}
	return w
}

func guiBind(w interface{}, name string, fn interface{}) error {
	ww := w.(webview2.WebView)
	_ = ww.Bind(name, fn)
	return nil
}

func guiSetSize(w interface{}, width, height int) {
	ww := w.(webview2.WebView)
	ww.SetSize(width, height, webview2.HintNone)
}

func guiNavigate(w interface{}, url string) {
	ww := w.(webview2.WebView)
	ww.Navigate(url)
}

func guiRun(w interface{}) {
	ww := w.(webview2.WebView)
	ww.Run()
}

func guiSetIcon() {
	setWindowIconFromICO()
}

func guiWatchMinimize() {
	watchMinimize()
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

func findMainHWND() uintptr {
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	h, _, _ := procFind.Call(0, uintptr(unsafe.Pointer(ptr)))
	return h
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

func openBrowser(url string) {
	cmd := exec.Command("cmd", "/c", "start", "", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
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

func setWindowIconFromICO() {
	if len(desktop.IconICO) == 0 {
		return
	}
	dir := filepath.Join(os.Getenv("APPDATA"), "devin-byok")
	_ = os.MkdirAll(dir, 0o755)
	icoPath := filepath.Join(dir, "app-icon.ico")
	if err := os.WriteFile(icoPath, desktop.IconICO, 0o644); err != nil {
		return
	}
	go func() {
		for i := 0; i < 50; i++ {
			h := findMainHWND()
			if h != 0 {
				applyIconFile(h, icoPath)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func applyIconFile(hwnd uintptr, icoPath string) {
	const (
		imageIcon      = 1
		lrLoadFromFile = 0x00000010
		lrDefaultSize  = 0x00000040
		wmSetIcon      = 0x0080
		iconSmall      = 0
		iconBig        = 1
	)
	user32 := syscall.NewLazyDLL("user32.dll")
	loadImage := user32.NewProc("LoadImageW")
	sendMessage := user32.NewProc("SendMessageW")
	pathPtr, err := syscall.UTF16PtrFromString(icoPath)
	if err != nil {
		return
	}
	hBig, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, 32, 32, lrLoadFromFile)
	hSmall, _, _ := loadImage.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, 16, 16, lrLoadFromFile)
	if hBig != 0 {
		sendMessage.Call(hwnd, wmSetIcon, iconBig, hBig)
	}
	if hSmall != 0 {
		sendMessage.Call(hwnd, wmSetIcon, iconSmall, hSmall)
	}
}
