package fluid

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=fluid).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The fluid: tipped one way and the other, and shaken.
func gifScene() gifs.Scene {
	const warm, T = 4.0, 8.0
	return gifs.Scene{Demo: "fluid", Warm: warm, Secs: T, Fade: 1.0,
		Inputs: func(t float64) map[string][]float64 {
			s := t - warm
			a := 0.0 // tilt about the screen, degrees
			a += 50 * (gifs.Ease(0.5, 1.5, s) - gifs.Ease(1.9, 2.9, s))
			a -= 55 * (gifs.Ease(2.9, 3.9, s) - gifs.Ease(4.3, 5.3, s))
			a += gifs.Wave(t, T, 0, 1.5)
			r := a * math.Pi / 180
			g := Vec3{9.81 * math.Sin(r), 9.81 * math.Cos(r), 1}
			lin := Vec3{}
			if s > 5.8 && s < 6.6 {
				lin = Vec3{12 * math.Sin(2*math.Pi*(s-5.8)/0.27), 0, 0}
			}
			acc := g.Add(lin)
			return map[string][]float64{"accelerometer": acc[:], "gravity": g[:], "linear_acceleration": lin[:]}
		}}
}
