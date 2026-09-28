package eye

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// TestLampStaysInFront: however the phone turns from the reference pose,
// the lamp stays in front of the face (so the eye is always lit).
func TestLampStaysInFront(t *testing.T) {
	rest := Vec3{-0.3, 0.35, 1}.Norm()
	ref, _ := FromRotationVector(QuatFromEuler([]float64{0, 80, 0}))
	worst := 1.0
	for yaw := -180.0; yaw <= 180; yaw += 30 {
		for pitch := -90.0; pitch <= 90; pitch += 30 {
			for roll := -180.0; roll <= 180; roll += 45 {
				R, _ := FromRotationVector(QuatFromEuler([]float64{yaw, pitch, roll}))
				l := frontLamp(rest, R.T().Mul(ref).Apply(rest))
				worst = math.Min(worst, l[2])
				if l[2] < 0.549 || math.Abs(l.Len()-1) > 1e-6 {
					t.Fatalf("pose %v %v %v: lamp %v not in front", yaw, pitch, roll, l)
				}
			}
		}
	}
	t.Logf("lowest lamp z over all poses: %.2f (1 = straight ahead)", worst)
}
