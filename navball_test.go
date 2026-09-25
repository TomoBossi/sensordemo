package main

import (
	"math"
	"testing"

	"github.com/TomoBossi/sensord/client"
)

// TestNavballSweep draws the navball over a sweep of orientations (turns,
// tilts, rolls, upside down, pointing straight up) and checks that the cell
// under the center marker shows the direction the long axis points.
func TestNavballSweep(t *testing.T) {
	var f Frame
	for yaw := -360.0; yaw <= 360; yaw += 45 {
		for pitch := -90.0; pitch <= 90; pitch += 30 {
			for roll := -180.0; roll < 180; roll += 60 {
				ss := &Streams{byKey: map[string]*Stream{"rotation_vector": {}, "magnetic_field": {}}}
				q := quatFromEuler([]float64{yaw, pitch, roll})
				ss.byKey["rotation_vector"].push(client.Event{T: 1, V: q})
				c := &navball{}
				f.Resize(60, 34)
				v := f.View(0, 0, 60, 34)
				c.Draw(v, ss, 0, 1)
				R, _ := FromRotationVector(q)
				nose := R.Apply(Vec3{0, 0, -1}) // default view: out of the back
				rr := math.Min(float64(v.H-4)/2, float64(v.W)/4-1)
				i := int(rr+1)*v.W + v.W/2 // the cell at the ball's center
				wantEl := math.Asin(nose[2]) * 180 / math.Pi
				if math.Abs(c.el[i]-wantEl) > 4 {
					t.Errorf("yaw %v pitch %v roll %v: center elevation %.1f, want %.1f", yaw, pitch, roll, c.el[i], wantEl)
				}
			}
		}
	}
}
