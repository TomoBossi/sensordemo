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

	// What the terminal shows now, to send only what changed.
	sentC []byte
	sentF []uint8
	sentW int
	sentH int
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

// Flush brings the terminal up to date with the frame, in one write: only
// the runs of characters that changed since the last Flush (the whole
// screen the first time, and after a resize), with color escapes only
// where the color changes. Nothing changed, nothing is written. Terminals
// draw slowly, so this is where small fonts get their frame rate back.
func (f *Frame) Flush(w interface{ Write([]byte) (int, error) }, pal *palette) {
	b := &f.buf
	b.Reset()
	n := f.W * f.H
	full := f.sentW != f.W || f.sentH != f.H || len(f.sentC) != n
	if full {
		f.sentC, f.sentF = make([]byte, n), make([]uint8, n)
		f.sentW, f.sentH = f.W, f.H
	}
	mapped := func(i int) uint8 {
		if pal == nil {
			return 0
		}
		return pal.Map(f.fg[i])
	}
	same := func(i int) bool {
		c := f.chars[i]
		return !full && c == f.sentC[i] && (c == ' ' || mapped(i) == f.sentF[i])
	}
	cur := uint8(0)
	put := func(i int) {
		c := f.chars[i]
		if fg := mapped(i); fg != cur && c != ' ' {
			cur = fg
			if fg == 0 {
				b.WriteString("\x1b[39m")
			} else {
				b.WriteString("\x1b[38;5;")
				b.WriteString(strconv.Itoa(int(fg)))
				b.WriteByte('m')
			}
		}
		b.WriteByte(c)
		f.sentC[i], f.sentF[i] = c, mapped(i)
	}
	const bridge = 5 // rewriting a short unchanged gap is cheaper than a jump
	for y := 0; y < f.H; y++ {
		row := y * f.W
		for x := 0; x < f.W; {
			if same(row + x) {
				x++
				continue
			}
			// A run of changes, bridging short gaps.
			end := x
			for e := x + 1; e < f.W && e-end <= bridge; e++ {
				if !same(row + e) {
					end = e
				}
			}
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(y + 1))
			b.WriteByte(';')
			b.WriteString(strconv.Itoa(x + 1))
			b.WriteByte('H')
			for i := x; i <= end; i++ {
				put(row + i)
			}
			x = end + 1
		}
	}
	if cur != 0 {
		b.WriteString("\x1b[39m")
	}
	if b.Len() > 0 {
		w.Write(b.Bytes())
	}
}
