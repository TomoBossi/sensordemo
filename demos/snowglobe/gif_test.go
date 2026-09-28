package snowglobe

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=snowglobe).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The snow globe: snow drifting down; a hard shake sends it swirling; a
// twist spins the liquid; the view turns a little. The last shake came
// as long before the start as the one in it before the end, so the end
// fades into a start that looks alike.
func gifScene() gifs.Scene {
	const warm, T = 20.0, 12.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 25*math.Sin(2*math.Pi*c) + twist(t, warm), Pitch: 60 + 8*math.Sin(4*math.Pi*c)}
	}
	lin := func(t float64) Vec3 {
		for _, at := range []float64{warm + 1.2 - T, warm + 1.2} { // a loop apart
			if s := t - at; s > 0 && s < 1.0 {
				return Vec3{15 * math.Sin(2*math.Pi*s/0.25), 9 * math.Sin(2*math.Pi*s/0.33), 0}
			}
		}
		return Vec3{}
	}
	return gifs.Scene{Demo: "snowglobe", Warm: warm, Secs: T, Fade: 1.5,
		Inputs: func(t float64) map[string][]float64 { return gifs.Motion(pose, lin, t) }}
}

// twist is a quick turn about the screen and back, for the snow globe.
func twist(t, warm float64) float64 {
	s := t - warm
	return 60 * (gifs.Ease(6.5, 7.1, s) - gifs.Ease(7.4, 8.2, s))
}
