package mapdemo

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=map).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The map around the square, turning as the phone turns, looking left and
// right.
func gifScene() gifs.Scene {
	const warm, T = 2.0, 8.0
	pose := func(t float64) gifs.Pose { // looking around, left and right
		return gifs.Pose{Yaw: 50 * math.Sin(2*math.Pi*gifs.Cycle(t, warm, T)), Pitch: 30}
	}
	return gifs.Scene{Demo: "map", Warm: warm, Secs: T,
		Inputs: func(t float64) map[string][]float64 {
			return gifs.Merge(gifs.Motion(pose, nil, t), map[string][]float64{"location": gifMapPlace, "gps": gifMapPlace})
		},
		Every: func(d Demo, t float64) {
			if t < warm-0.5 || t > warm-0.4 {
				return
			}
			// The map data arrives over the network: wait for it.
			m := d.(*mapDemo)
			for i := 0; i < 1800; i++ {
				m.mu.Lock()
				ok, loading, err := m.ways != nil, m.loading, m.lastErr
				m.mu.Unlock()
				if ok && !loading {
					return
				}
				if i%50 == 0 {
					fmt.Printf("map: waiting (loading %v, %q)\n", loading, err)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}}
}

// Where the map stands: GIFMAP=lat,lon to try another spot; the square
// by default, the Obelisco on 9 de Julio.
var gifMapPlace = func() []float64 {
	p := append([]float64(nil), gifs.Place...)
	if s := os.Getenv("GIFMAP"); s != "" {
		fmt.Sscanf(s, "%g,%g", &p[0], &p[1])
	}
	return p
}()
