package eye

import (
	"math"
	"math/rand"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "eye",
		Desc: "a 3D eye: shuts when you cover the proximity sensor, looks where you tilt, lit by the room",
		Uses: []string{"proximity", "light"},
		New:  func(specs []string) Demo { return &eye{open: 1, rng: rand.New(rand.NewSource(7))} },
	})
}

// eye ray-marches a soft 3D model built from signed distance functions
// blended with smooth minimums, so parts merge like tissue: an eyeball with a
// corneal bulge, iris and pupil; a domed skin surface with a brow ridge, a
// cheek and the upper lid crease; eyelids that grow out of that skin with
// rounded margins and close over the eyeball; the skin rolling into the eye
// socket; and the caruncle in the inner corner. Lashes are drawn over it.
//
// The eyeball turns toward the low side when you tilt the phone; the lids
// close when the proximity sensor is covered, on blinks, and partly in bright
// light. Light comes from a lamp fixed in the room (so it shifts as the phone
// turns) mixed with a soft fixed light, scaled by the light sensor. The skin
// fades to black away from the eye.
//
// Eye coordinates are the screen's: x right, y up, z toward the viewer.
type eye struct {
	open      float64 // 0 closed .. 1 open, animated
	lookX     float64
	lookY     float64
	light     float64 // smoothed brightness, 0 dark .. 1 blinding
	nextBlink float64
	blinkEnd  float64
	rng       *rand.Rand

	// Recentering: the pose and gravity that count as "straight ahead", so
	// the gaze and the lamp are relative to how you hold the phone.
	ref        Mat3
	hasRef     bool
	firstCount int
	g0         Vec3
	hasG0      bool

	// per frame
	gaze   Vec3 // where the eyeball looks
	thU    float64
	thL    float64
	pupilA float64 // pupil angular radius
}

func (e *eye) Setup(ss *Streams) ([]*Gauge, error) {
	p, err := ss.Subscribe("proximity", 0)
	if err != nil {
		return nil, err
	}
	g := GaugesFor(p, 1)
	if l, err := ss.Subscribe("light", 0); err == nil {
		g = append(g, GaugesFor(l, 1)...)
	}
	if o, err := ss.Subscribe("game_rotation_vector", 30); err == nil {
		g = append(g, GaugesFor(o, 3)...)
	}
	ss.Subscribe("accelerometer", 30) // gaze follows gravity; not shown
	return g, nil
}

func (e *eye) Help() []string {
	return []string{
		"A 3D eye lit by a lamp above you: tilt or turn the phone and the light and the glint shift across it.",
		"Cover the top of the phone (near the earpiece) and it closes; tilt it and it looks toward the low side; it blinks on its own.",
		"Brighter light: brighter eye, smaller pupil; direct sun makes it squint.",
		"r      recenter: look straight ahead, lamp back in front",
		"space  blink",
	}
}

func (e *eye) Key(k byte) {
	switch k {
	case ' ':
		e.nextBlink = 0
	case 'r':
		e.hasRef, e.hasG0 = false, false
		e.firstCount = -1 // take the next reading, no settling wait
	}
}

const (
	eyeBall = iota // sclera, iris and pupil, told apart when shading
	eyeSkin        // face and eyelids
	eyeCaruncle
)

// almond is the eye opening: negative inside, narrower toward the corners.
func almond(x, y float64) float64 {
	const a, b = 1.0, 0.4 // long and low, like the first eye
	yy := y / (b * (1 - 0.3*x*x/(a*a)))
	return (math.Hypot(x/a, yy) - 1) * b
}

// faceHeight is the skin surface around the eye, as a height above the
// eyeball's center: a gentle dome, the brow ridge above, the cheek below,
// and the crease above the upper lid.
func faceHeight(x, y float64) float64 {
	h := 0.66 - 0.06*x*x - 0.04*y*y
	h += 0.24 * math.Exp(-(y-1.1)*(y-1.1)/0.09) * Clamp01(1-0.2*x*x) // brow
	h += 0.12 * math.Exp(-((y+1.05)*(y+1.05)/0.1 + x*x/1.6))         // cheek
	yc := 0.52 + 0.18*(1-x*x/1.3)                                    // crease arc
	if math.Abs(x) < 1.1 {
		h -= 0.08 * math.Exp(-(y-yc)*(y-yc)/0.006) * (1 - x*x/1.21)
	}
	return h
}

func (e *eye) ball(p Vec3) float64 {
	b := p.Len() - 1
	c := p.Sub(e.gaze.Scale(0.42)).Len() - 0.62 // the cornea bulges out
	return Smin(b, c, 0.12)
}

func (e *eye) scene(p Vec3) (float64, int) {
	d, m := e.ball(p), eyeBall

	// Face: the height field, rolled smoothly into the socket at the opening.
	skin := (p[2] - faceHeight(p[0], p[1])) * 0.75
	skin = Smax(skin, -almond(p[0], p[1]), 0.12)

	// Lids: a shell over the eyeball above the upper edge angle and below
	// the lower one, with rounded margins, blended into the face so they
	// fold into it.
	shell := math.Abs(p.Len()-1.07) - 0.055
	up := Vec3{0, math.Cos(e.thU), -math.Sin(e.thU)}
	lo := Vec3{0, -math.Cos(e.thL), math.Sin(e.thL)}
	lids := math.Min(Smax(shell, -p.Dot(up), 0.05), Smax(shell, -p.Dot(lo), 0.05))
	skin = Smin(skin, lids, 0.1)
	if skin < d {
		d, m = skin, eyeSkin
	}

	// Caruncle: the pink bump in the inner corner.
	car := p.Sub(Vec3{-0.8, -0.05, 0.5}).Len() - 0.1
	if car < d {
		d, m = car, eyeCaruncle
	}
	return d, m
}

func (e *eye) normal(p Vec3) Vec3 {
	const eps = 0.002
	f := func(q Vec3) float64 { v, _ := e.scene(q); return v }
	return Vec3{
		f(p.Add(Vec3{eps, 0, 0})) - f(p.Sub(Vec3{eps, 0, 0})),
		f(p.Add(Vec3{0, eps, 0})) - f(p.Sub(Vec3{0, eps, 0})),
		f(p.Add(Vec3{0, 0, eps})) - f(p.Sub(Vec3{0, 0, eps})),
	}.Norm()
}

var (
	eyeRamp     = []byte(".,:-=+*#%@")
	scleraCol   = []uint8{237, 239, 241, 244, 247, 249, 251, 253, 255, 231}
	irisCol     = []uint8{22, 28, 34, 64, 70, 106, 142, 148, 184, 186}
	pupilCol    = []uint8{16, 16, 232, 233, 234, 235, 236, 237, 238, 239}
	skinCol     = []uint8{52, 94, 95, 131, 137, 173, 174, 180, 181, 223}
	caruncleCol = []uint8{52, 88, 89, 125, 131, 167, 168, 174, 175, 218}
)

// frontLamp keeps the lamp in front of the face: the phone's rotation moves
// it only part of the way (damped), and never beyond a cone around straight
// ahead, so the eye can't end up lit from behind.
func frontLamp(rest, moved Vec3) Vec3 {
	l := rest.Add(moved.Sub(rest).Scale(0.6)).Norm()
	const minZ = 0.55 // at most ~57 degrees off straight ahead
	if l[2] < minZ {
		xy := math.Hypot(l[0], l[1])
		s := math.Sqrt(1-minZ*minZ) / math.Max(xy, 1e-9)
		l = Vec3{l[0] * s, l[1] * s, minZ}
	}
	return l
}

// eyeFade is 1 at the eye opening and dissolves outward: briefly at the
// corners, but slowly above and below, so more of the lids, the crease and
// the cheek show, fading gradually into black.
func eyeFade(x, y float64) float64 {
	lid := 0.4 * math.Sqrt(Clamp01(1-x*x)) // half-height of the opening here
	dx := math.Max(0, math.Abs(x)-0.85) / 0.4
	dy := math.Max(0, math.Abs(y)-lid) / 1.05
	r := math.Hypot(dx, dy)
	f := Clamp01(1 - r)
	return f * f * (3 - 2*f) // smooth, with a long soft tail
}

// camera frames the eye and some face around it; returns the pixel
// direction scales.
func eyeCamera(w, h int) (camZ, tanX, tanY float64) {
	camZ = 6
	aspect := float64(2*h) / float64(w)
	// The opening (half-width 0.95) spans ~80% of the width; on wide
	// screens, fit the height instead.
	tanX = 1.15 / (camZ - 0.6)
	if tanX*aspect < 0.72/(camZ-0.6) {
		tanX = 0.72 / (camZ - 0.6) / aspect
	}
	return camZ, tanX, tanX * aspect
}

func (e *eye) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 6 {
		return
	}
	// Proximity is binary on this phone: 0 (near) or its maximum (far).
	near := false
	if r := ss.Get("proximity").Read(); r.OK && len(r.V) > 0 {
		near = r.V[0] < 1
	}
	target := 0.5 // no light sensor: medium
	if l := ss.Get("light"); l != nil {
		if r := l.Read(); r.OK && len(r.V) > 0 {
			target = r.V[0] / (r.V[0] + 300)
		}
	}
	e.light += (target - e.light) * math.Min(1, dt*3)

	if t >= e.nextBlink {
		e.blinkEnd = t + 0.15
		e.nextBlink = t + 2.5 + e.rng.Float64()*4
	}
	full := 1 - math.Max(0, (e.light-0.88)/0.12)*0.5 // squint from ~3000 lux
	want := full
	if near || t < e.blinkEnd {
		want = 0
	}
	e.open += (want - e.open) * math.Min(1, dt*18)

	if a := ss.Get("accelerometer"); a != nil {
		if r := a.Read(); r.OK && len(r.V) >= 2 {
			g := ToScreen(ss, r.V) // gravity in screen coords is (-ax, +ay)
			if !e.hasG0 {
				e.g0, e.hasG0 = g, true
			}
			// Look toward the side that tipped down since recentering.
			tx := math.Max(-1, math.Min(1, -(g[0]-e.g0[0])/5))
			ty := math.Max(-1, math.Min(1, (g[1]-e.g0[1])/4))
			e.lookX += (tx - e.lookX) * math.Min(1, dt*6)
			e.lookY += (ty - e.lookY) * math.Min(1, dt*6)
		}
	}

	e.gaze = Vec3{e.lookX * 0.5, -e.lookY * 0.4, 1}.Norm()
	// Lid edge angles; closed, the upper lid reaches past the lower one.
	e.thU = -0.12 + (0.38+0.12)*e.open
	e.thL = -0.05 + (-0.32+0.05)*e.open
	e.pupilA = 0.24 - 0.12*e.light

	// The lamp hangs above you in the room; in eye coordinates it moves as
	// the phone turns. A soft fixed light keeps the change gentle.
	// At the reference pose the lamp is in front, a little up and left.
	lamp0 := Vec3{-0.3, 0.35, 1}.Norm()
	lamp := lamp0
	if o := ss.Get("game_rotation_vector"); o != nil {
		if r := o.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				R = R.Mul(ScreenFrame(ss))
				if e.firstCount == 0 {
					e.firstCount = r.Count
				}
				// wait for sensor fusion to settle, unless recentering
				if !e.hasRef && (e.firstCount < 0 || r.Count-e.firstCount >= 30) {
					e.ref, e.hasRef = R, true
				}
				if e.hasRef {
					lamp = frontLamp(lamp0, R.T().Mul(e.ref).Apply(lamp0))
				}
			}
		}
	}
	fill := Vec3{-0.4, 0.45, 0.8}.Norm()
	strength := 0.4 + 0.75*e.light

	camZ, tanX, tanY := eyeCamera(v.W, v.H)
	cam := Vec3{0, 0, camZ}
	ParallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * tanX
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * tanY
			dir := Vec3{u, w, -1}.Norm()
			e.shade(v, x, y, cam, dir, lamp, fill, strength)
		}
	})
	e.overlay(v, camZ, tanX, tanY)
}

func (e *eye) shade(v *View, x, y int, cam, dir, lamp, fill Vec3, strength float64) {
	// Rays that land far from the eye would fade to black anyway: skip them.
	if hx, hy := dir[0]/-dir[2]*(cam[2]-0.6), dir[1]/-dir[2]*(cam[2]-0.6); eyeFade(hx, hy) <= 0 {
		return
	}
	t := cam[2] - 1.5 // nothing is nearer than z = 1.5
	for i := 0; i < 80; i++ {
		p := cam.Add(dir.Scale(t))
		d, mat := e.scene(p)
		if d < 0.002 {
			// Fade the face to black away from the eye.
			fade := eyeFade(p[0], p[1])
			if fade <= 0 {
				return
			}
			n := e.normal(p)
			view := dir.Scale(-1)
			diffuse := 0.6*math.Max(0, n.Dot(lamp)) + 0.4*math.Max(0, n.Dot(fill))
			lum := (0.12 + 0.75*diffuse) * strength
			cols := skinCol
			shininess, wet := 14.0, 0.12
			switch mat {
			case eyeCaruncle:
				cols, shininess, wet = caruncleCol, 40, 0.5
			case eyeBall:
				cosA := n.Dot(e.gaze)
				ang := math.Acos(math.Max(-1, math.Min(1, cosA)))
				shininess, wet = 160, 1 // a tight, wet glint
				switch {
				case ang < e.pupilA:
					cols = pupilCol
					lum *= 0.35
				case ang < 0.5: // a big iris that touches the lids
					cols = irisCol
					side := n.Sub(e.gaze.Scale(cosA))
					phi := math.Atan2(side[1], side[0])
					fiber := 0.75 + 0.25*math.Sin(phi*23+math.Sin(phi*7)*2)
					rim := 1 - 0.55*math.Pow(math.Max(0, (ang-0.38)/0.12), 2)
					lum *= fiber * rim
				default:
					cols = scleraCol
					// shadowed near the lids, as they overhang the eyeball
					lum *= 0.55 + 0.45*Clamp01(1-math.Abs(p[1])/0.55)
				}
			}
			spec := (math.Pow(math.Max(0, n.Dot(lamp.Add(view).Norm())), shininess) +
				0.6*math.Pow(math.Max(0, n.Dot(fill.Add(view).Norm())), shininess)) * wet * strength
			if mat == eyeBall && spec > 0.5 {
				c := byte('O')
				if spec > 0.85 {
					c = '@'
				}
				v.Set(x, y, c, 231) // the wet glint
				return
			}
			// Dissolve: past the lids the face breaks into scattered cells,
			// fewer and dimmer outward, from a fixed per-cell noise so it
			// holds still.
			if fade < 1 && fade < CellNoise(x, y) {
				return
			}
			lum = math.Min(1, (lum+spec*0.5)*math.Sqrt(fade))
			if lum < 0.035 {
				return
			}
			k := min(int(lum*float64(len(eyeRamp))), len(eyeRamp)-1)
			v.Set(x, y, eyeRamp[k], cols[k])
			return
		}
		t += d
		if t > cam[2]+2 {
			return
		}
	}
}

// overlay draws what reads better as lines than as shading: the lid
// margins while the eye is open, and when it is shut, the closed lid as one
// curved line; lashes along the upper lid either way.
func (e *eye) overlay(v *View, camZ, tanX, tanY float64) {
	project := func(p Vec3) (int, int) {
		s := camZ - p[2]
		return int((p[0]/s/tanX + 1) / 2 * float64(v.W)), int((1 - p[1]/s/tanY) / 2 * float64(v.H))
	}
	// A point on the lid margin at angle th, at horizontal position x.
	margin := func(x, th float64) (int, int) {
		const r = 1.11
		rr := math.Sqrt(r*r - x*x)
		return project(Vec3{x, rr * math.Sin(th), rr * math.Cos(th)})
	}
	col := rampAt(skinCol, 0.55+0.45*e.light)
	const span = 0.9
	shut := e.open < 0.15
	lastLash := -99
	for x := -span; x <= span; x += 0.01 {
		taper := 1 - math.Abs(x)/span // margins meet at the corners
		if shut {
			// the closed lid: a gentle downward curve, like the first eye
			sx, sy := margin(x, (e.thU+e.thL)/2-0.04*taper)
			c := byte('=')
			if taper < 0.15 {
				c = '-'
			}
			v.Set(sx, sy, c, col)
			if sx != lastLash && sx%3 == 0 && taper > 0.2 {
				lastLash = sx
				lash := map[bool]byte{true: '/', false: '\\'}[x < 0]
				if math.Abs(x) < 0.25 {
					lash = '|'
				}
				v.Set(sx, sy+1, lash, 236)
			}
			continue
		}
		ux, uy := margin(x, e.thL+(e.thU-e.thL)*(0.5+0.5*math.Sqrt(taper)))
		lx, ly := margin(x, e.thU+(e.thL-e.thU)*(0.5+0.5*math.Sqrt(taper)))
		v.Set(ux, uy, '~', col)
		v.Set(lx, ly, '~', col)
		if ux != lastLash && ux%2 == 0 && taper > 0.12 {
			lastLash = ux
			lash := byte('|')
			switch {
			case x < -0.3:
				lash = '\\'
			case x > 0.3:
				lash = '/'
			}
			v.Set(ux, uy-1, lash, 236)
		}
	}
}

// rampAt picks the color at fraction f (0..1) of a dark-to-bright ramp.
func rampAt(ramp []uint8, f float64) uint8 {
	return ramp[min(int(math.Max(0, f)*float64(len(ramp))), len(ramp)-1)]
}
