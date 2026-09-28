package sundial

import (
	"math"
	"testing"
	"time"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=sundial).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The sundial, from before sunrise to after sunset in 13 seconds: the
// sun comes up, the shadow sweeps round the hours, the sun goes down.
func gifScene() gifs.Scene {
	start := time.Date(2026, 9, 26, 6, 5, 0, 0, time.FixedZone("-03", -3*3600))
	const warm, T = 0.2, 13.0
	return gifs.Scene{Demo: "sundial", Start: start, ClockX: 3600, Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			// From the west-south-west, looking north-east: the gnomon's
			// triangle in view, and its shadow as it swings round. The
			// camera drifts slowly round the dial and back, rising and
			// dipping a little, so the loop meets itself.
			pose := func(t float64) gifs.Pose {
				c := gifs.Cycle(t, warm, T)
				return gifs.Pose{Yaw: -70 + 16*math.Sin(2*math.Pi*c), Pitch: 55 + 5*math.Sin(4*math.Pi*c)}
			}
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
