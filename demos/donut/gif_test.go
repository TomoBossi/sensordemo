package donut

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=donut).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The donut: held still in the room while you walk once around it, and
// look at it from above and below; the room's light rises from dim, where
// only its highlights show, to bright, and falls again.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 10.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 360 * c, Pitch: 70 + 25*math.Sin(2*math.Pi*c), Roll: gifs.Wave(t, T, 0, 2)}
	}
	return gifs.Scene{Demo: "donut", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			x := (1 - math.Cos(2*math.Pi*gifs.Cycle(t, warm, T))) / 2
			lux := math.Round(math.Pow(10, 0.3+3.2*x)) // 2 to 3000 lux
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"light": {lux}})
		}}
}
