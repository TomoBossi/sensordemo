package homing

import (
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=homing).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// Homing: the phone lying flat, turned once round; the arrow keeps
// pointing at the saved place, a kilometre off.
func gifScene() gifs.Scene {
	const warm, T = 3.0, 10.0
	pose := func(t float64) gifs.Pose {
		return gifs.Pose{Yaw: 360 * gifs.Cycle(t, warm, T), Pitch: 20 + gifs.Wave(t, T, 0, 4), Roll: gifs.Wave(t, T, 0, 0, 3)}
	}
	return gifs.Scene{Demo: "homing", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
