package main

import (
	"math"
	"testing"
)

// However the phone turns, the view stays within its limits: never more
// than 50 degrees around the candle, never from below its rim, never from
// straight above; and the reference pose looks from the default angle.
func TestCandleViewBounded(t *testing.T) {
	deg := 180 / math.Pi
	if v := candleView(Vec3{0, 0, 1}); math.Abs(math.Asin(v[1])*deg-27) > 0.5 || math.Abs(v[0]) > 1e-9 {
		t.Fatalf("rest view %v, want 27 degrees up, straight on", v)
	}
	for yaw := -180.0; yaw <= 180; yaw += 10 {
		for pitch := -89.0; pitch <= 89; pitch += 8 {
			y, p := yaw/deg, pitch/deg
			v := candleView(Vec3{math.Sin(y) * math.Cos(p), math.Sin(p), math.Cos(y) * math.Cos(p)})
			around := math.Atan2(v[0], v[2]) * deg
			up := math.Asin(v[1]) * deg
			if math.Abs(around) > 50 || up < 1.5 || up > 66 || math.Abs(v.Len()-1) > 1e-9 {
				t.Fatalf("turned %v, %v: view %.1f around, %.1f up", yaw, pitch, around, up)
			}
		}
	}
}

func BenchmarkCandle(b *testing.B) {
	ss, _ := OpenMock("orient=0,0,0;linear_acceleration=0,0,0;proximity=5")
	c := &candle{}
	c.Setup(ss)
	var f Frame
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Resize(88, 75)
		c.Draw(f.View(0, 0, 88, 75), ss, float64(i)/30, 1.0/30)
	}
}
