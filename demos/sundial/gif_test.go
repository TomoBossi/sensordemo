package sundial

import (
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
	return gifs.Scene{Demo: "sundial", Start: start, ClockX: 3600, Warm: 0.2, Secs: 13,
		Inputs: func(t float64) map[string][]float64 {
			// From the west-south-west, looking north-east: the gnomon's
			// triangle in view, and its shadow as it swings round.
			pose := func(t float64) gifs.Pose { return gifs.Pose{Yaw: -70, Pitch: 55} }
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
