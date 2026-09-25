package main

import (
	"os"

	"golang.org/x/term"
)

// Terminal owns the tty for the life of a demo: raw keyboard input, the
// alternate screen, and a hidden cursor. Restore undoes all of it.
type Terminal struct {
	in    int
	out   *os.File
	saved *term.State
	Keys  chan byte
}

func OpenTerminal() (*Terminal, error) {
	t := &Terminal{in: int(os.Stdin.Fd()), out: os.Stdout, Keys: make(chan byte, 16)}
	st, err := term.MakeRaw(t.in)
	if err != nil {
		return nil, err
	}
	t.saved = st
	t.out.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	go t.readKeys()
	return t, nil
}

func (t *Terminal) readKeys() {
	buf := make([]byte, 16)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		for _, b := range buf[:n] {
			select {
			case t.Keys <- b:
			default:
			}
		}
	}
}

// Size is the current size in character cells. It is re-read every frame, so
// resizing the window (or pinch-zooming Termux) takes effect immediately.
func (t *Terminal) Size() (w, h int) {
	w, h, err := term.GetSize(int(t.out.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func (t *Terminal) Restore() {
	t.out.WriteString("\x1b[0m\x1b[?25h\x1b[?1049l")
	if t.saved != nil {
		term.Restore(t.in, t.saved)
	}
}
