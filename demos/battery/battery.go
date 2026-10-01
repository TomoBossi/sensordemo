package battery

import (
	"fmt"
	"math"
	"math/rand"
	"strings"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "battery",
		Desc: "a 3D battery filled with glowing charge: its level, current, temperature and state, live",
		Uses: []string{"battery"},
		New:  func(specs []string) Demo { return &battery{} },
	})
}

// battery is the phone's battery as a cell in 3D, ray marched: a glass
// tube between a steel foot and a copper top, filled with glowing charge
// up to the battery's level, green when full, through yellow, to red when
// low. The charge is a liquid: its surface sloshes, waves and all, when
// the phone moves, and it stays level however you look at it.
//
// What it shows, from sensord's "battery":
//   - charging: a cable at its foot (or, charging wirelessly, rings on the
//     floor) carries pulses of light in; bands of light rise through the
//     charge and bubbles stream up, faster the more current flows;
//   - discharging: sparks leave from the top terminal, more the more it
//     draws, and the charge dims slowly downward;
//   - full: the surface twinkles;
//   - temperature: warm, a haze shimmers above the top and the glass
//     warms in tint; cold, frost gathers on the glass;
//   - battery saver: the charge dims;
//   - and in words below: level, current, power, voltage, temperature,
//     health, cycles, charge left and the time to empty or full, plus the
//     phone's thermal throttling ("thermal"), if it has any.
//
// Turning the phone swings the view around the cell, within limits, as in
// the candle. Shaking it sloshes the charge: its surface is shallow water
// (see fluid), pushed by the phone's acceleration.
//
// Scene units: the tube's radius is 1, y is up its axis, the floor at
// y = floorY.
type battery struct {
	rng *rand.Rand

	// Smoothed readings, so the charge rises and falls instead of jumping.
	level, cur, avg float64
	temp, volt      float64
	hasData         bool

	status, plugged, health int
	saver, throttle         int
	charge, cycles, toFull  float64

	flow  float64 // phase of the bands of light through the charge
	pulse float64 // phase of the pulses along the cable
	fl    fluid   // the charge's surface, sloshing

	sparks []spark
	haze   []spark

	ref        Mat3
	hasRef     bool
	firstCount int
	eye        Vec3

	t    float64
	zbuf []float64
}

type spark struct {
	p, v      Vec3
	age, life float64
	hot       bool // haze: shimmering heat, not a spark
}

// The battery's values, as sensord's "battery" reports them.
const (
	vLevel = iota
	vStatus
	vPlugged
	vHealth
	vTemp
	vVolt
	vCur
	vAvg
	vCharge
	vCycles
	vToFull
	vSaver
)

// Android's codes.
const (
	stCharging    = 2
	stDischarging = 3
	stNotCharging = 4
	stFull        = 5
	plugWireless  = 4
)

const (
	tubeR   = 1.0
	innerR  = 0.9
	glassY0 = -1.45 // the tube, between the caps
	glassY1 = 1.45
	capH    = 0.3
	nubR    = 0.36
	nubH    = 0.24
	floorY  = glassY0 - capH
	viewD   = 11.0
	viewEl  = 16 * math.Pi / 180
)

var (
	greenRamp  = []uint8{22, 22, 28, 28, 34, 35, 41, 47, 83, 84, 120}
	limeRamp   = []uint8{58, 64, 64, 70, 106, 112, 148, 154, 190, 191, 192}
	yellowRamp = []uint8{58, 94, 94, 136, 178, 214, 220, 221, 227, 228, 229}
	redRamp    = []uint8{52, 52, 88, 88, 124, 160, 196, 203, 209, 210, 217}
	copperRamp = []uint8{52, 94, 94, 130, 136, 172, 178, 179, 214, 220, 223}
	steelRamp  = []uint8{233, 235, 237, 239, 241, 243, 245, 247, 249, 251, 253, 255}
	glassRamp  = []uint8{23, 23, 24, 30, 31, 37, 73, 110, 152, 195, 231}
	warmRamp   = []uint8{52, 88, 130, 166, 172, 208, 214, 215, 222}
	frostRamp  = []uint8{24, 67, 110, 153, 189, 195, 231}
	cableRamp  = []uint8{233, 234, 235, 236, 238, 240, 243}
	ringRamp   = []uint8{17, 18, 24, 31, 38, 45, 87, 123, 159}
)

// chargeRamp is the charge's color at level (0..100).
func chargeRamp(level float64) []uint8 {
	switch {
	case level >= 55:
		return greenRamp
	case level >= 35:
		return limeRamp
	case level >= 16:
		return yellowRamp
	default:
		return redRamp
	}
}

func (b *battery) Setup(ss *Streams) ([]*Gauge, error) {
	b.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("battery", 4)
	if err != nil {
		return nil, err
	}
	ss.Subscribe("thermal", 1)
	if _, err := ss.Subscribe("game_rotation_vector", 60); err != nil {
		ss.Subscribe("rotation_vector", 60)
	}
	ss.Subscribe("linear_acceleration", 60)
	b.eye = Vec3{0, math.Sin(viewEl), math.Cos(viewEl)}
	return []*Gauge{
		{Spec: st.Spec, Index: vLevel, Label: "level", Unit: "%", Scale: &Linear{Min: 0, Max: 100}},
		{Spec: st.Spec, Index: vCur, Label: "current", Unit: "mA", Scale: &Symmetric{Max: 1000}},
		{Spec: st.Spec, Index: vTemp, Label: "temp", Unit: "C", Scale: &Linear{Min: 15, Max: 50}},
	}, nil
}

func (b *battery) Help() []string {
	return []string{
		"The phone's battery, live: the charge fills the cell up to its level, and pulses in while charging or sparks away while in use. Plug the phone in, unplug it, turn on battery saver, and watch it change.",
		"Turn the phone to look at the cell from another side (within a range); move it to slosh the charge.",
		"r  face it again",
	}
}

func (b *battery) Key(k byte) {
	if k == 'r' {
		b.hasRef = false
		b.firstCount = 0
	}
}

// read takes the latest readings, easing the continuous ones.
func (b *battery) read(ss *Streams, dt float64) {
	r := ss.Get("battery").Read()
	if !r.OK || len(r.V) <= vSaver {
		return
	}
	v := r.V
	ease := func(x *float64, to, tau float64) {
		if math.IsNaN(to) {
			return
		}
		if !b.hasData || math.IsNaN(*x) {
			*x = to
			return
		}
		*x += (to - *x) * math.Min(1, dt/tau)
	}
	ease(&b.level, v[vLevel], 0.6)
	ease(&b.cur, v[vCur], 0.3)
	avg := v[vAvg]
	if math.IsNaN(avg) {
		avg = v[vCur]
	}
	ease(&b.avg, avg, 20) // for the time left: steadier than the current
	ease(&b.temp, v[vTemp], 1)
	ease(&b.volt, v[vVolt], 1)
	b.status, b.plugged, b.health = int(v[vStatus]), int(v[vPlugged]), int(v[vHealth])
	b.charge, b.cycles, b.toFull = v[vCharge], v[vCycles], v[vToFull]
	b.saver = int(v[vSaver])
	b.hasData = true
	if tr := ss.Get("thermal").Read(); tr.OK && len(tr.V) > 0 {
		b.throttle = int(tr.V[0])
	}
}

func (b *battery) charging() bool { return b.status == stCharging && b.cur > 5 }

// look turns the phone's rotation since the reference into a view around
// the cell, easing into its limits.
func (b *battery) look(ss *Streams, dt float64) {
	spec := "game_rotation_vector"
	if ss.Get(spec) == nil {
		spec = "rotation_vector"
	}
	e := Vec3{0, 0, 1}
	if s := ss.Get(spec); s != nil {
		if r := s.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				R = R.Mul(ScreenFrame(ss))
				if b.firstCount == 0 {
					b.firstCount = r.Count
				}
				if !b.hasRef && r.Count-b.firstCount >= 20 {
					b.ref, b.hasRef = R, true
				}
				if b.hasRef {
					e = R.T().Apply(b.ref.Apply(Vec3{0, 0, 1}))
				}
			}
		}
	}
	b.eye = b.eye.Add(batteryView(e).Sub(b.eye).Scale(math.Min(1, dt*10))).Norm()
}

// batteryView maps where the screen faces now, in the reference pose's
// frame (e), to where the camera looks from: 60 degrees either side, and
// from level with the top to well above it.
func batteryView(e Vec3) Vec3 {
	const yawLim, pitchLim = 60 * math.Pi / 180, 34 * math.Pi / 180
	yaw := yawLim * math.Tanh(math.Atan2(e[0], e[2])*1.2/yawLim)
	pitch := math.Asin(math.Max(-1, math.Min(1, e[1])))
	el := viewEl + pitchLim*math.Tanh(pitch*1.2/pitchLim)
	el = 0.02 + 0.1*math.Log1p(math.Exp((el-0.02)/0.1)) // never from below the floor
	return Vec3{math.Sin(yaw) * math.Cos(el), math.Sin(el), math.Cos(yaw) * math.Cos(el)}
}

// fillY is the height of the charge's surface at the middle.
func (b *battery) fillY() float64 {
	return glassY0 + (glassY1-glassY0)*Clamp01(b.level/100)
}

func (b *battery) Draw(v *View, ss *Streams, t, dt float64) {
	dt = math.Min(dt, 0.1)
	b.t = t
	b.read(ss, dt)
	b.look(ss, dt)
	// Moving the phone moves the cell: in the scene, its acceleration is
	// along the view's axes (screen x is the view's right, y its up, z
	// toward the viewer), and it sloshes the charge.
	var acc Vec3
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			acc = ToScreen(ss, r.V)
		}
	}
	Rt, U := viewBasis(b.eye)
	aw := Rt.Scale(acc[0]).Add(U.Scale(acc[1])).Add(b.eye.Scale(acc[2]))
	for k := range aw {
		aw[k] = math.Max(-20, math.Min(20, aw[k]))
	}
	b.fl.step(aw, dt)
	amps := math.Abs(b.cur) / 1000
	switch {
	case b.charging():
		b.flow += dt * (0.25 + 0.6*math.Min(3, amps))
		b.pulse += dt * (0.6 + 1.2*math.Min(3, amps))
	case b.status == stDischarging || b.status == stNotCharging:
		b.flow -= dt * (0.08 + 0.25*math.Min(3, amps))
	}
	b.spawn(dt, amps)
	if v.H < 12 || v.W < 20 {
		return
	}
	textH := 9
	if v.H < 40 {
		textH = 4
	}
	b.render(v.Sub(0, 0, v.W, v.H-textH), t)
	b.words(v.Sub(0, v.H-textH, v.W, textH))
}

// spawn adds and moves the sparks leaving the top while the battery is in
// use, and the heat shimmering above it when it's warm.
func (b *battery) spawn(dt, amps float64) {
	top := glassY1 + capH + nubH
	if !b.charging() && b.cur < -5 && b.rng.Float64() < dt*(2+amps*8) {
		a := b.rng.Float64() * 2 * math.Pi
		r := nubR * 0.8 * math.Sqrt(b.rng.Float64())
		sp := 0.8 + 0.8*b.rng.Float64()
		b.sparks = append(b.sparks, spark{
			p:    Vec3{r * math.Cos(a), top + 0.02, r * math.Sin(a)},
			v:    Vec3{(b.rng.Float64() - 0.5) * 0.8, sp, (b.rng.Float64() - 0.5) * 0.8},
			life: 0.8 + 0.8*b.rng.Float64(),
		})
	}
	if heat := Smoothstep(36, 46, b.temp); heat > 0 && b.rng.Float64() < dt*heat*30 {
		a := b.rng.Float64() * 2 * math.Pi
		r := 0.9 * math.Sqrt(b.rng.Float64())
		b.haze = append(b.haze, spark{
			p:    Vec3{r * math.Cos(a), glassY1 + capH*0.6, r * math.Sin(a)},
			v:    Vec3{0, 0.5 + 0.3*b.rng.Float64(), 0},
			life: 1.6 + 1.2*b.rng.Float64(), hot: true,
		})
	}
	move := func(ps []spark, wobble float64) []spark {
		kept := ps[:0]
		for _, p := range ps {
			p.age += dt
			if p.age > p.life {
				continue
			}
			p.v[0] += (b.rng.Float64() - 0.5) * wobble * dt
			p.v[2] += (b.rng.Float64() - 0.5) * wobble * dt
			if !p.hot {
				p.v[1] -= 0.6 * dt // sparks slow as they arc away
			}
			p.p = p.p.Add(p.v.Scale(dt))
			kept = append(kept, p)
		}
		return kept
	}
	b.sparks = move(b.sparks, 3)
	b.haze = move(b.haze, 1.5)
}

// Materials.
const (
	mNone = iota
	mGlass
	mSteel
	mCopper
	mCable
	mPlug
)

// solidSDF is the cell's opaque parts and the glass tube's outside, and,
// when plugged in by wire, the cable along the floor to its foot.
func (b *battery) solidSDF(p Vec3) (float64, int) {
	r := math.Hypot(p[0], p[2])
	// The tube: its surface, where the ray goes on inside.
	d := math.Max(r-tubeR, math.Abs(p[1])-glassY1)
	m := mGlass
	// The foot and the top: short rounded cylinders a little wider than
	// the tube, and the terminal on top.
	rounded := func(r, y0, y1, rr, round float64) float64 {
		qx := r - (rr - round)
		qy := math.Abs(p[1]-(y0+y1)/2) - ((y1-y0)/2 - round)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - round
	}
	if foot := rounded(r, floorY, glassY0+0.02, tubeR+0.03, 0.08); foot < d {
		d, m = foot, mSteel
	}
	top := rounded(r, glassY1-0.02, glassY1+capH, tubeR+0.03, 0.08)
	top = Smin(top, rounded(r, glassY1+capH-0.1, glassY1+capH+nubH, nubR, 0.06), 0.06)
	if top < d {
		d, m = top, mCopper
	}
	if b.plugged != 0 && b.plugged&plugWireless == 0 {
		// The plug against the foot, and the cable from it, off to the right
		// and toward you.
		q := Vec3{p[0] - 1.25, p[1] - (floorY + 0.14), p[2] - 0.35}
		if pl := SdBox(q, Vec3{0.2, 0.12, 0.14}) - 0.03; pl < d {
			d, m = pl, mPlug
		}
		a, e := Vec3{1.4, floorY + 0.09, 0.35}, Vec3{3.6, floorY + 0.09, 9}
		ab := e.Sub(a)
		h := Clamp01(p.Sub(a).Dot(ab) / ab.Dot(ab))
		if c := p.Sub(a.Add(ab.Scale(h))).Len() - 0.09; c < d {
			d, m = c, mCable
		}
	}
	return d, m
}

func (b *battery) solidNormal(p Vec3) Vec3 {
	const e = 0.004
	var n Vec3
	for _, k := range [4]Vec3{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}} {
		d, _ := b.solidSDF(p.Add(k.Scale(e)))
		n = n.Add(k.Scale(d))
	}
	return n.Norm()
}

// chargeHit finds where a ray that entered the glass at p meets the
// charge, if it does before leaving the tube: it walks the span inside
// the tube in short steps, checking for the surface beneath, and narrows
// the crossing down. Fixed steps can't miss a steep wave, as a distance
// bound on a sloshing height field might.
func (b *battery) chargeHit(p, dir Vec3) (Vec3, bool) {
	if b.level < 0.5 {
		return Vec3{}, false
	}
	// Inside the tube's inner wall: from where the ray crosses it in, to
	// where it crosses it out.
	A := dir[0]*dir[0] + dir[2]*dir[2]
	B := 2 * (p[0]*dir[0] + p[2]*dir[2])
	C := p[0]*p[0] + p[2]*p[2] - innerR*innerR
	disc := B*B - 4*A*C
	if A < 1e-9 || disc <= 0 {
		return Vec3{}, false
	}
	sq := math.Sqrt(disc)
	t0, t1 := math.Max(0, (-B-sq)/(2*A)), (-B+sq)/(2*A)
	under := func(t float64) bool {
		q := p.Add(dir.Scale(t))
		return q[1] <= b.surfaceY(q[0], q[2]) && q[1] >= glassY0
	}
	if under(t0) {
		return p.Add(dir.Scale(t0)), true // the charge's side, against the glass
	}
	const step = 0.025
	for t := t0 + step; t < t1+step; t += step {
		t = math.Min(t, t1)
		if under(t) {
			lo, hi := t-step, t
			for k := 0; k < 8; k++ {
				if m := (lo + hi) / 2; under(m) {
					hi = m
				} else {
					lo = m
				}
			}
			return p.Add(dir.Scale(hi)), true
		}
		if t == t1 {
			break
		}
	}
	return Vec3{}, false
}

// surfaceY is the charge's surface height at x, z: its level, and the waves
// on it, inside the tube.
func (b *battery) surfaceY(x, z float64) float64 {
	if b.level < 0.5 {
		return glassY0
	}
	return math.Max(glassY0+0.01, math.Min(glassY1-0.03, b.fillY()+b.fl.at(x, z)))
}

var light = Vec3{-0.55, 0.65, 0.52}.Norm() // the room's light, from the upper left front

// flat is v's direction across the floor.
func flat(v Vec3) Vec3 { return Vec3{v[0], 0, v[2]}.Norm() }

// bayer4 orders a 4x4 cell's thresholds, for dithering.
var bayer4 = [16]float64{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

// viewBasis is the view's right and up, looking from eye at the cell.
func viewBasis(eye Vec3) (Vec3, Vec3) {
	F := eye.Scale(-1)
	Rt := F.Cross(Vec3{0, 1, 0}).Norm()
	return Rt, Rt.Cross(F)
}

func (b *battery) render(v *View, t float64) {
	w, h := v.W, v.H
	E := b.eye
	target := Vec3{0, 0.1, 0}
	F := E.Scale(-1)
	Rt, U := viewBasis(E)
	origin := target.Add(E.Scale(viewD))
	tanY := 2.45 / viewD
	tanX := tanY * float64(w) / (2 * float64(h))
	if fit := 1.9 / viewD; tanX < fit { // a narrow screen: fit the width instead
		tanY *= fit / tanX
		tanX = fit
	}
	if len(b.zbuf) != w*h {
		b.zbuf = make([]float64, w*h)
	}
	ParallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(h)*2
			dir := F.Add(Rt.Scale(u * tanX)).Add(U.Scale(vv * tanY)).Norm()
			hitT, m := math.Inf(1), mNone
			tt := 0.0
			for i := 0; i < 110 && tt < 40; i++ {
				p := origin.Add(dir.Scale(tt))
				if p[1] < floorY-0.01 {
					break
				}
				d, mm := b.solidSDF(p)
				if d < 0.003 {
					hitT, m = tt, mm
					break
				}
				tt += math.Max(d, 0.004)
			}
			b.zbuf[y*w+x] = hitT
			var ch byte
			var col uint8
			switch {
			case m == mGlass:
				ch, col = b.glass(origin.Add(dir.Scale(hitT)), dir, x, y, t)
			case m != mNone:
				ch, col = b.shadeSolid(origin.Add(dir.Scale(hitT)), dir, m, t)
			case dir[1] < 0:
				// The floor, lit by the charge's glow around the cell.
				ft := (floorY - origin[1]) / dir[1]
				b.zbuf[y*w+x] = ft
				ch, col = b.floor(origin.Add(dir.Scale(ft)), x, y, t)
			}
			if ch != 0 {
				v.Set(x, y, ch, col)
			}
		}
	})
	b.drawSparks(v, origin, F, Rt, U, tanX, tanY)
}

// glass shades a ray that met the tube at p: its reflection of the room's
// light (a bright stripe, and its rim, strong at grazing angles), over what
// shows through it: the charge, or the empty tube above it.
func (b *battery) glass(p, dir Vec3, x, y int, t float64) (byte, uint8) {
	n := Vec3{p[0], 0, p[2]}.Norm()
	cosi := -dir.Dot(n)
	fres := 0.04 + 0.96*math.Pow(1-math.Max(0, cosi), 5)
	refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
	cold := Smoothstep(8, 0, b.temp)
	warm := Smoothstep(36, 46, b.temp)
	// The room's light is a tall window: the glass mirrors it as a broad,
	// soft sheen where its reflection faces it, and it's lit on the side
	// toward it: together they show its roundness.
	sheen := math.Pow(math.Max(0, flat(refl).Dot(flat(light))), 4)
	lit := math.Max(0, n.Dot(flat(light)))
	th := (bayer4[(y%4)*4+x%4] + 0.5) / 16 // for dithering: density shows brightness
	// Frost, when cold: specks gathering at the rims first.
	if cold > 0 {
		k := Hash2(int(math.Atan2(p[2], p[0])*40), int(p[1]*18), 7)
		edge := 1 - cosi
		if float64(k%1000)/1000 < cold*(0.1+0.5*edge) {
			return "*.+"[k%3], Pick(frostRamp, 0.4+0.6*edge)
		}
	}
	// Into the tube: the charge, if the ray meets it before it leaves.
	q, hit := b.chargeHit(p, dir)
	rim := Smoothstep(0.35, 0.9, fres)
	if hit {
		if rim > 0.5 {
			return '|', Pick(glassRamp, 0.65+0.35*rim)
		}
		// The sheen brightens the charge behind it.
		return b.shadeCharge(q, dir, 0.55*sheen, t)
	}
	// The empty tube: its far wall, faint, catching the charge's glow just
	// above the surface; the glass warms in tint when hot.
	glowUp := 0.0
	if b.level > 0.5 {
		glowUp = 0.5 * math.Exp(-math.Max(0, p[1]-b.fillY())*1.6) * b.brightness()
	}
	ramp := glassRamp
	if warm > 0.3 {
		ramp = warmRamp
	}
	if glowUp > 0.22 && sheen < 0.3 { // the shine shows over the glow
		return ".:"[BoolIdx(glowUp > 0.4)], Pick(chargeRamp(b.level), glowUp)
	}
	if rim > 0.35 {
		return '|', Pick(ramp, 0.5+0.5*rim)
	}
	// The glass itself: faint, thicker toward the sides, lit on the side
	// toward the light and bright where it mirrors the window; the
	// brighter, the denser.
	i := 0.12 + 0.45*fres + 0.2*lit + 0.7*sheen
	if i*0.9 > th {
		return ".:+*"[min(3, int(i*3.2))], Pick(ramp, i)
	}
	return 0, 0
}

// brightness is how brightly the charge glows: fuller is brighter, battery
// saver dims it, charging lifts it.
func (b *battery) brightness() float64 {
	br := 0.55 + 0.35*Clamp01(b.level/100)
	if b.saver != 0 {
		br *= 0.6
	}
	if b.charging() {
		br += 0.1
	}
	if b.level < 16 { // low: a slow warning throb
		br *= 0.75 + 0.25*math.Sin(b.t*3)
	}
	return br
}

// shadeCharge lights the glowing charge at p: brightest at its surface,
// with the bands of light moving through it, the bubbles rising while it
// charges and the twinkles when full.
func (b *battery) shadeCharge(p, dir Vec3, lift, t float64) (byte, uint8) {
	ramp := chargeRamp(b.level)
	br := b.brightness()
	depth := b.surfaceY(p[0], p[2]) - p[1]
	i := br*(0.56+0.12*Noise3(p[0]*3, p[1]*3-t*0.3, p[2]*3)) + lift
	// Its side, round: lit toward the room's light, darker away from it.
	if side := math.Hypot(p[0], p[2]) / innerR; side > 0.97 {
		i *= 0.8 + 0.3*math.Max(0, Vec3{p[0], 0, p[2]}.Norm().Dot(flat(light)))
	}
	// The surface, a little brighter where its waves face the light.
	if depth < 0.05 {
		sx, sz := b.fl.slope(p[0], p[2])
		n := Vec3{-sx, 1, -sz}.Norm()
		i = math.Max(i, math.Min(br*0.9, br*(0.68+0.3*n.Dot(light))))
	} else {
		i += 0.25 * br * math.Exp(-depth*3)
	}
	// Bands of light: rising while charging, sinking slowly while in use.
	band := math.Pow(0.5+0.5*math.Sin(2*math.Pi*(p[1]*0.55-b.flow)), 10)
	if b.charging() {
		i += 0.3 * band
	} else {
		i += 0.15 * band
	}
	th := math.Atan2(p[2], p[0])
	// Bubbles, streaming up while charging, more and faster with more
	// current: each a character, in a grid of slots that rises with them,
	// most slots empty.
	if b.charging() && depth > 0.1 {
		amps := math.Min(3, b.cur/1000)
		u := th * innerR / 0.22
		vv := (p[1] - t*(0.8+amps*0.6)) / 0.34
		cu, cv := math.Floor(u), math.Floor(vv)
		k := Hash2(int(cu), int(cv), 11)
		if float64(k%1000)/1000 < 0.12+0.1*amps {
			// Where in its slot this one sits, and whether p is on it.
			ou, ov := 0.25+0.5*float64(k>>10%100)/100, 0.25+0.5*float64(k>>17%100)/100
			if math.Abs(u-cu-ou) < 0.13 && math.Abs(vv-cv-ov) < 0.16 {
				return "oO"[k>>24%2], Pick(ramp, 1)
			}
		}
	}
	// Full: the surface twinkles.
	if b.status == stFull && depth < 0.25 {
		k := Hash2(int(th*20), int(t*4), int(p[1]*9))
		if k%23 == 0 {
			return '*', 231
		}
	}
	const solid = ".:*#%@"
	return solid[int(Clamp01(i)*5+0.5)], Pick(ramp, i)
}

// shadeSolid lights the steel foot, the copper top, and the plug and cable.
func (b *battery) shadeSolid(p, dir Vec3, m int, t float64) (byte, uint8) {
	n := b.solidNormal(p)
	diff := math.Max(0, n.Dot(light))
	refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
	spec := math.Pow(math.Max(0, refl.Dot(light)), 30)
	// The charge lights what's near it from below: the top's underside
	// and the foot's inside, faintly.
	glow := 0.0
	if b.level > 0.5 {
		glow = 0.12 * b.brightness() * math.Exp(-math.Abs(p[1]-b.fillY())*1.5)
	}
	i := 0.08 + 0.6*diff + glow
	const solid = ".:*#%@"
	switch m {
	case mCopper:
		if spec > 0.5 {
			return '@', 230
		}
		i += spec * 0.5
		// The '+' on top of the terminal.
		if n[1] > 0.9 && p[1] > glassY1+capH+nubH-0.01 && (math.Abs(p[0]) < 0.05 && math.Abs(p[2]) < 0.2 || math.Abs(p[2]) < 0.05 && math.Abs(p[0]) < 0.2) {
			return '+', 52
		}
		return solid[int(Clamp01(i)*5+0.5)], Pick(copperRamp, i)
	case mSteel, mPlug:
		if spec > 0.5 {
			return '@', 255
		}
		i += spec * 0.5
		return solid[int(Clamp01(i)*5+0.5)], Pick(steelRamp, i)
	case mCable:
		// Pulses of light run along it toward the cell while charging.
		if b.charging() {
			s := p.Sub(Vec3{1.4, 0, 0.35}).Len()
			if f := s*1.2 + b.pulse*2; f-math.Floor(f) < 0.18 {
				return '#', Pick(chargeRamp(b.level), 1)
			}
		}
		i = 0.1 + 0.5*diff + spec*0.4
		return solid[int(Clamp01(i)*5+0.5)], Pick(cableRamp, i)
	}
	return 0, 0
}

// floor draws the table the cell stands on: dark, lit by the charge's glow
// around the foot, in sparse dots; a shadow toward the back right; and,
// charging wirelessly, rings spreading from under it.
func (b *battery) floor(p Vec3, x, y int, t float64) (byte, uint8) {
	r := math.Hypot(p[0], p[2])
	if r > 9 {
		return 0, 0
	}
	g := 0.0
	if b.level > 0.5 {
		g = b.brightness() * 0.7 / (1 + (r-1)*(r-1)*1.4)
	}
	if b.plugged&plugWireless != 0 {
		ring := math.Pow(0.5+0.5*math.Cos(2*math.Pi*(r*0.7-t*0.6)), 8) * Smoothstep(4.5, 1.2, r)
		if b.charging() && ring > 0.3 && r > 1.05 {
			return "-~="[BoolIdx(ring > 0.6)*2], Pick(ringRamp, ring)
		}
	}
	// The shadow the room's light casts: away from it.
	sx, sz := p[0]+light[0]*0.9, p[2]+light[2]*0.9
	if math.Hypot(sx, sz) < 1.05 {
		g *= 0.25
	}
	if (x+y)%2 != 0 || g < 0.08 {
		return 0, 0
	}
	return ".:"[BoolIdx(g > 0.3)], Pick(chargeRamp(b.level), Clamp01(g*0.8))
}

// drawSparks draws the sparks and the heat haze where nothing nearer hides
// them.
func (b *battery) drawSparks(v *View, origin, F, Rt, U Vec3, tanX, tanY float64) {
	w, h := v.W, v.H
	put := func(p spark, ch byte, col uint8) {
		d := p.p.Sub(origin)
		z := d.Dot(F)
		if z <= 0 {
			return
		}
		x := int((d.Dot(Rt)/z/tanX + 1) / 2 * float64(w))
		y := int((1 - d.Dot(U)/z/tanY) / 2 * float64(h))
		if x < 0 || y < 0 || x >= w || y >= h || d.Len() > b.zbuf[y*w+x] {
			return
		}
		v.Set(x, y, ch, col)
	}
	for _, p := range b.haze {
		f := p.age / p.life
		// Shimmering: the wave in it drifts as it rises.
		wave := math.Sin(p.p[1]*6 - b.t*5 + p.p[0]*3)
		if math.Abs(wave) < 0.3 {
			continue
		}
		put(p, "~-"[BoolIdx(f > 0.6)], Pick(warmRamp, 0.9-0.7*f))
	}
	for _, p := range b.sparks {
		f := p.age / p.life
		put(p, "*+'."[min(3, int(f*4))], Pick(chargeRamp(b.level), 1-0.7*f))
	}
}

// words prints the level in big type and the rest in words below the cell.
func (b *battery) words(v *View) {
	if !b.hasData {
		msg := "waiting for the battery..."
		v.Text((v.W-len(msg))/2, v.H/2, msg, 244)
		return
	}
	ramp := chargeRamp(b.level)
	pct := fmt.Sprintf("%d%%", int(math.Round(b.level)))
	lines := b.lines()
	if v.H >= 9 {
		bw, _ := BannerSize(pct, 2)
		DrawBanner(v, pct, (v.W-bw)/2, 0, 2, '#', Pick(ramp, 0.9))
		for i, l := range lines {
			v.Text(max(0, (v.W-len(l))/2), 6+i, l, []uint8{252, 247, 244}[i])
		}
		return
	}
	// Short screens: one line each.
	head := pct + "  " + lines[0]
	v.Text(max(0, (v.W-len(head))/2), 0, head, Pick(ramp, 0.9))
	for i, l := range lines[1:] {
		v.Text(max(0, (v.W-len(l))/2), 1+i, l, 247)
	}
}

// lines is the state, the electrics, and the charge and time left.
func (b *battery) lines() []string {
	var state string
	switch b.status {
	case stCharging:
		state = "charging"
		if src := plugName(b.plugged); src != "" {
			state += " (" + src + ")"
		}
	case stFull:
		state = "full"
	case stNotCharging:
		state = "plugged in, not charging"
	case stDischarging:
		state = "in use"
	default:
		state = "state unknown"
	}
	one := []string{state}
	if !math.IsNaN(b.cur) {
		one = append(one, fmt.Sprintf("%+.0f mA", b.cur))
		if !math.IsNaN(b.volt) {
			one = append(one, fmt.Sprintf("%.2f W", math.Abs(b.cur)*b.volt/1000))
		}
	}
	if b.saver != 0 {
		one = append(one, "battery saver")
	}
	two := []string{}
	if !math.IsNaN(b.temp) {
		two = append(two, fmt.Sprintf("%.1f C", b.temp))
	}
	if !math.IsNaN(b.volt) {
		two = append(two, fmt.Sprintf("%.3f V", b.volt))
	}
	two = append(two, "health "+healthName(b.health))
	if !math.IsNaN(b.cycles) {
		two = append(two, fmt.Sprintf("%.0f cycles", b.cycles))
	}
	if b.throttle > 0 {
		two = append(two, "throttling: "+throttleName(b.throttle))
	}
	three := []string{}
	rate := b.avg // steadier, once it agrees with what flows now
	if math.IsNaN(rate) || rate*b.cur <= 0 || math.Abs(rate) < 0.3*math.Abs(b.cur) {
		rate = b.cur
	}
	full := math.NaN()
	if !math.IsNaN(b.charge) && b.level > 1 {
		full = b.charge / b.level * 100
		three = append(three, fmt.Sprintf("%.0f of ~%.0f mAh", b.charge, full))
	}
	switch {
	case b.charging() && !math.IsNaN(b.toFull) && b.toFull > 0:
		three = append(three, "full in "+hm(b.toFull))
	case b.charging() && !math.IsNaN(full) && rate > 20:
		three = append(three, "full in about "+hm((full-b.charge)/rate*3600))
	case !b.charging() && b.status != stFull && !math.IsNaN(b.charge) && rate < -20:
		three = append(three, "about "+hm(b.charge/-rate*3600)+" left at this rate")
	}
	return []string{strings.Join(one, "   "), strings.Join(two, "   "), strings.Join(three, "   ")}
}

func hm(s float64) string {
	m := int(math.Round(s / 60))
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh %02dm", m/60, m%60)
}

func plugName(p int) string {
	switch {
	case p&plugWireless != 0:
		return "wireless"
	case p&1 != 0:
		return "charger"
	case p&2 != 0:
		return "USB"
	case p&8 != 0:
		return "dock"
	}
	return ""
}

func healthName(h int) string {
	switch h {
	case 2:
		return "good"
	case 3:
		return "overheating"
	case 4:
		return "dead"
	case 5:
		return "over voltage"
	case 6:
		return "failing"
	case 7:
		return "cold"
	}
	return "unknown"
}

func throttleName(s int) string {
	names := []string{"none", "light", "moderate", "severe", "critical", "emergency", "shutdown"}
	if s >= 0 && s < len(names) {
		return names[s]
	}
	return fmt.Sprint(s)
}
