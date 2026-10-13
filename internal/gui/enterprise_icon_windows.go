//go:build windows && !nogui

package gui

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

var enterpriseWindowIcons = map[w32.HWND]w32.HICON{}

func bindNativeBrandWindow(w *application.WebviewWindow) {
	w.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(*application.WindowEvent) {
		application.InvokeAsync(func() { nativeBrandWindowIcon(w, brandAppIcon()) })
	})
}

func releaseNativeBrandWindow(w *application.WebviewWindow) {
	hwnd := w32.HWND(uintptr(w.NativeWindow()))
	if icon := enterpriseWindowIcons[hwnd]; icon != 0 {
		w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, 0)
		w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, 0)
		w32.DestroyIcon(icon)
		delete(enterpriseWindowIcons, hwnd)
	}
}

// Wails beta.24's App.SetIcon does nothing on Windows. Window icons need
// WM_SETICON, independent of the tray and the signed EXE's static icon.
func nativeBrandWindowIcon(w *application.WebviewWindow, logo []byte) {
	hwnd := w32.HWND(uintptr(w.NativeWindow()))
	if hwnd == 0 {
		return
	}
	if len(logo) == 0 {
		logo = appIconLarge
	}
	icon, err := w32.CreateLargeHIconFromImage(logo)
	if err != nil {
		return
	}
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, uintptr(icon))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, uintptr(icon))
	if old := enterpriseWindowIcons[hwnd]; old != 0 {
		w32.DestroyIcon(old)
	}
	enterpriseWindowIcons[hwnd] = icon
}
