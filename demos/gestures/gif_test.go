package gestures

import (
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=gestures).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// Gestures: a few steps, a chop, a twist, face down and up again, a lift.
func gifScene() gifs.Scene {
	const warm, T = 1.0, 10.5
	return gifs.Scene{Demo: "gestures", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			s := t - warm
			count := func(times ...float64) float64 {
				n := 0.0
				for _, at := range times {
					if s >= at {
						n++
					}
				}
				return n
			}
			steps := 0.0
			for at := 0.4; at < 2.8; at += 0.55 {
				if s >= at {
					steps++
				}
			}
			flip := 2.0
			if s >= 6.8 && s < 8.0 {
				flip = 1
			}
			return map[string][]float64{
				"step_detector": {1, steps},
				"chop_chop":     {1, count(3.4)},
				"flip_twist":    {1, count(5.1)},
				"flip":          {flip},
				"wake_gesture":  {1, count(9.2)},
			}
		}}
}
