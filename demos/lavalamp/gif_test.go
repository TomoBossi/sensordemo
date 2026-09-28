package lavalamp

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=lavalamp).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The lava lamp: a walk once around it; a shake breaks the wax into
// blobs that merge again. The wax won't repeat, so the end fades into
// the start.
func gifScene() gifs.Scene {
	const warm, T = 25.0, 10.0
	pose := func(t float64) gifs.Pose {
		c := gifs.Cycle(t, warm, T)
		return gifs.Pose{Yaw: 360 * c, Pitch: 90 + 8*math.Sin(2*math.Pi*c)}
	}
	lin := func(t float64) Vec3 {
		s := t - warm
		if s > 3.5 && s < 4.1 {
			return Vec3{14 * math.Sin(2*math.Pi*(s-3.5)/0.2), 0, 0}
		}
		return Vec3{}
	}
	return gifs.Scene{Demo: "lavalamp", Warm: warm, Secs: T, Fade: 1.2,
		Inputs: func(t float64) map[string][]float64 { return gifs.Motion(pose, lin, t) }}
}
