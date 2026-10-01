package battery

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=battery).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The battery in use at a third, sparks leaving its top; plugged in by
// cable: pulses run in, bubbles stream up, the charge rises into green
// and the cell warms until heat shimmers over it; unplugged, in use
// again. The view drifts round it; the phone is shaken twice, and the
// charge sloshes and settles.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 13.0
	const plugAt, unplugAt = 3.5, 10.5
	rec := func(t float64) float64 { return math.Max(0, t-warm) }
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 22 * math.Sin(2*math.Pi*c), Pitch: 6 * math.Sin(4*math.Pi*c)}
	}
	push := func(s, at float64) float64 { // a nudge and back
		if s < at || s > at+0.5 {
			return 0
		}
		return math.Sin(2 * math.Pi * (s - at) / 0.5)
	}
	shake := func(s, at, dur, hz float64) float64 { // a few shakes, easing in and out
		if s < at || s > at+dur {
			return 0
		}
		return math.Sin(math.Pi*(s-at)/dur) * math.Sin(2*math.Pi*hz*(s-at))
	}
	lin := func(t float64) Vec3 {
		s := rec(t)
		return Vec3{3*(push(s, plugAt)-push(s, unplugAt)) + 9*shake(s, 0.8, 1.3, 2.2), 0, 7 * shake(s, 7, 1.1, 2.6)}
	}
	return gifs.Scene{
		Demo: "battery", Warm: warm, Secs: T, Fade: 1,
		Inputs: func(t float64) map[string][]float64 {
			s := rec(t)
			charging := s >= plugAt && s < unplugAt
			level, cur, temp := 33.0, -620.0, 31.0
			status, plugged := 3.0, 0.0
			switch {
			case charging:
				u := (s - plugAt) / (unplugAt - plugAt)
				level = 33 + 27*u
				cur = 1850
				temp = 31 + 12*Smoothstep(0, 0.8, u)
				status, plugged = 2, 2
			case s >= unplugAt:
				level, temp = 60, 43-6*(s-unplugAt)/(T-unplugAt)
			}
			volt := 3.7 + 0.004*level
			if charging {
				volt += 0.25
			}
			m := gifs.Motion(pose, lin, t)
			m["battery"] = []float64{level, status, plugged, 2, temp, volt, cur, cur * 0.95, 61.2 * level, 7, math.NaN(), 0}
			m["thermal"] = []float64{0, 0.4}
			return m
		},
	}
}
