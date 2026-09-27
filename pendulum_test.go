package main

import (
	"math"
	"math/rand"
	"testing"
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
