package kaleidoscope

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=kaleidoscope).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The kaleidoscope: turned slowly one way, then back; the pattern unfolds
// and folds back into where it began.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 12.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 120 * (1 - math.Cos(2*math.Pi*c)) / 2}
	}
	return gifs.Scene{Demo: "kaleidoscope", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 { return gifs.Motion(pose, nil, t) }}
}
