package main

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

// TestFluidSettles runs the fluid under fixed gravity and prints frames
// (FLUID_PRINT=1 go test -run Fluid -v).
func TestFluidSettles(t *testing.T) {
	f := &fluid{fill: 0.35, rng: rand.New(rand.NewSource(1))}
	w, h := 60, 24
	f.layout(w, h)
	n := len(f.px)
	start := time.Now()
	for frame := 0; frame < 300; frame++ {
		for s := 0; s < fluidSteps; s++ {
			f.step(0, 9.8*fluidGravity)
		}
	}
	per := time.Since(start) / 300
	for i := range f.px {
		if f.px[i] != f.px[i] || f.py[i] != f.py[i] {
			t.Fatal("NaN position")
		}
	}
	t.Logf("%d particles, %v per frame", n, per)
	if os.Getenv("FLUID_PRINT") != "" {
		var fr Frame
		fr.Resize(w, h)
		ss := &Streams{byKey: map[string]*Stream{"accelerometer": {}}}
		f.Draw(fr.View(0, 0, w, h), ss, 0, 0)
		for y := 0; y < h; y++ {
			fmt.Println("|" + string(fr.chars[y*w:(y+1)*w]) + "|")
		}
	}
}
