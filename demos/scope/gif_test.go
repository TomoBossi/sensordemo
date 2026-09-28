package scope

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=scope).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The scope: accelerometer, gyroscope and light as a hand moves the phone
// about and taps it, and a shadow passes; every trace repeats with the loop.
func gifScene() gifs.Scene {
	const warm, T = 9.0, 8.0
	pose := func(t float64) gifs.Pose {
		return gifs.Pose{Yaw: gifs.Wave(t, T, 25, 0, 8), Pitch: 50 + gifs.Wave(t, T, 0, 15, 0, 5), Roll: gifs.Wave(t, T, 0, 0, 12)}
	}
	lin := func(t float64) Vec3 {
		p := math.Mod(t, T/2)
		tap := 6 * math.Exp(-(p-1)*(p-1)/0.002)
		return Vec3{0, 0, tap}
	}
	return gifs.Scene{Demo: "scope", Specs: []string{"accelerometer", "gyroscope", "light"}, Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			m := gifs.Motion(pose, lin, t)
			m["light"] = []float64{math.Round(180 + 120*math.Sin(2*math.Pi*t/T) - 150*math.Exp(-math.Pow(math.Mod(t, T)-5.5, 2)/0.1))}
			return m
		}}
}
