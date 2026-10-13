// Package brandicon applies the same icon standard to uploads and release assets.
package brandicon

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
)

const Size = 1024
const Version = "1"

type Icons struct {
	Source, Light, Dark, AppLight, AppDark []byte
}

func Generate(b []byte) (*Icons, error) {
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("logo must be at most 2 MiB")
	}
	c, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || (format != "png" && format != "jpeg") || c.Width < 1 || c.Height < 1 || c.Width > 4096 || c.Height > 4096 || int64(c.Width)*int64(c.Height) > 4<<20 {
		return nil, fmt.Errorf("logo must be a PNG or JPEG with at most 4 million pixels")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	src := image.NewNRGBA(image.Rect(0, 0, c.Width, c.Height))
	opaque := true
	for y := range c.Height {
		for x := range c.Width {
			v := color.NRGBAModel.Convert(img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y)).(color.NRGBA)
			opaque = opaque && v.A == 255
			src.SetNRGBA(x, y, v)
		}
	}
	if opaque {
		removeUniformBackground(src)
	}
	box := image.Rectangle{}
	for y := range c.Height {
		for x := range c.Width {
			if src.NRGBAAt(x, y).A != 0 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if box.Empty() {
		return nil, fmt.Errorf("logo has no visible pixels")
	}
	const side = 820 // 80% foreground, like the original bird glyph.
	w, h := side, side
	if box.Dx() > box.Dy() {
		h = max(1, side*box.Dy()/box.Dx())
	} else {
		w = max(1, side*box.Dx()/box.Dy())
	}
	normalized := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	fit(normalized, image.Rect((Size-w)/2, (Size-h)/2, (Size-w)/2+w, (Size-h)/2+h), src, box)
	light, dark := tint(normalized, 0x16), tint(normalized, 0xf4)
	images := []*image.NRGBA{normalized, light, dark, tile(light, false), tile(dark, true)}
	encoded := make([][]byte, len(images))
	for i, img := range images {
		var out bytes.Buffer
		if err := png.Encode(&out, img); err != nil {
			return nil, err
		}
		encoded[i] = out.Bytes()
	}
	if len(encoded[0]) > 2<<20 {
		return nil, fmt.Errorf("logo must be at most 2 MiB")
	}
	return &Icons{encoded[0], encoded[1], encoded[2], encoded[3], encoded[4]}, nil
}

func distance(a, b color.NRGBA) int {
	return max(abs(int(a.R)-int(b.R)), abs(int(a.G)-int(b.G)), abs(int(a.B)-int(b.B)))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Only a consistent corner color is treated as a backdrop. This is not
// photographic subject segmentation; transparent artwork is preferable.
func removeUniformBackground(img *image.NRGBA) {
	b := img.Bounds()
	bg := img.NRGBAAt(0, 0)
	for _, p := range []image.Point{{b.Max.X - 1, 0}, {0, b.Max.Y - 1}, {b.Max.X - 1, b.Max.Y - 1}} {
		if distance(bg, img.NRGBAAt(p.X, p.Y)) > 12 {
			return
		}
	}
	contrast := false
	for y := range b.Dy() {
		for x := range b.Dx() {
			contrast = contrast || distance(bg, img.NRGBAAt(x, y)) > 40
		}
	}
	if !contrast {
		return
	} // A solid-color mark is still valid artwork.
	for y := range b.Dy() {
		for x := range b.Dx() {
			c := img.NRGBAAt(x, y)
			d := distance(bg, c)
			if d < 32 {
				c.A = uint8(max(0, d-8) * 255 / 24)
				if c.A == 0 {
					c.R, c.G, c.B = 0, 0, 0
				}
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// Resample premultiplied channels so transparent edges cannot create halos.
func fit(dst *image.NRGBA, at image.Rectangle, src image.Image, box image.Rectangle) {
	for y := range at.Dy() {
		for x := range at.Dx() {
			if at.Size() == box.Size() {
				dst.Set(at.Min.X+x, at.Min.Y+y, src.At(box.Min.X+x, box.Min.Y+y))
				continue
			}
			fx := max(0, min(float64(box.Dx()-1), (float64(x)+0.5)*float64(box.Dx())/float64(at.Dx())-0.5))
			fy := max(0, min(float64(box.Dy()-1), (float64(y)+0.5)*float64(box.Dy())/float64(at.Dy())-0.5))
			x0, y0 := int(fx), int(fy)
			var channels [4]float64
			for iy := range 2 {
				for ix := range 2 {
					weight := []float64{1 - (fx - float64(x0)), fx - float64(x0)}[ix] * []float64{1 - (fy - float64(y0)), fy - float64(y0)}[iy]
					r, g, b, a := src.At(box.Min.X+min(x0+ix, box.Dx()-1), box.Min.Y+min(y0+iy, box.Dy()-1)).RGBA()
					for j, v := range []uint32{r, g, b, a} {
						channels[j] += float64(v) * weight
					}
				}
			}
			c := color.NRGBA{}
			if channels[3] > 0 {
				c = color.NRGBA{uint8(math.Round(channels[0] / channels[3] * 255)), uint8(math.Round(channels[1] / channels[3] * 255)), uint8(math.Round(channels[2] / channels[3] * 255)), uint8(math.Round(channels[3] / 257))}
			}
			dst.SetNRGBA(at.Min.X+x, at.Min.Y+y, c)
		}
	}
}

func tint(src *image.NRGBA, ink uint8) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for y := range Size {
		for x := range Size {
			out.SetNRGBA(x, y, color.NRGBA{ink, ink, ink, src.NRGBAAt(x, y).A})
		}
	}
	return out
}

func tile(glyph *image.NRGBA, dark bool) *image.NRGBA {
	const inset, side = 102, 820
	const radius = float64(side) * 0.225
	foreground := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	fit(foreground, image.Rect(inset, inset, inset+side, inset+side), glyph, glyph.Bounds())
	out := image.NewNRGBA(glyph.Bounds())
	lo, hi := 255.0, 236.0
	if dark {
		lo, hi = 58, 34
	}
	for y := range Size {
		for x := range Size {
			coverage := 0
			for sy := range 4 {
				for sx := range 4 {
					px, py := float64(x)+(float64(sx)+0.5)/4, float64(y)+(float64(sy)+0.5)/4
					if px < inset || py < inset || px > inset+side || py > inset+side {
						continue
					}
					cx := max(inset+radius, min(px, inset+side-radius))
					cy := max(inset+radius, min(py, inset+side-radius))
					if math.Hypot(px-cx, py-cy) <= radius {
						coverage++
					}
				}
			}
			if coverage == 0 {
				continue
			}
			c := foreground.NRGBAAt(x, y)
			bg := lo + (hi-lo)*(float64(y)+0.5)/Size
			value := uint8(math.Round(bg*(1-float64(c.A)/255) + float64(c.R)*float64(c.A)/255))
			out.SetNRGBA(x, y, color.NRGBA{value, value, value, uint8(coverage * 255 / 16)})
		}
	}
	return out
}
