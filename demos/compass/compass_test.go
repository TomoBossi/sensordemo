package compass

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"

	"github.com/TomoBossi/sensord/client"
)

func clientEvent(v []float64) client.Event { return client.Event{T: 1, V: v} }

// TestCompassSpins turns the phone several full turns each way, in small
// steps, and draws every frame: the heading must stay in range throughout.
func TestCompassSpins(t *testing.T) {
	for _, dir := range []float64{1, -1} {
		ss := FakeStreams("rotation_vector", "magnetic_field")
		c := &compass{}
		var f Frame
		f.Resize(60, 30)
		for i := 0; i < 400; i++ {
			yaw := dir * float64(i) * 5 // 2000 degrees: five and a half turns
			ss.Get("rotation_vector").Push(clientEvent(QuatFromEuler([]float64{yaw, 0, 0})))
			c.Draw(f.View(0, 0, 60, 30), ss, float64(i)/30, 1.0/30)
			if c.heading < -math.Pi || c.heading > math.Pi {
				t.Fatalf("dir %v step %d: heading %v out of range", dir, i, c.heading)
			}
		}
	}
}
