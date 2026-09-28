package candle

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=candle).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The candle: the view swings around it, up to look into the pool and
// down; two breaths of air make the flame lean and waver; a hand snuffs
// it (smoke curls up) and it's lit again. It ends as it began: lit,
// looking from the front, the smoke long gone.
func gifScene() gifs.Scene {
	const warm, T = 3.0, 12.0
	rec := func(t float64) float64 { return math.Max(0, t-warm) }
	pose := func(t float64) gifs.Pose {
		u := rec(t) / T * 2 * math.Pi
		return gifs.Pose{Yaw: 44 * math.Sin(u), Pitch: 20 * math.Sin(2*u)}
	}
	gust := func(s, at, dur float64) float64 { // one push and back
		if s < at || s > at+dur {
			return 0
		}
		return math.Sin(2 * math.Pi * (s - at) / dur)
	}
	lin := func(t float64) Vec3 {
		s := rec(t)
		return Vec3{4 * gust(s, 1.2, 0.9), 0, 3.5 * gust(s, 8.3, 1.0)}
	}
	return gifs.Scene{
		Demo: "candle", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			m := gifs.Motion(pose, lin, t)
			prox := 5.0
			if s := rec(t); t > warm && s > 3.0 && s < 4.6 {
				prox = 0 // a hand over the phone
			}
			m["proximity"] = []float64{prox}
			return m
		},
	}
}
