package core

import (
	"math"
	"testing"
)

// TestHeadingFollowsLongAxis: with the long axis pointing a given way, the
// heading must not change when the phone rolls around that axis or tilts it
// moderately up or down. Held upright (long axis vertical), it follows the
// back of the phone instead.
func TestHeadingFollowsLongAxis(t *testing.T) {
	deg := func(r float64) float64 { return r * 180 / math.Pi }
	for _, yaw := range []float64{0, 70, -120} {
		for _, pitch := range []float64{0, 30, -30, 60} {
			for _, roll := range []float64{-80, -45, 0, 45, 80} {
				R, _ := FromRotationVector(QuatFromEuler([]float64{yaw, pitch, roll}))
				// quatFromEuler's yaw turns counterclockwise from north
				want := -yaw
				got := deg(Heading(R))
				if d := math.Abs(math.Remainder(got-want, 360)); d > 2 {
					t.Errorf("yaw %v pitch %v roll %v: heading %.1f, want %.1f", yaw, pitch, roll, got, want)
				}
			}
		}
	}
	// Upright, screen facing you: the back of the phone points where you look.
	R, _ := FromRotationVector(QuatFromEuler([]float64{30, 90, 0}))
	back := R.Apply(Vec3{0, 0, -1})
	want := deg(math.Atan2(back[0], back[1]))
	if d := math.Abs(math.Remainder(deg(Heading(R))-want, 360)); d > 2 {
		t.Errorf("upright: heading %.1f, want the back's %.1f", deg(Heading(R)), want)
	}
}
