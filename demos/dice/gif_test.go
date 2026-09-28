package dice

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=dice).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// Dice: six kinds in the tray, at rest; a shake throws them all, they
// tumble and settle, and the total shows; again. The end fades into the
// first throw's dice.
func gifScene() gifs.Scene {
	const warm, T = 5.0, 8.5
	return gifs.Scene{Demo: "dice", Arg: "1d4+1d6+1d8+1d10+1d12+1d20", Warm: warm, Secs: T, Fade: 0.8,
		Setup: func(t *testing.T, d Demo) { d.(*dice).rng = rand.New(rand.NewSource(11)) },
		Inputs: func(t float64) map[string][]float64 {
			s := t - warm
			shake := 0.0
			for _, at := range []float64{1.2, 5.0} {
				if s > at && s < at+0.6 {
					shake = 22 * math.Sin(2*math.Pi*(s-at)/0.2)
				}
			}
			return map[string][]float64{"linear_acceleration": {shake, 0.5 * shake, 0.25 * shake}}
		}}
}
