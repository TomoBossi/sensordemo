package space

import (
	"math"
	"strconv"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// TestSpaceStartsFacingAwayFromSun: whatever the phone's arbitrary initial
// yaw, once aimed the view faces about 150 degrees from the sun.
func TestSpaceStartsFacingAwayFromSun(t *testing.T) {
	for _, yaw := range []float64{0, 90, 200, -45} {
		ss, _ := OpenMock("orient=" + ftoa(yaw) + ",85,0")
		s := &space{}
		s.Setup(ss)
		s.firstCount = -1 // don't wait for fusion to settle
		var f Frame
		f.Resize(40, 20)
		v := f.View(0, 0, 40, 20)
		for i := 0; i < 5; i++ {
			s.Draw(v, ss, 0, 1.0/30)
		}
		R, _ := FromRotationVector(QuatFromEuler([]float64{yaw, 85, 0}))
		fwd := s.yaw.Mul(R).Apply(Vec3{0, 0, -1})
		fwd = Vec3{fwd[0], fwd[1], 0}.Norm()
		sun := Vec3{sunDir[0], sunDir[1], 0}.Norm()
		ang := math.Acos(fwd.Dot(sun)) * 180 / math.Pi
		if math.Abs(ang-150) > 3 {
			t.Errorf("yaw %v: view is %.0f degrees from the sun, want ~150", yaw, ang)
		}
	}
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
