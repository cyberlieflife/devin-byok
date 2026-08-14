//go:build darwin

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void devin_order_out(void *window);
void devin_make_key_and_order_front(void *window);
void devin_activate_and_focus(void *window);
int devin_is_miniaturized(void *window);
*/
import "C"

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"devin-byok/internal/desktop"
	"devin-byok/internal/platform"

	"github.com/webview/webview_go"
)

var (
	mainWindow   webview.WebView
	mainWindowMu sync.Mutex
	lockFile     *os.File
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
	ww.Dispatch(func() { C.devin_activate_and_focus(ww.Window()) })
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
		dispatchMainWindow(func(w webview.WebView) {
			if C.devin_is_miniaturized(w.Window()) != 0 {
				C.devin_order_out(w.Window())
			}
		})
	}
}

func hideMainWindow() {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()
	if w != nil {
		w.Dispatch(func() { C.devin_order_out(w.Window()) })
	}
}

func showMainWindow() {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()
	if w != nil {
		w.Dispatch(func() { C.devin_activate_and_focus(w.Window()) })
	}
}

func dispatchMainWindow(fn func(webview.WebView)) {
	mainWindowMu.Lock()
	w := mainWindow
	mainWindowMu.Unlock()
	if w != nil {
		w.Dispatch(func() { fn(w) })
	}
}

func bringExistingToFront() {
	_ = exec.Command("open", "-a", title).Start()
}

func openBrowser(url string) {
	_ = exec.Command("open", url).Start()
}

func ensureSingleInstance() bool {
	dir := platform.DataDir()
	_ = os.MkdirAll(dir, 0o755)
	lockPath := filepath.Join(dir, "gui.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return false
		}
		return true
	}
	lockFile = f
	return true
}
