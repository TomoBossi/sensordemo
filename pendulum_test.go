package main

import (
	"math"
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
