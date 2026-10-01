package flicker

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=flicker).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The view circles the bulb twice, to show it in the round. The phone's back
// covered (the bulb off); under a lamp that doesn't
// flicker (it glows steadily); under one that flickers at 100 Hz (it pulses
// 12.5 times a second); under a dimmed LED strip that pulses at 390 Hz (it
// blinks as fast as the GIF can show). The readings carry the sensor's odd
// stray value, as on the phone.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 14.0
	return gifs.Scene{
		Demo: "flicker", Warm: warm, Secs: T, Fade: 1,
		Inputs: func(t float64) map[string][]float64 {
			s := math.Max(0, t-warm)
			flk, als := 2.0, 0.0
			switch {
			case t < warm || s < 2.5:
			case s < 5.5:
				flk, als = 6, 15000
			case s < 10:
				flk, als = 102, 4500
			default:
				flk, als = 390, 900
			}
			// A stray reading now and then, a run of four, as the sensor does.
			if k := int(t * 7.5); k%11 == 3 && flk != 2 {
				flk = []float64{424, 50, 222, 8}[k%4]
			}
			pose := func(t float64) gifs.Pose {
				c := gifs.Cycle(t, warm, T)
				// Twice round in a loop, the view circling the bulb: across and
				// up, a quarter turn apart, so the cage turns before the eye.
				// Quick enough that the resting pose, drifting after the
				// phone, doesn't take it all back.
				u := 2 * math.Pi * 2 * c
				return gifs.Pose{Yaw: 46 * math.Sin(u), Pitch: 26 * math.Cos(u)}
			}
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"rearflk": {flk}, "rearals": {als}})
		},
	}
}
