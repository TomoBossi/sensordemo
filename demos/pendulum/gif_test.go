package pendulum

import (
	"math/rand"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=pendulum).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The pendulum, at twice the Speed: from an empty tray, the swing draws
// its figure; the end fades back to the empty tray.
func gifScene() gifs.Scene {
	return gifs.Scene{Demo: "pendulum", Warm: 0.5, Secs: 12, Fade: 1.5, Speed: 2,
		Setup: func(t *testing.T, d Demo) {
			p := d.(*pendulum)
			p.rng = rand.New(rand.NewSource(3))
			clear(p.h)
			p.sand = 1
			p.swing()
		},
		Inputs: func(t float64) map[string][]float64 {
			return map[string][]float64{"linear_acceleration": {0, 0, 0}}
		}}
}
