package main

import (
	"math"
	"testing"
	"time"
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

// Snuffing and relighting quickly leaves no smoke rising through the
// flame: what hadn't risen never does, what was low is gone.
func TestCandleRelightStopsSmoke(t *testing.T) {
	ss, _ := OpenMock("orient=0,0,0;linear_acceleration=0,0,0;proximity=5")
	c := &candle{}
	c.Setup(ss)
	var f Frame
	step := func(n int) {
		for i := 0; i < n; i++ {
			f.Resize(60, 40)
			c.Draw(f.View(0, 0, 60, 40), ss, float64(i)/30, 1.0/30)
		}
	}
	step(10)
	for k := 0; k < 4; k++ {
		c.Key(' ') // out
		step(9)
		c.Key(' ') // lit again
		for _, p := range c.smoke {
			if p.age < 0 || p.p[1] < candleWickTop+2.8 {
				t.Fatalf("round %d: smoke at %.2f (age %.2f) with the flame lit", k, p.p[1], p.age)
			}
		}
		step(3)
	}
	step(40)
	if len(c.smoke) > 0 {
		t.Errorf("%d puffs still around 1.3 s after relighting", len(c.smoke))
	}
}

// Moving the phone to the right leaves the air behind: the flame leans
// left, and the air is unsettled.
func TestCandleLeansIntoTheWind(t *testing.T) {
	ss, _ := OpenMock("orient=0,0,0;linear_acceleration=3,0,0;proximity=5")
	c := &candle{}
	c.Setup(ss)
	time.Sleep(50 * time.Millisecond)
	var f Frame
	for i := 0; i < 30; i++ {
		f.Resize(60, 40)
		c.Draw(f.View(0, 0, 60, 40), ss, float64(i)/30, 1.0/30)
	}
	if c.sway[0] > -0.2 || c.turb < 0.3 {
		t.Errorf("lean %.2f, turbulence %.2f", c.sway[0], c.turb)
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
