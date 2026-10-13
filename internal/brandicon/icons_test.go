package brandicon

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func decoded(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestWhiteArtworkHasContrastingThemeAndAppVariants(t *testing.T) {
	b, err := os.ReadFile("../enterprisebuild/testdata/microsoft-white-transparent.png")
	if err != nil {
		t.Fatal(err)
	}
	icons, err := Generate(b)
	if err != nil {
		t.Fatal(err)
	}
	light, dark := decoded(t, icons.Light), decoded(t, icons.Dark)
	for _, p := range []image.Point{{300, 300}, {724, 300}, {300, 724}, {724, 724}} {
		l := color.NRGBAModel.Convert(light.At(p.X, p.Y)).(color.NRGBA)
		d := color.NRGBAModel.Convert(dark.At(p.X, p.Y)).(color.NRGBA)
		if l.R > 30 || d.R < 240 || l.A != 255 || d.A != 255 {
			t.Fatalf("white mark has insufficient theme contrast at %v: %v %v", p, l, d)
		}
	}
	for _, p := range []image.Point{{0, 0}, {512, 512}, {512, 300}} {
		_, _, _, a := light.At(p.X, p.Y).RGBA()
		if a != 0 {
			t.Fatalf("glyph lost transparency at %v", p)
		}
	}
	for i, b := range [][]byte{icons.AppLight, icons.AppDark} {
		app := decoded(t, b)
		_, _, _, edge := app.At(0, 0).RGBA()
		if edge != 0 {
			t.Fatal("application tile lost outer margin")
		}
		ink := color.NRGBAModel.Convert(app.At(340, 340)).(color.NRGBA)
		bg := color.NRGBAModel.Convert(app.At(512, 512)).(color.NRGBA)
		if i == 0 && (ink.R > 30 || bg.R < 230) || i == 1 && (ink.R < 240 || bg.R > 60) {
			t.Fatalf("application theme contrast: %v %v", ink, bg)
		}
	}
	for _, b := range [][]byte{icons.Source, icons.Light, icons.Dark, icons.AppLight, icons.AppDark} {
		img := decoded(t, b)
		if img.Bounds() != image.Rect(0, 0, 1024, 1024) {
			t.Fatal("wrong standard size")
		}
	}
	second, err := Generate(icons.Source)
	if err != nil || !bytes.Equal(second.Source, icons.Source) {
		t.Fatal("normalization changes the frozen source when prepared again", err)
	}
}

func TestJPEGUniformBackdropAndNonSquareGeometry(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 40))
	for y := range 40 {
		for x := range 100 {
			c := color.NRGBA{255, 255, 255, 255}
			if x >= 10 && x < 90 && y >= 15 && y < 25 {
				c = color.NRGBA{20, 20, 20, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	icons, err := Generate(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	light := decoded(t, icons.Light)
	_, _, _, a := light.At(512, 300).RGBA()
	if a != 0 {
		t.Fatal("JPEG white backdrop became a rectangular logo")
	}
	_, _, _, a = light.At(512, 512).RGBA()
	if a == 0 {
		t.Fatal("JPEG logo disappeared")
	}
	_, _, _, a = light.At(102, 512).RGBA()
	if a == 0 {
		t.Fatal("non-square image did not keep its aspect ratio and 80% fit")
	}
	var empty bytes.Buffer
	_ = png.Encode(&empty, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	if _, err := Generate(empty.Bytes()); err == nil {
		t.Fatal("accepted empty transparent artwork")
	}
	if _, err := Generate([]byte("<svg/>")); err == nil {
		t.Fatal("accepted executable/vector input")
	}
}
