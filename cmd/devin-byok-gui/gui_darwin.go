//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"devin-byok/internal/desktop"
	"devin-byok/internal/platform"

	"github.com/webview/webview_go"
)

var (
	mainWindow      webview.WebView
	mainWindowMu    sync.Mutex
	lockFile        *os.File
)

func guiInit() bool {
	if !ensureSingleInstance() {
		bringExistingToFront()
		return false
	}
	return true
}

func guiCreateWindow(uiURL string) interface{} {
	w := webview.New(false)
	if w == nil {
		return nil
	}
	w.SetTitle(title)
	w.SetSize(1100, 780, webview.HintNone)

	mainWindowMu.Lock()
	mainWindow = w
	mainWindowMu.Unlock()

	return w
}

func guiBind(w interface{}, name string, fn interface{}) error {
	ww := w.(webview.WebView)
	ww.Bind(name, fn)
	return nil
}

func guiSetSize(w interface{}, width, height int) {
	ww := w.(webview.WebView)
	ww.SetSize(width, height, webview.HintNone)
}

func guiNavigate(w interface{}, url string) {
	ww := w.(webview.WebView)
	ww.Navigate(url)
}

func guiRun(w interface{}) {
	ww := w.(webview.WebView)
	ww.Run()
}

func guiSetIcon() {
	// macOS app icon is set via .app bundle; use tray icon instead
}

func guiWatchMinimize() {
	for {
		time.Sleep(400 * time.Millisecond)
		if !desktop.LoadPrefs().MinimizeToTray {
			continue
		}
		// macOS: webview auto-minimizes via native window manager
	}
}

func hideMainWindow() {
	mainWindowMu.Lock()
	defer mainWindowMu.Unlock()
	if mainWindow != nil {
		mainWindow.SetSize(0, 0, webview.HintMin)
	}
}

func showMainWindow() {
	mainWindowMu.Lock()
	defer mainWindowMu.Unlock()
	if mainWindow != nil {
		mainWindow.SetSize(1100, 780, webview.HintNone)
	}
}

func bringExistingToFront() {
	showMainWindow()
}

func ensureSingleInstance() bool {
	dir := platform.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	lockPath := filepath.Join(dir, "gui.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true
	}
	lockFile = f
	return true
}
