package eye

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=eye).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The eye: held in the hand, it trembles a little and the iris follows;
// the room dims and the pupil opens, brightens and it narrows, bright sun
// and it squints; a hand passes over and it shuts.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 13.0
	pose := func(t float64) gifs.Pose {
		return gifs.Pose{Yaw: gifs.Wave(t, T, 0, 1.2, 0, 0.8, 0, 0.5), Pitch: 60 + gifs.Wave(t, T, 0, 0, 2.5, 0, 1.5, 0, 1), Roll: gifs.Wave(t, T, 0, 2, 0, 1.4, 0, 0, 0.8)}
	}
	return gifs.Scene{Demo: "eye", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			m := gifs.Motion(pose, nil, t)
			s := math.Max(0, t-warm)
			// Room light, dimming, then bright, then sun, then the room.
			lux := 150.0
			switch {
			case s < 2:
			case s < 4:
				lux = 150 - 130*gifs.Ease(2, 3, s)
			case s < 6:
				lux = 20 + 1480*gifs.Ease(4, 5, s)
			case s < 8:
				lux = 1500 + 20000*gifs.Ease(6, 6.8, s)
			default:
				lux = 21500 - 21350*gifs.Ease(8, 9.3, s)
			}
			prox := 5.0
			if s > 10.6 && s < 11.4 {
				prox = 0
			}
			m["light"] = []float64{math.Round(lux)}
			m["proximity"] = []float64{prox}
			return m
		}}
}
