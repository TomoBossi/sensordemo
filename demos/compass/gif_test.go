package compass

import (
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=compass).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The compass: the phone lying flat, turned once all the way round, a
// little unsteady in the hand.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 10.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 360 * c, Pitch: gifs.Wave(t, T, 0, 3, 0, 1), Roll: gifs.Wave(t, T, 0, 0, 2.5)}
	}
	return gifs.Scene{Demo: "compass", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
