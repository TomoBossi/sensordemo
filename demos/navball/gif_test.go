package navball

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=navball).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The navball: the phone turned through heading, pitch and roll, back to
// where it began.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 12.0
	pose := func(t float64) gifs.Pose {
		u := gifs.Cycle(t, warm, T) * 2 * math.Pi
		return gifs.Pose{Yaw: 360 * gifs.Cycle(t, warm, T), Pitch: 35 + 40*math.Sin(u), Roll: 30 * math.Sin(2*u)}
	}
	return gifs.Scene{Demo: "navball", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifs.Place})
		}}
}
