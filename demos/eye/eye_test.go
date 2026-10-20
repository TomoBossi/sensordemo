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

// hit marches a ray from the camera and reports what it meets, if anything.
func (e *eye) hit(cam, dir Vec3) (int, bool) {
	t := cam[2] - 1.5
	for i := 0; i < 200; i++ {
		d, m := e.scene(cam.Add(dir.Scale(t)))
		if d < 0.002 {
			return m, true
		}
		t += d
		if t > cam[2]+2 {
			break
		}
	}
	return 0, false
}

// The 3D lids open exactly to the margins the overlay draws: however the
// eye looks, wherever it's open or half shut, a ray through a point just
// past a drawn margin meets skin, never the eyeball, so the iris can't
// show outside the outline; and just inside the margins the eyeball shows.
func TestLidsMatchTheirMargins(t *testing.T) {
	camZ := 6.0
	cam := Vec3{0, 0, camZ}
	through := func(p Vec3) Vec3 { return p.Sub(cam).Norm() }
	at := func(x, th float64) Vec3 {
		rr := math.Sqrt(1.11*1.11 - x*x)
		return Vec3{x, rr * math.Sin(th), rr * math.Cos(th)}
	}
	for _, open := range []float64{1, 0.6} {
		for _, look := range [][2]float64{{0, 0}, {-1, 0}, {1, 0}, {0, 1}, {0, -1}, {-1, -1}, {1, 1}} {
			e := &eye{open: open, lookX: look[0], lookY: look[1]}
			e.pose()
			for x := -0.86; x <= 0.86; x += 0.04 {
				thU, _ := e.lidAngle(x, true)
				thL, _ := e.lidAngle(x, false)
				const past = 0.05 // radians beyond the margin, onto the lid
				for _, th := range []float64{thU + past, thL - past} {
					if m, ok := e.hit(cam, through(at(x, th))); ok && m == eyeBall {
						t.Fatalf("open %.1f, look %v: the eyeball shows past the margin at x=%.2f", open, look, x)
					}
				}
				// Where the opening is wide enough to see into (the margins are
				// rounded), the eyeball shows between them, or in the inner
				// corner the caruncle.
				if thU-thL > 0.3 {
					if m, ok := e.hit(cam, through(at(x, (thU+thL)/2))); !ok || (m != eyeBall && m != eyeCaruncle) {
						t.Fatalf("open %.1f, look %v: no eyeball between the margins at x=%.2f (got %d)", open, look, x, m)
					}
				}
			}
		}
	}
}
