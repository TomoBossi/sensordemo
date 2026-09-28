package detector

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=detector).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The detector: zeroed on a desk, swept slowly past a steel bolt (strong),
// then a screw (faint), and back to nothing.
func gifScene() gifs.Scene {
	const warm, T = 6.0, 10.0
	pose := func(t float64) gifs.Pose {
		return gifs.Pose{Yaw: 20 + gifs.Wave(t, T, 6, 0, 2), Pitch: gifs.Wave(t, T, 0, 1.5), Roll: gifs.Wave(t, T, 0, 0, 1)}
	}
	return gifs.Scene{Demo: "detector", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			m := gifs.Motion(pose, nil, t)
			// The sweep repeats from the start, so the trace at the
			// bottom (the last few seconds) is the same when the loop
			// wraps; the zero, in the first second, finds nothing near.
			s := math.Mod(t-warm+10*T, T)
			// The metal's field, in the phone's frame: it rises and falls as
			// the phone passes over.
			extra := Vec3{0.3, -0.5, 0.8}.Scale(70 * Bump(s, 2.2, 0.8)).Add(Vec3{-0.6, 0.2, 0.7}.Scale(9 * Bump(s, 7.0, 0.7)))
			u := m["magnetic_field_uncalibrated"]
			m["magnetic_field_uncalibrated"] = []float64{u[0] + extra[0], u[1] + extra[1], u[2] + extra[2], u[3], u[4], u[5]}
			c := m["magnetic_field"]
			m["magnetic_field"] = []float64{c[0] + extra[0], c[1] + extra[1], c[2] + extra[2], c[3]}
			return m
		}}
}
