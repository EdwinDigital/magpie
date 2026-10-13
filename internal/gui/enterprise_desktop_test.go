//go:build !nogui

package gui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/settings"
)

func TestEnterpriseTrayLogoKeepsAspectAndTemplateContrast(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	img := image.NewNRGBA(image.Rect(0, 0, 40, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 40; x++ {
			c := color.NRGBA{A: 255}
			if x >= 20 {
				c.R, c.G, c.B = 255, 255, 255
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	e := settings.EnterpriseDefaults()
	e.LogoAsset = distribution.AssetID(data.Bytes())
	e.OverrideFields = []string{"logoAsset"}
	dir := filepath.Join(settings.Dir(), "branding")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(e.LogoAsset, "sha256:")+".png"), data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	logo, err := png.Decode(bytes.NewReader(brandTrayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	alpha := func(x, y int) uint32 { _, _, _, a := logo.At(x, y).RGBA(); return a }
	if logo.Bounds().Dx() != 44 || alpha(10, 0) != 0 || alpha(10, 22) == 0 || alpha(34, 22) != 0 {
		t.Fatal("tray logo was stretched or its white background became a solid template")
	}
}

func TestEnterpriseWhiteTrayLogoUsesRetinaTemplateAlpha(t *testing.T) {
	b, err := os.ReadFile("../enterprisebuild/testdata/microsoft-white-transparent.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(brandTrayIconFor(b)))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 44, 44) {
		t.Fatal("enterprise template lost the original bird's Retina canvas size")
	}
	for _, at := range []image.Point{{12, 12}, {32, 12}, {12, 32}, {32, 32}} {
		c := color.NRGBAModel.Convert(img.At(at.X, at.Y)).(color.NRGBA)
		if c != (color.NRGBA{A: 255}) {
			t.Fatalf("white logo did not become opaque template ink at %v: %v", at, c)
		}
	}
	for _, at := range []image.Point{{0, 0}, {22, 22}, {22, 12}, {12, 22}} {
		_, _, _, a := img.At(at.X, at.Y).RGBA()
		if a != 0 {
			t.Fatalf("template margin or central gap became opaque at %v", at)
		}
	}
}

func TestEnterpriseIconThemeFollowsPreferencesAndSystem(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MAGPIE_THEME", "")
	old := enterpriseSystemDark
	t.Cleanup(func() { enterpriseSystemDark = old })
	for _, tc := range []struct {
		setting string
		system  bool
		want    string
	}{{"light", true, "light"}, {"dark", false, "dark"}, {"system", true, "dark"}, {"system", false, "light"}} {
		s := settings.Load()
		s.Theme = tc.setting
		if err := settings.Save(s); err != nil {
			t.Fatal(err)
		}
		enterpriseSystemDark = func() bool { return tc.system }
		if got := enterpriseIconTheme(); got != tc.want {
			t.Fatalf("setting=%s system=%v: %s", tc.setting, tc.system, got)
		}
	}
}
