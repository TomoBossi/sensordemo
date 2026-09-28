package hourglass

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=hourglass).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The hourglass, at its own minute: the first grains fall; the phone is
// turned over (the GIF turns with it) and those few grains fall back, so
// all the sand is on one side; turned again, the grains start to fall, as
// at the start.
func gifScene() gifs.Scene {
	const warm, T = 5.5, 4.6
	flip := func(t, at float64) float64 { return 180 * gifs.Ease(at, at+0.8, t) }
	angle := func(t float64) float64 { // turned about the screen, degrees
		a := flip(t, 0.4) + flip(t, warm-1.1) // before: all the sand to one side, then back
		s := t - warm
		return a + flip(s, 0.5) + flip(s, 0.5+0.8+1.8+0.4) // a loop ends 0.3 s after the second flip, as it began
	}
	return gifs.Scene{Demo: "hourglass", Warm: warm, Secs: T, Turn: angle,
		Inputs: func(t float64) map[string][]float64 {
			a := (angle(t) + gifs.Wave(t, T, 0, 1.2, 0, 0.6)) * math.Pi / 180
			g := Vec3{9.81 * math.Sin(a), 9.81 * math.Cos(a), 0.8}
			return map[string][]float64{"gravity": g[:], "accelerometer": g[:]}
		}}
}
