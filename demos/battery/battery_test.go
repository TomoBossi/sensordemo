package battery

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// However the phone turns, the view stays within its limits: at most 60
// degrees around the cell, never from below the floor; the reference pose
// looks from the default angle.
func TestBatteryViewBounded(t *testing.T) {
	deg := 180 / math.Pi
	if v := batteryView(Vec3{0, 0, 1}); math.Abs(math.Asin(v[1])*deg-16) > 0.5 || math.Abs(v[0]) > 1e-9 {
		t.Fatalf("rest view %v, want 16 degrees up, straight on", v)
	}
	for yaw := -180.0; yaw <= 180; yaw += 10 {
		for pitch := -89.0; pitch <= 89; pitch += 8 {
			y, p := yaw/deg, pitch/deg
			v := batteryView(Vec3{math.Sin(y) * math.Cos(p), math.Sin(p), math.Cos(y) * math.Cos(p)})
			if around, up := math.Atan2(v[0], v[2])*deg, math.Asin(v[1])*deg; math.Abs(around) > 60 || up < 1 || up > 55 {
				t.Fatalf("turned %v, %v: view %.1f around, %.1f up", yaw, pitch, around, up)
			}
		}
	}
}

func run(t *testing.T, mock string, w, h, frames int) (*battery, *Frame) {
	t.Helper()
	ss, err := OpenMock(mock + ";orient=0,0,0;linear_acceleration=0,0,0")
	if err != nil {
		t.Fatal(err)
	}
	b := &battery{}
	if _, err := b.Setup(ss); err != nil {
		t.Fatal(err)
	}
	// Mock readings arrive on their own clock: wait for the first.
	for end := time.Now().Add(2 * time.Second); !ss.Get("battery").Read().OK && time.Now().Before(end); {
		time.Sleep(5 * time.Millisecond)
	}
	var f Frame
	for i := 0; i < frames; i++ {
		f.Resize(w, h)
		b.Draw(f.View(0, 0, w, h), ss, float64(i)/30, 1.0/30)
	}
	return b, &f
}

func text(f *Frame, w, h int) string {
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if c, _ := f.Cell(x, y); c != 0 {
				sb.WriteByte(c)
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// The words follow the state: in use with the time left, charging with the
// time to full, and whatever the phone doesn't report (NaN) left out, not
// printed as NaN.
func TestBatteryWords(t *testing.T) {
	for _, c := range []struct{ mock, want, not string }{
		{"battery=52,3,0,2,28,3.85,-400,-400,3180,7,NaN,0", "left at this rate", "NaN"},
		{"battery=40,2,2,2,33,4.1,1500,1500,2400,7,3600,0", "full in 1h 00m", "left at"},
		{"battery=100,5,1,2,30,4.3,0,0,6100,7,NaN,1", "battery saver", "NaN"},
		{"battery=52,3,0,2,NaN,NaN,NaN,NaN,NaN,NaN,NaN,0", "in use", "NaN"},
	} {
		_, f := run(t, c.mock, 90, 50, 40)
		s := text(f, 90, 50)
		if !strings.Contains(s, c.want) || strings.Contains(s, c.not) {
			t.Errorf("%s: want %q and no %q in:\n%s", c.mock, c.want, c.not, s[strings.LastIndex(s[:len(s)-200], "\n"):])
		}
	}
}

// Small and odd sizes draw without trouble, and the charge rises to its
// level, whatever the view.
func TestBatterySizes(t *testing.T) {
	for _, sz := range [][2]int{{20, 12}, {40, 20}, {97, 94}, {200, 40}, {10, 5}} {
		b, _ := run(t, "battery=75,2,4,2,41,4.1,900,900,4500,7,NaN,0", sz[0], sz[1], 30)
		if math.Abs(b.level-75) > 0.5 {
			t.Errorf("%v: level %.1f, want 75", sz, b.level)
		}
	}
}

// The charge keeps its volume however it sloshes; a steady push tilts its
// surface to a slope of a/g, the water piling up away from the push;
// and once the shaking stops, it settles.
func TestFluid(t *testing.T) {
	var f fluid
	vol := func() float64 {
		s := 0.0
		for i, h := range f.h {
			if f.wet[i] {
				s += h
			}
		}
		return s
	}
	for i := 0; i < 90; i++ { // a second of shaking, side to side
		a := 12 * math.Sin(float64(i)/30*2*math.Pi*2.5)
		f.step([3]float64{a, 0, a * 0.5}, 1.0/30)
	}
	if v := vol(); math.Abs(v) > 1e-9 {
		t.Fatalf("volume changed by %g", v)
	}
	shaken := f.energy()
	for i := 0; i < 240; i++ {
		f.step([3]float64{}, 1.0/30)
	}
	if e := f.energy(); e > shaken*0.05 || shaken < 1e-3 {
		t.Fatalf("energy %g after settling, %g shaken", e, shaken)
	}
	// Pushed steadily toward +x, the water piles up on the -x side.
	for i := 0; i < 600; i++ {
		f.step([3]float64{4.9, 0, 0}, 1.0/30)
	}
	if sx, sz := f.slope(0, 0); math.Abs(sx+0.5) > 0.05 || math.Abs(sz) > 0.02 {
		t.Fatalf("slope %.3f, %.3f under a steady push; want -0.5, 0", sx, sz)
	}
}

// Ripples a few cells long die out within a fraction of a second, while
// the whole surface's slosh lingers: no grid-sized jitter.
func TestFluidRipplesDie(t *testing.T) {
	var ripple, slosh fluid
	ripple.setup()
	slosh.setup()
	for j := 0; j < fluidN; j++ {
		for i := 0; i < fluidN; i++ {
			x, _ := ripple.center(i, j)
			ripple.h[j*fluidN+i] = 0.05 * float64(1-2*((i+j)%2)) // every other cell
			slosh.h[j*fluidN+i] = 0.2 * x
		}
	}
	e0r, e0s := ripple.energy(), slosh.energy()
	for i := 0; i < 6; i++ { // 0.2 s
		ripple.step([3]float64{}, 1.0/30)
	}
	for i := 0; i < 30; i++ { // 1 s
		slosh.step([3]float64{}, 1.0/30)
	}
	if e := ripple.energy(); e > e0r*0.05 {
		t.Errorf("ripples kept %.0f%% of their energy after 0.2 s", 100*e/e0r)
	}
	if e := slosh.energy(); e < e0s*0.3 {
		t.Errorf("the slosh kept only %.0f%% of its energy after 1 s", 100*e/e0s)
	}
}

// A phone's jittery acceleration, held still in a hand, barely ruffles the
// surface.
func TestFluidIgnoresJitter(t *testing.T) {
	var f fluid
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 120; i++ {
		f.step([3]float64{(rng.Float64() - 0.5) * 1.6, (rng.Float64() - 0.5) * 1.6, (rng.Float64() - 0.5) * 1.6}, 1.0/60) // about the noise measured on the phone, at its worst axis
	}
	top := 0.0
	for i, h := range f.h {
		if f.wet[i] {
			top = math.Max(top, math.Abs(h))
		}
	}
	if top > 0.03 {
		t.Errorf("jitter raised waves of %.3f", top)
	}
}
