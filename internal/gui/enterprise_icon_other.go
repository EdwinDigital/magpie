//go:build !windows && !nogui

package gui

import "github.com/wailsapp/wails/v3/pkg/application"

func nativeBrandWindowIcon(*application.WebviewWindow, []byte) {}
func bindNativeBrandWindow(*application.WebviewWindow)         {}
func releaseNativeBrandWindow(*application.WebviewWindow)      {}
