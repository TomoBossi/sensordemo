package detector

import (
	"math"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/TomoBossi/sensord/client"
	. "github.com/TomoBossi/sensordemo/internal/core"
)

// A phone whose magnetometer reads Earth's field plus a big fixed offset,
// like this one before calibrating.
var (
	testEarth = Vec3{0, 20, -15} // world frame, 25 uT
	testBias  = Vec3{-18, 9, 30}
)

// poseRotation turns the phone so its axis a points up, spun yaw about
// the vertical: world = R * phone.
func poseRotation(a Vec3, yaw float64) Mat3 {
	z := Vec3{0, 0, 1}
	var r0 Mat3
	switch c := a.Dot(z); {
	case c > 0.999:
		r0 = Identity()
	case c < -0.999:
		r0 = RotAxis(Vec3{1, 0, 0}, math.Pi)
	default:
		r0 = RotAxis(a.Cross(z).Norm(), math.Acos(c))
	}
	return RotAxis(z, yaw).Mul(r0)
}

type fakePhone struct {
	ss  *Streams
	d   *detector
	v   *View
	now float64
	rng *rand.Rand
}

func newFakePhone(t *testing.T) *fakePhone {
	t.Setenv("SENSORDEMO_MAGCAL", filepath.Join(t.TempDir(), "magcal.json"))
	ss := FakeStreams("magnetic_field_uncalibrated", "gravity")
	var f Frame
	f.Resize(60, 30)
	return &fakePhone{ss: ss, d: &detector{source: "magnetic_field_uncalibrated", calib: &calibration{}},
		v: f.View(0, 0, 60, 30), rng: rand.New(rand.NewSource(1))}
}

// hold keeps the phone in orientation R, with a field extra added, for n
// frames, with a little sensor noise.
func (p *fakePhone) hold(R Mat3, extra float64, n int) {
	for i := 0; i < n; i++ {
		e := R.T().Apply(testEarth)
		e = e.Add(e.Norm().Scale(extra))
		raw := e.Add(testBias).Add(Vec3{p.rng.NormFloat64(), p.rng.NormFloat64(), p.rng.NormFloat64()}.Scale(0.3))
		up := R.T().Apply(Vec3{0, 0, 9.8})
		p.ss.Get("magnetic_field_uncalibrated").Push(clientEvent([]float64{raw[0], raw[1], raw[2], 0, 0, 0}))
		p.ss.Get("gravity").Push(clientEvent([]float64{up[0], up[1], up[2]}))
		p.now += 1.0 / 30
		p.d.Draw(p.v, p.ss, p.now, 1.0/30)
	}
}

func (p *fakePhone) calibrate() {
	for i, pose := range calPoses {
		p.hold(poseRotation(pose.axis, float64(i)), 0, 25)
	}
}

func TestDetectorCalibrates(t *testing.T) {
	p := newFakePhone(t)
	p.calibrate()
	d := p.d
	if !d.hasCal || d.calib != nil {
		t.Fatalf("not calibrated: %q", d.calMsg)
	}
	if e := d.cal.Bias.Sub(testBias).Len(); e > 1 {
		t.Errorf("offset %v, want %v (off by %.1f uT)", d.cal.Bias, testBias, e)
	}
	if math.Abs(d.cal.Radius-testEarth.Len()) > 1 {
		t.Errorf("Earth's field %.1f uT, want %.1f", d.cal.Radius, testEarth.Len())
	}
	// Saved and reused.
	d2 := &detector{}
	ss, _ := OpenMock("magnetic_field_uncalibrated=1,2,3,0,0,0;gravity=0,0,9.8")
	d2.Setup(ss)
	if !d2.hasCal || d2.cal != d.cal {
		t.Errorf("reloaded %+v, want %+v", d2.cal, d.cal)
	}
}

// Calibrated, turning the phone every which way doesn't move the needle,
// and metal does; the zero stays where it was set.
func TestDetectorIgnoresTurning(t *testing.T) {
	p := newFakePhone(t)
	p.calibrate()
	p.hold(Identity(), 0, 30) // zeroes
	if !p.d.hasBase {
		t.Fatal("no zero")
	}
	zero := p.d.base
	for i := 0; i < 40; i++ {
		a := Vec3{p.rng.NormFloat64(), p.rng.NormFloat64(), p.rng.NormFloat64()}.Norm()
		p.hold(poseRotation(a, p.rng.Float64()*6), 0, 3)
		if math.Abs(p.d.dev) > 1 {
			t.Fatalf("turning alone reads %+.1f uT", p.d.dev)
		}
	}
	p.hold(Identity(), 12, 30)
	if p.d.dev < 8 {
		t.Errorf("12 uT of metal reads %+.1f", p.d.dev)
	}
	p.hold(Identity(), 12, 30*60) // a minute over the metal
	if p.d.base != zero || p.d.dev < 8 {
		t.Errorf("zero moved to %.2f (was %.2f), reading %+.1f", p.d.base, zero, p.d.dev)
	}
}

// Zeroing waits for the phone to hold still.
func TestDetectorZeroWaitsForStillness(t *testing.T) {
	p := newFakePhone(t)
	p.calibrate()
	p.d.hasBase = false
	p.d.startZero()
	for i := 0; i < 60; i++ {
		p.hold(Identity(), 15*math.Sin(float64(i)), 1)
	}
	if p.d.hasBase {
		t.Fatalf("zeroed while the field swung: %.1f +- %.1f", p.d.base, p.d.noise)
	}
	p.hold(Identity(), 0, 30)
	if !p.d.hasBase || math.Abs(p.d.base-testEarth.Len()) > 0.5 {
		t.Errorf("zero %.2f after holding still, want about %.1f", p.d.base, testEarth.Len())
	}
}

func clientEvent(v []float64) client.Event { return client.Event{T: 1, V: v} }
