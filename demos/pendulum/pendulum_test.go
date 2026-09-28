package pendulum

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func BenchmarkPendulum(b *testing.B) {
	ss, _ := OpenMock("linear_acceleration=0,0,0")
	s := &pendulum{}
	s.Setup(ss)
	var f Frame
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Resize(88, 75)
		s.Draw(f.View(0, 0, 88, 75), ss, float64(i)/30, 1.0/30)
	}
}

// Sand is neither lost nor made: all that left the funnel is in the tray,
// through settling and a shake.
func TestPendulumSandKept(t *testing.T) {
	ss, _ := OpenMock("linear_acceleration=0,0,0")
	s := &pendulum{}
	s.Setup(ss)
	for i := 0; i < 60*240; i++ {
		s.step(1.0/240, [2]float64{})
		if i%8 == 0 {
			s.relaxAll()
		}
	}
	s.level(0.8)
	s.relaxAll()
	var sum float64
	for _, h := range s.h {
		sum += h
	}
	got, want := sum*pendCell*pendCell, (1-s.sand)*180*pendFlow
	if want == 0 || math.Abs(got-want)/want > 1e-6 {
		t.Fatalf("tray holds %.6g, the funnel let out %.6g", got, want)
	}
	// And the swing died down, the funnel closing.
	if sz := s.swingSize(); sz > 0.1 {
		t.Errorf("still swinging %.2f after a minute", sz)
	}
}

// A fresh swing, whatever its ratio and phase, never reaches the wall: a
// pendulum's path is smooth, and bouncing off the wall would kink it.
func TestPendulumSwingFitsTray(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		ss, _ := OpenMock("linear_acceleration=0,0,0")
		s := &pendulum{}
		s.Setup(ss)
		s.rng = rand.New(rand.NewSource(seed))
		for r := range pendRatios {
			s.ratio = r
			s.swing()
			far := 0.0
			for i := 0; i < 240*40; i++ {
				s.move(1.0/240, [2]float64{})
				far = math.Max(far, math.Hypot(s.p[0], s.p[1]))
			}
			if far > 0.86 {
				t.Errorf("seed %d, %s: the swing reached %.3f, into the wall", seed, pendRatios[r].name, far)
			}
			if far < 0.6 {
				t.Errorf("seed %d, %s: the swing only reached %.3f, a small figure", seed, pendRatios[r].name, far)
			}
		}
	}
}

// Friction shrinks the swing evenly: as a plain pendulum (1:1) an oval
// stays the same oval rather than flattening into a line, a Lissajous
// figure keeps its proportions, and the size goes down steadily.
func TestPendulumKeepsItsShape(t *testing.T) {
	for _, c := range []struct {
		ratio  int
		ax, az float64
	}{{3, 0.5, 0.45}, {3, 0.6, 0.2}, {0, 0.6, 0.5}, {2, 0.4, 0.6}} {
		ss, _ := OpenMock("linear_acceleration=0,0,0")
		s := &pendulum{}
		s.Setup(ss)
		s.ratio = c.ratio
		wx, wz := s.omegas()
		s.p, s.v = [2]float64{c.ax, 0}, [2]float64{0, c.az * wz}
		size0 := math.Hypot(c.ax, c.az)
		for i := 0; i < 240*12; i++ {
			s.move(1.0/240, [2]float64{})
		}
		gx, gz := s.axisSwing(0, wx), s.axisSwing(1, wz)
		if want := c.az / c.ax; math.Abs(gz/gx-want) > 0.03*want {
			t.Errorf("%s from %.2f x %.2f: after 12 s, %.3f x %.3f (proportion %.3f, was %.3f)",
				pendRatios[c.ratio].name, c.ax, c.az, gx, gz, gz/gx, want)
		}
		if lost := size0 - math.Hypot(gx, gz); math.Abs(lost-0.36) > 0.05 {
			t.Errorf("%s: shrank %.3f in 12 s, want about 0.36", pendRatios[c.ratio].name, lost)
		}
	}
}
