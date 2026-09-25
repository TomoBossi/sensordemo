package main

import (
	"bytes"
	"strconv"
)

// Frame is one screen of ASCII characters, each with an optional 256-color
// foreground (0 = terminal default). Demos draw into a View of it.
type Frame struct {
	W, H  int
	chars []byte
	fg    []uint8
	buf   bytes.Buffer
}

func (f *Frame) Resize(w, h int) {
	if w != f.W || h != f.H {
		f.W, f.H = w, h
		f.chars = make([]byte, w*h)
		f.fg = make([]uint8, w*h)
	}
	for i := range f.chars {
		f.chars[i] = ' '
		f.fg[i] = 0
	}
}

// View is a rectangle of a frame with its own coordinates.
type View struct {
	f          *Frame
	X, Y, W, H int
}

func (f *Frame) View(x, y, w, h int) *View {
	return &View{f: f, X: x, Y: y, W: max(w, 0), H: max(h, 0)}
}

func (v *View) Set(x, y int, c byte, fg uint8) {
	if x < 0 || y < 0 || x >= v.W || y >= v.H {
		return
	}
	i := (v.Y+y)*v.f.W + v.X + x
	v.f.chars[i] = c
	v.f.fg[i] = fg
}

func (v *View) Get(x, y int) byte {
	if x < 0 || y < 0 || x >= v.W || y >= v.H {
		return 0
	}
	return v.f.chars[(v.Y+y)*v.f.W+v.X+x]
}

// Text writes s from (x, y), clipped to the view.
func (v *View) Text(x, y int, s string, fg uint8) {
	for i := 0; i < len(s); i++ {
		v.Set(x+i, y, s[i], fg)
	}
}

// Flush draws the whole frame in one write. Color escapes are emitted only
// where the color changes, and not at all when color is off.
func (f *Frame) Flush(w interface{ Write([]byte) (int, error) }, color bool) {
	b := &f.buf
	b.Reset()
	b.WriteString("\x1b[H")
	cur := uint8(0)
	for y := 0; y < f.H; y++ {
		if y > 0 {
			b.WriteString("\r\n")
		}
		row := f.chars[y*f.W : (y+1)*f.W]
		for x, c := range row {
			if color {
				if fg := f.fg[y*f.W+x]; fg != cur && c != ' ' {
					cur = fg
					if fg == 0 {
						b.WriteString("\x1b[39m")
					} else {
						b.WriteString("\x1b[38;5;")
						b.WriteString(strconv.Itoa(int(fg)))
						b.WriteByte('m')
					}
				}
			}
			b.WriteByte(c)
		}
	}
	if color && cur != 0 {
		b.WriteString("\x1b[39m")
	}
	w.Write(b.Bytes())
}
