//go:build !nogui

package gui

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/update"
)

func brandTrayIcon() []byte {
	b, _ := enterpriseLogo(settings.ResolveEnterprise(settings.Load()).LogoAsset)
	return brandTrayIconFor(b)
}

func brandTrayIconFor(b []byte) []byte {
	if len(b) == 0 {
		return trayIcon
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return trayIcon
	}
	// Like tray.png, use 2x pixels for the menu bar's 22-point template.
	const size = 44
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := img.Bounds()
	width, height := size*bounds.Dx()/max(bounds.Dx(), bounds.Dy()), size*bounds.Dy()/max(bounds.Dx(), bounds.Dy())
	width, height = max(width, 1), max(height, 1)
	ox, oy := (size-width)/2, (size-height)/2
	useLuminance := false
	if opaque, ok := img.(interface{ Opaque() bool }); ok {
		useLuminance = opaque.Opaque()
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(img.At(bounds.Min.X+x*bounds.Dx()/width, bounds.Min.Y+y*bounds.Dy()/height)).(color.NRGBA)
			// Template imagery uses opacity, not RGB; white JPEG backgrounds
			// should not become a solid square in the menu bar.
			alpha := c.A
			if useLuminance {
				lum := (uint32(c.R)*299 + uint32(c.G)*587 + uint32(c.B)*114) / 1000
				alpha = uint8(uint32(c.A) * (255 - lum) / 255)
			}
			out.SetNRGBA(ox+x, oy+y, color.NRGBA{A: alpha})
		}
	}
	var buf bytes.Buffer
	if png.Encode(&buf, out) != nil {
		return trayIcon
	}
	return buf.Bytes()
}

func (h *host) enterpriseChanged() {
	application.InvokeSync(func() {
		s := enterpriseState()
		b, _ := json.Marshal(s)
		for _, w := range []*application.WebviewWindow{h.main, h.panel} {
			if w != nil {
				w.SetTitle(s.Effective.AppName)
				w.ExecJS("window.receiveEnterprise?.(" + string(b) + ");")
				nativeBrandWindowIcon(w, brandAppIcon())
			}
		}
		logo := brandAppIcon()
		if len(logo) == 0 {
			logo = appIconLarge
		}
		h.app.SetIcon(logo)
		if h.tray != nil {
			h.tray.SetTooltip(s.Effective.AppName)
			if runtime.GOOS == "darwin" {
				h.tray.SetTemplateIcon(brandTrayIcon())
			} else {
				if len(brandLogo()) == 0 {
					logo = appIcon
				}
				h.tray.SetIcon(logo)
			}
		}
	})
	if onLang != nil {
		onLang()
	}
	if onTrayUsage != nil {
		onTrayUsage()
	}
}

func (h *host) restartConfiguredApplication() error {
	configurationRestart.Store(true)
	var err error
	if b := update.Bundle(); b != "" {
		err = update.Relaunch(b)
	} else {
		var exe string
		exe, err = update.Executable()
		if err == nil {
			err = relaunchBinary(exe, "", h.MainShown(), "settings")
		}
	}
	if err != nil {
		configurationRestart.Store(false)
		return err
	}
	time.AfterFunc(250*time.Millisecond, h.Quit)
	return nil
}
