//go:build darwin

package enterprisebuild

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/proc"
)

func TestEnterpriseICNSNativeWhiteLogoRoundTrip(t *testing.T) {
	_, profile, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "prepared")
	if _, err := Prepare(profile, "testdata/microsoft-white-transparent.png", out); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		name := "enterprise.icns"
		if theme == "dark" {
			name = "enterprise-dark.icns"
		}
		icons := filepath.Join(t.TempDir(), "decoded.iconset")
		b, err := proc.Command("/usr/bin/iconutil", "-c", "iconset", filepath.Join(out, name), "-o", icons).CombinedOutput()
		if err != nil {
			t.Fatalf("macOS could not decode enterprise ICNS: %v: %s", err, b)
		}
		files, err := filepath.Glob(filepath.Join(icons, "*.png"))
		if err != nil || len(files) < 6 {
			t.Fatalf("macOS lost icon representations: %v: %v", files, err)
		}
		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(f)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
			visible, clear := 0, 0
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if c.A == 0 {
						clear++
						continue
					}
					visible++
					if c.R != c.G || c.G != c.B {
						t.Fatalf("macOS decoded white ICNS as colored/corrupt pixels in %s at %d,%d: %v", filepath.Base(path), x, y, c)
					}
				}
			}
			if visible == 0 || clear == 0 {
				t.Fatalf("macOS lost the white mark or transparency in %s", filepath.Base(path))
			}
			ink := color.NRGBAModel.Convert(img.At(img.Bounds().Dx()/3, img.Bounds().Dy()/3)).(color.NRGBA)
			background := color.NRGBAModel.Convert(img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2)).(color.NRGBA)
			if theme == "light" && (ink.R > 80 || background.R < 220) || theme == "dark" && (ink.R < 200 || background.R > 80) {
				t.Fatalf("native application icon is not readable on its %s plate: %v %v", theme, ink, background)
			}
		}
	}
}
