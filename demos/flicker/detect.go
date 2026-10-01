package flicker

import (
	"fmt"
	"math"
	"sort"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// detector turns the flicker sensor's readings into what the light is
// doing. Each measurement arrives as a run of a few equal readings; now and
// then one is off on its own (a 400 in steady light, a 50 in a dim room),
// so a state, or a frequency, counts only once it holds for most of the
// last second and a half, and a new one takes over only after it has held
// for a moment. Each reading counts for as long as it held, until the
// next: the sensor repeats itself about 30 times a second, but as an
// on-change sensor, a recording or a mock may not.
type detector struct {
	recent []sample // the last window's readings
	past   []sample // half a minute of what was settled, for the chart: 2 dark, 6 steady, else the frequency

	state    int     // what the light is doing, settled
	freq     float64 // the settled flicker frequency, Hz
	cand     int     // what the readings say now
	candF    float64
	candFrom float64 // since when they've said it
}

type sample struct{ t, v float64 }

const (
	stUnknown = iota
	stDark
	stSteady
	stFlicker

	window   = 1.5 // seconds of readings a state is judged on
	settle   = 0.8 // how long a new state must hold to take over
	pastSecs = 30
)

// The readings' meaning: 2 is dark, 4 to 8 is light that doesn't flicker
// (or not that the sensor can tell), anything from 15 up is a frequency.
func classify(v float64) int {
	switch {
	case math.IsNaN(v):
		return stUnknown
	case v <= 3:
		return stDark
	case v < 15:
		return stSteady
	}
	return stFlicker
}

func (d *detector) add(t, v float64) {
	d.recent = append(d.recent, sample{t, v})
}

// judge says what the last window's readings show, each weighed by how
// long it held: mostly a frequency (and one frequency, give or take),
// mostly dark, or else steady light.
func (d *detector) judge(t float64) (int, float64) {
	type held struct{ v, w float64 }
	var all, fs []held
	var total, dark, flick float64
	for i, s := range d.recent {
		from, to := math.Max(s.t, t-window), t
		if i+1 < len(d.recent) {
			to = d.recent[i+1].t
		}
		w := math.Max(0, to-from)
		all = append(all, held{s.v, w})
		total += w
		switch classify(s.v) {
		case stDark:
			dark += w
		case stFlicker:
			fs = append(fs, held{s.v, w})
			flick += w
		}
	}
	if total < 0.05 {
		return stUnknown, 0
	}
	// Hysteresis: a state is easier to stay in than to enter, so a light
	// on the edge doesn't flip the verdict back and forth.
	enterFlick, enterDark := 0.45, 0.6
	if d.state == stFlicker {
		enterFlick = 0.25
	}
	if d.state == stDark {
		enterDark = 0.4
	}
	if flick >= total*enterFlick { // most of the rest is the odd stray reading
		sort.Slice(fs, func(i, j int) bool { return fs[i].v < fs[j].v })
		med, acc := fs[0].v, 0.0
		for _, f := range fs { // the weighted median
			if acc += f.w; acc >= flick/2 {
				med = f.v
				break
			}
		}
		var near, sum float64
		for _, f := range fs {
			if math.Abs(f.v-med) <= med*0.08 {
				near += f.w
				sum += f.v * f.w
			}
		}
		if near >= flick*0.7 {
			return stFlicker, mains(sum / near)
		}
	}
	if dark >= total*enterDark {
		return stDark, 0
	}
	return stSteady, 0
}

// update settles the state from the readings so far, at time t.
func (d *detector) update(t float64) {
	// Drop what's past the window, but the reading in effect as it starts.
	i0 := 0
	for i0+1 < len(d.recent) && d.recent[i0+1].t <= t-window {
		i0++
	}
	d.recent = d.recent[i0:]
	i := 0
	for i+1 < len(d.past) && d.past[i+1].t < t-pastSecs {
		i++
	}
	d.past = d.past[i:]

	st, f := d.judge(t)
	same := st == d.cand && (st != stFlicker || math.Abs(f-d.candF) <= d.candF*0.08)
	if !same {
		d.cand, d.candF, d.candFrom = st, f, t
	} else if st == stFlicker {
		d.candF += (f - d.candF) * 0.2
	}
	// At the start, wait for enough readings to judge on.
	if len(d.recent) == 0 || t-d.recent[0].t < settle {
		return
	}
	if t-d.candFrom >= settle || d.state == stUnknown {
		d.state, d.freq = d.cand, d.candF
	}
	v := map[int]float64{stDark: 2, stSteady: 6, stFlicker: math.Round(d.freq)}[d.state]
	if d.state != stUnknown && (len(d.past) == 0 || d.past[len(d.past)-1].v != v) {
		d.past = append(d.past, sample{t, v})
	}
}

// screenRates are the refresh rates screens run at, video and games.
var screenRates = []float64{30, 60, 90, 144, 240}

// mains snaps a frequency near mains lighting's or a screen's to it: those
// are exact, and the sensor reads them a few hertz off (100 Hz as 102).
func mains(f float64) float64 {
	for _, m := range append([]float64{100, 120}, screenRates...) {
		if math.Abs(f-m) <= m*0.04 {
			return m
		}
	}
	return f
}

// flicker is the settled flicker frequency, or 0 for none.
func (d *detector) flicker() float64 {
	if d.state == stFlicker {
		return d.freq
	}
	return 0
}

// verdict says in words what the light is doing, and in what color.
func (d *detector) verdict() (string, uint8) {
	switch d.state {
	case stDark:
		return "dark: point the back of the phone at a light", 244
	case stSteady:
		return "steady light: no flicker", 120
	case stFlicker:
		f := d.freq
		what := "fast flicker: LEDs or a screen dimmed by pulsing (PWM)"
		screen := false
		for _, r := range screenRates {
			screen = screen || f == r
		}
		switch {
		case f == 100:
			what = "mains lighting, on a 50 Hz grid"
		case f == 120:
			what = "a 120 Hz screen, or mains lighting on a 60 Hz grid"
		case screen:
			what = "most likely a screen: video, or a game"
		case f < 80:
			what = "slow flicker: a failing lamp, or a dimmer"
		}
		return fmt.Sprintf("flickers at %.0f Hz: %s", f, what), 214
	}
	return "waiting for the flicker sensor...", 244
}

// chart draws the last half minute of what the detector settled on: each
// column a moment, flicker plotted by its frequency on a log scale, steady
// light and dark along the bottom.
func (d *detector) chart(v *View, t float64) {
	h := v.H
	if h < 3 || v.W < 20 {
		return
	}
	const lo, hi = 20.0, 1000.0
	plotH := h - 1 // the last row is the time axis
	left := 5
	w := v.W - left - 1
	row := func(f float64) int {
		u := (math.Log10(f) - math.Log10(lo)) / (math.Log10(hi) - math.Log10(lo))
		return plotH - 1 - int(Clamp01(u)*float64(plotH-1)+0.5)
	}
	for _, mark := range []struct {
		f    float64
		text string
	}{{1000, " 1k"}, {100, "100"}, {30, " 30"}} {
		y := row(mark.f)
		v.Text(0, y, mark.text, 240)
		for x := left; x < left+w; x += 2 {
			v.Set(x, y, '.', 236)
		}
	}
	v.Text(0, plotH, "-30s", 240)
	v.Text(left+w-3, plotH, "now", 240)
	// Each column shows the reading in effect at its moment.
	k := 0
	for x := left; x < left+w; x++ {
		at := t - pastSecs + (float64(x-left)+0.5)/float64(w)*pastSecs
		for k+1 < len(d.past) && d.past[k+1].t <= at {
			k++
		}
		if len(d.past) == 0 || d.past[k].t > at {
			continue
		}
		switch r := d.past[k].v; classify(r) {
		case stFlicker:
			v.Set(x, row(r), '#', 214)
		case stSteady:
			v.Set(x, plotH-1, '_', 71)
		case stDark:
			v.Set(x, plotH-1, '.', 238)
		}
	}
}
