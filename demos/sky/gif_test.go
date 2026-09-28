package sky

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=sky).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The sky: the light sensor from covered to sunlight and back: new moon,
// waxing to full, glowing, the sun blazing.
func gifScene() gifs.Scene {
	const warm, T = 1.0, 12.0
	return gifs.Scene{Demo: "sky", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			c := gifs.Cycle(t, warm, T)
			// Up and back down on a log scale, 0.5 lux to 40000.
			x := (1 - math.Cos(2*math.Pi*c)) / 2
			lux := math.Pow(10, -0.3+4.9*x)
			return map[string][]float64{"light": {math.Round(lux)}}
		}}
}
