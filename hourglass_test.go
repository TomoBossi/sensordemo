package main

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// The upper bulb empties in the set time, give or take a little: the neck
// is metered, and the sand must keep up with the meter.
func TestHourglassTiming(t *testing.T) {
	for _, c := range []struct {
		w, h int
		dur  time.Duration
	}{{60, 44, 20 * time.Second}, {150, 90, 60 * time.Second}} {
		size := [2]int{c.w, c.h}
		ss, _ := OpenMock("gravity=0,9.8,0")
		g := &hourglass{}
		g.Setup(ss)
		g.dur = c.dur
		var f Frame
		f.Resize(size[0], size[1])
		v := f.View(0, 0, size[0], size[1])
		empty := -1.0
		want := c.dur.Seconds()
		for i := 0; i < int(want*1.5*30) && empty < 0; i++ {
			tm := float64(i) / 30
			g.Draw(v, ss, tm, 1.0/30)
			if g.upper == 0 {
				empty = tm
			}
		}
		t.Logf("%dx%d: %d grains, empty after %.1f s of %.0f", size[0], size[1], g.total, empty, want)
		if empty < want*0.95 || empty > want*1.05 {
			t.Errorf("%dx%d: the upper bulb emptied after %.1f s, want about %.0f", size[0], size[1], empty, want)
		}
	}
}

func BenchmarkHourglass(b *testing.B) {
	for _, jitter := range []bool{false, true} {
		b.Run(map[bool]string{false: "flowing", true: "tilting"}[jitter], func(b *testing.B) {
			ss, _ := OpenMock("gravity=0,9.8,0")
			g := &hourglass{}
			g.Setup(ss)
			var f Frame
			f.Resize(150, 90)
			v := f.View(0, 0, 150, 90)
			for i := 0; i < 60; i++ {
				g.Draw(v, ss, float64(i)/30, 1.0/30)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if jitter { // every frame a slightly different tilt wakes all
					g.wakeU = [2]float64{1, 0}
				}
				g.Draw(v, ss, float64(i)/30, 1.0/30)
			}
		})
	}
}

// Tilting the phone slowly makes the sand slide gradually: the slopes of
// the pile give way from the first degrees, a little at a time. (A flat bed
// of sand holds until it tips past the angle sand rests at, about 35
// degrees, and then its top slides off together, as real sand does; the
// check stops short of that.)
func TestHourglassGradualTilt(t *testing.T) {
	ss := &Streams{byKey: map[string]*Stream{"gravity": {}}}
	g := &hourglass{rng: rand.New(rand.NewSource(1)), dur: 10 * time.Second, grav: [2]float64{0, 9.8}}
	var f Frame
	f.Resize(60, 44)
	v := f.View(0, 0, 60, 44)
	push := func(deg float64) {
		r := deg * math.Pi / 180
		ss.Get("gravity").push(clientEvent([]float64{-9.8 * math.Sin(r), 9.8 * math.Cos(r), 0}))
	}
	now := 0.0
	run := func(frames int) {
		for i := 0; i < frames; i++ {
			now += 1.0 / 30
			g.Draw(v, ss, now, 1.0/30)
		}
	}
	push(0)
	run(30 * 20) // everything down and settled
	pos := func() []int16 { return append(append([]int16{}, g.gx...), g.gy...) }
	var moved []int
	prev := pos()
	for deg := 1; deg <= 40; deg++ {
		push(float64(deg))
		run(20)
		cur, n := pos(), 0
		for i := range cur {
			if cur[i] != prev[i] {
				n++
			}
		}
		moved = append(moved, n)
		prev = cur
	}
	t.Logf("grains moving per degree: %v", moved)
	first, total, peak, steps := -1, 0, 0, 0
	for d, n := range moved[:30] {
		if n > 0 && first < 0 {
			first = d + 1
		}
		if n > 0 {
			steps++
		}
		total += n
		peak = max(peak, n)
	}
	if first < 0 || first > 3 {
		t.Errorf("the sand only started sliding at %d degrees", first)
	}
	if steps < 5 || peak > total/2 {
		t.Errorf("below 30 degrees it moved in %d steps, the biggest %d of %d: not gradual", steps, peak, total)
	}
}
