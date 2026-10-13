//go:build darwin && cgo && !nogui

package gui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEnterpriseWhiteMenuBarNativeAppearance(t *testing.T) {
	logo, err := os.ReadFile("../enterprisebuild/testdata/microsoft-white-transparent.png")
	if err != nil {
		t.Fatal(err)
	}
	template := brandTrayIconFor(logo)
	for _, dark := range []bool{false, true} {
		for _, cells := range [][]trayCell{nil, {{Letter: "C", Rows: []string{"8%"}}}} {
			b, w, h := trayImagePNG(cells, template, 22, 2, dark, false)
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if h != 44 || w < 44 || inkIn(img, image.Rect(0, 0, 44, 44), dark) < 200 {
				t.Fatalf("menu bar lost enterprise template ink: dark=%v cells=%d size=%dx%d", dark, len(cells), w, h)
			}
			if out := os.Getenv("MAGPIE_TRAY_PREVIEW"); out != "" {
				b, _, _ = trayImagePNG(cells, template, 22, 2, dark, true)
				name := fmt.Sprintf("enterprise-tray-%s-%d.png", map[bool]string{true: "dark", false: "light"}[dark], len(cells))
				if err := os.WriteFile(filepath.Join(out, name), b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
