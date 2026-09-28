package planetarium

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=planetarium).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The planetarium: the phone held up to the evening sky, turned across
// it and back.
func gifScene() gifs.Scene {
	const warm, T = 3.0, 12.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		// From the Southern Cross and Centaurus to Scorpius, and back.
		return gifs.Pose{Yaw: -235 + 35*math.Sin(2*math.Pi*c), Pitch: 133 + 8*math.Sin(4*math.Pi*c)}
	}
	return gifs.Scene{Demo: "planetarium", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
