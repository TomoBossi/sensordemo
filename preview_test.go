package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
)

// PREVIEW=demo:WxH:frames:mock:out.png renders a demo's frame as an image,
// in its terminal colors with rough glyph shapes, for judging looks
// without a phone.
func TestPreview(t *testing.T) {
	spec := os.Getenv("PREVIEW")
	if spec == "" {
		t.Skip("set PREVIEW=demo:WxH:frames:mock:out.png")
	}
	p := strings.SplitN(spec, ":", 5)
	var w, h int
	fmtSscanf(p[1], &w, &h)
	frames, _ := strconv.Atoi(p[2])
	ss, err := OpenMock(p[3])
	if err != nil {
		t.Fatal(err)
	}
	e := find(p[0])
	if i := strings.IndexByte(p[0], ' '); i > 0 {
		e = find(p[0][:i])
		demoArg = p[0][i+1:]
	}
	d := e.new(nil)
	if _, err := d.Setup(ss); err != nil {
		t.Fatal(err)
	}
	var f Frame
	f.Resize(w, h)
	for i := 0; i < frames; i++ {
		d.Draw(f.View(0, 0, w, h), ss, float64(i)/30, 1.0/30)
	}
	const cw, ch = 8, 16
	img := image.NewRGBA(image.Rect(0, 0, w*cw, h*ch))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c, fg := f.chars[y*w+x], f.fg[y*w+x]
			col := xterm(fg)
			for py := 0; py < ch; py++ {
				for px := 0; px < cw; px++ {
					if a := glyph(c, float64(px)/cw, float64(py)/ch); a > 0 {
						img.Set(x*cw+px, y*ch+py, color.RGBA{uint8(float64(col.R) * a), uint8(float64(col.G) * a), uint8(float64(col.B) * a), 255})
					} else {
						img.Set(x*cw+px, y*ch+py, color.RGBA{0, 0, 0, 255})
					}
				}
			}
		}
	}
	out, _ := os.Create(p[4])
	defer out.Close()
	png.Encode(out, img)
}

func fmtSscanf(s string, w, h *int) {
	parts := strings.Split(s, "x")
	*w, _ = strconv.Atoi(parts[0])
	*h, _ = strconv.Atoi(parts[1])
}

func xterm(c uint8) color.RGBA {
	if c == 0 {
		return color.RGBA{200, 200, 200, 255}
	}
	if c < 16 {
		b := [16][3]uint8{{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0}, {0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
			{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255}}[c]
		return color.RGBA{b[0], b[1], b[2], 255}
	}
	if c >= 232 {
		v := uint8(8 + 10*int(c-232))
		return color.RGBA{v, v, v, 255}
	}
	i := int(c) - 16
	return color.RGBA{uint8(cubeLevels[i/36]), uint8(cubeLevels[i/6%6]), uint8(cubeLevels[i%6]), 255}
}

// glyph is a rough coverage of character c at (u, v) in its cell.
func glyph(c byte, u, v float64) float64 {
	near := func(d float64) float64 {
		if d < 0 {
			d = -d
		}
		if d < 0.09 {
			return 1
		}
		return 0
	}
	switch c {
	case ' ':
		return 0
	case '|', '!':
		return near(u - 0.5)
	case '/':
		return near((u - 0.5) + (v-0.5)*0.5)
	case '\\':
		return near((u - 0.5) - (v-0.5)*0.5)
	case '-':
		return near(v - 0.5)
	case '_':
		return near(v - 0.9)
	case '=':
		return max(near(v-0.4), near(v-0.62))
	case '~':
		return near(v - 0.5 - 0.08*sinApprox(u*6.3))
	case '.', ',':
		if near(u-0.5) > 0 && v > 0.78 && v < 0.9 {
			return 1
		}
		return 0
	case '\'', '`':
		if near(u-0.5) > 0 && v > 0.15 && v < 0.35 {
			return 1
		}
		return 0
	case ':':
		if near(u-0.5) > 0 && (v > 0.3 && v < 0.4 || v > 0.7 && v < 0.8) {
			return 1
		}
		return 0
	case '"':
		if (near(u-0.35) > 0 || near(u-0.65) > 0) && v > 0.15 && v < 0.35 {
			return 1
		}
		return 0
	}
	// Anything else: a patch as dense as the character.
	dens := map[byte]float64{'+': 0.4, '*': 0.5, 'o': 0.45, 'O': 0.6, '#': 0.75, '%': 0.8, '@': 0.95, '(': 0.35, ')': 0.35, '[': 0.4, ']': 0.4, '^': 0.3, 'A': 0.6, '8': 0.7}[c]
	if dens == 0 {
		dens = 0.55
	}
	if u > 0.12 && u < 0.88 && v > 0.12 && v < 0.88 {
		return dens
	}
	return 0
}

func sinApprox(x float64) float64 {
	for x > 3.14159 {
		x -= 6.28318
	}
	return x * (1 - x*x/6)
}
