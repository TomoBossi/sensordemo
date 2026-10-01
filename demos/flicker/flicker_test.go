package flicker

import (
	"math"
	"strings"
	"testing"

	"github.com/TomoBossi/sensord/client"
	. "github.com/TomoBossi/sensordemo/internal/core"
)

// feed gives the detector readings at the sensor's pace (30 a second, in
// runs of four), from at for secs, with value v, except stray runs.
func feed(d *detector, at, secs float64, v func(i int) float64) float64 {
	t := at
	for i := 0; t < at+secs; i++ {
		d.add(t, v(i/4))
		d.update(t)
		t += 1.0 / 30
	}
	return t
}

// The detector settles on what the light does, ignoring the stray runs of
// odd readings the sensor makes, and takes up a change within 2 seconds.
func TestDetector(t *testing.T) {
	var d detector
	now := feed(&d, 0, 2, func(int) float64 { return 2 })
	if d.state != stDark {
		t.Fatalf("covered: state %d, want dark", d.state)
	}
	stray := func(base float64) func(int) float64 {
		return func(run int) float64 {
			if run%9 == 4 {
				return []float64{424, 50, 222}[run%3]
			}
			return base
		}
	}
	now = feed(&d, now, 3, stray(6))
	if d.state != stSteady {
		t.Fatalf("steady light with strays: state %d, want steady", d.state)
	}
	start := now
	for d.state != stFlicker && now < start+3 {
		now = feed(&d, now, 0.1, stray(102))
	}
	if d.flicker() != 100 || now-start > 2 {
		t.Fatalf("a 102 Hz lamp: %v Hz after %.1f s, want 100 within 2 s", d.flicker(), now-start)
	}
	now = feed(&d, now, 3, stray(390))
	if s, _ := d.verdict(); math.Abs(d.flicker()-390) > 5 || !strings.Contains(s, "PWM") {
		t.Fatalf("a dimmed LED: %v Hz, %q", d.flicker(), s)
	}
}

func run(t *testing.T, flk float64, w, h, frames int) (*flicker, []float64) {
	t.Helper()
	ss, err := OpenMock("rearals=3000;orient=0,0,0")
	if err != nil {
		t.Fatal(err)
	}
	s := &flicker{}
	if _, err := s.Setup(ss); err != nil {
		t.Fatal(err)
	}
	st := ss.Get("rearflk")
	var f Frame
	var glow []float64
	for i := 0; i < frames; i++ {
		tt := float64(i) / 30
		st.Push(clientEvent(tt, flk))
		f.Resize(w, h)
		s.Draw(f.View(0, 0, w, h), ss, tt, 1.0/30)
		glow = append(glow, s.glow)
	}
	return s, glow
}

// The bulb plays the light back eight times slower, and what the screen
// can't show at that, at its limit: bright and dim on alternate frames.
// Steady light burns steadily; dark, not at all.
func TestBulb(t *testing.T) {
	s, glow := run(t, 102, 97, 80, 90)
	if s.limit {
		t.Errorf("a 100 Hz lamp at 30 fps: at the limit, want 12.5 blinks a second")
	}
	if lo, hi := minmax(glow[60:]); hi-lo < 0.4 {
		t.Errorf("a 100 Hz lamp: glow only between %.2f and %.2f", lo, hi)
	}
	s, glow = run(t, 390, 97, 80, 90)
	if !s.limit {
		t.Errorf("a 390 Hz LED: not at the limit")
	}
	for i := 70; i < 87; i++ {
		if (glow[i] > glow[i+1]) == (glow[i+1] > glow[i+2]) {
			t.Fatalf("at the limit, it should alternate bright and dim: %v", glow[i:i+3])
		}
	}
	_, glow = run(t, 6, 97, 80, 60)
	if lo, hi := minmax(glow[45:]); hi-lo > 0.02 || lo < 0.5 {
		t.Fatalf("steady light: glow between %.2f and %.2f", lo, hi)
	}
	_, glow = run(t, 2, 97, 80, 60)
	if glow[len(glow)-1] != 0 {
		t.Errorf("dark: glow %v, want 0", glow[len(glow)-1])
	}
}

func minmax(v []float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return lo, hi
}

// The view stays within its limits however the phone turns: never more
// than 35 degrees off straight on (before the seat's few degrees up), and
// at rest, straight on from the default seat.
func TestBulbViewBounded(t *testing.T) {
	deg := 180 / math.Pi
	if v := bulbView(Vec3{0, 0, 1}); math.Abs(math.Asin(v[1])*deg-viewEl*deg) > 1e-6 || math.Abs(v[0]) > 1e-9 {
		t.Fatalf("rest view %v", v)
	}
	seat := Vec3{0, math.Sin(viewEl), math.Cos(viewEl)}
	for yaw := -180.0; yaw <= 180; yaw += 10 {
		for pitch := -89.0; pitch <= 89; pitch += 8 {
			y, p := yaw/deg, pitch/deg
			v := bulbView(Vec3{math.Sin(y) * math.Cos(p), math.Sin(p), math.Cos(y) * math.Cos(p)})
			if off := math.Acos(math.Min(1, v.Dot(seat))) * deg; off > 35.01 || math.Abs(v.Len()-1) > 1e-9 {
				t.Fatalf("turned %v, %v: %.1f degrees off the seat", yaw, pitch, off)
			}
		}
	}
}

// Any size draws, small ones too.
func TestSizes(t *testing.T) {
	for _, sz := range [][2]int{{10, 5}, {20, 12}, {60, 30}, {97, 50}, {97, 94}, {200, 50}} {
		run(t, 102, sz[0], sz[1], 20)
	}
}

func clientEvent(t, v float64) client.Event {
	return client.Event{T: int64(t * 1e9), V: []float64{v}}
}

// A light on the edge, its flicker readings coming and going around the
// threshold, doesn't flip the verdict back and forth.
func TestDetectorSteadyVerdict(t *testing.T) {
	var d detector
	now := feed(&d, 0, 3, func(int) float64 { return 102 })
	changes := 0
	last := d.state
	for k := 0; k < 300; k++ { // 10 s: two runs in five flicker, the rest steady
		v := 6.0
		if k/4%5 < 2 {
			v = 102
		}
		d.add(now, v)
		d.update(now)
		now += 1.0 / 30
		if d.state != last {
			changes++
			last = d.state
		}
	}
	if changes > 1 {
		t.Fatalf("the verdict changed %d times in 10 s", changes)
	}
}

// What the verdict calls a light, by its frequency: mains lighting at 100,
// a screen at its refresh rates, either at 120, PWM otherwise.
func TestVerdict(t *testing.T) {
	for f, want := range map[float64]string{
		102: "mains lighting, on a 50 Hz grid",
		61:  "screen",
		119: "120 Hz screen, or mains",
		145: "screen",
		390: "PWM",
		40:  "slow flicker",
	} {
		var d detector
		feed(&d, 0, 3, func(int) float64 { return f })
		if s, _ := d.verdict(); !strings.Contains(s, want) {
			t.Errorf("%v Hz: %q, want %q", f, s, want)
		}
	}
}
