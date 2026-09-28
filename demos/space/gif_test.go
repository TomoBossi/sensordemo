package space

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=space).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// Space: a walk round a square of the world's lanes, between its objects,
// turning at each corner and looking up and down as you go; you end where
// you began, facing the same way.
func gifScene() gifs.Scene {
	const warm, side, turn = 3.0, 3.15, 0.9
	const P = side + turn
	const T = 4 * P
	yaw := func(t float64) float64 {
		s := t - warm
		y := 10 * gifs.Ease(1, 2, t) // along a lane (the world is turned to face 190 degrees)
		for k := 0.0; k < 4; k++ {
			y += 90 * gifs.Ease(k*P+side, k*P+side+turn, s)
		}
		return y
	}
	pose := func(t float64) gifs.Pose {
		return gifs.Pose{Yaw: yaw(t), Pitch: 90 + 7*math.Sin(2*math.Pi*math.Max(0, t-warm)/P)}
	}
	return gifs.Scene{Demo: "space", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			m := gifs.Motion(pose, nil, t)
			steps := 0.0
			if s := t - warm; s > 0 {
				k := math.Floor(s / P)
				in := s - k*P
				steps = k * 7
				if in > 0.2 {
					steps += math.Min(7, math.Floor((in-0.2)/0.45)+1)
				}
			}
			m["step_detector"] = []float64{1, steps}
			return m
		}}
}
