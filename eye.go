package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "eye",
		desc: "a 3D eye: shuts when you cover the proximity sensor, looks where you tilt, lit by the room",
		uses: []string{"proximity", "light"},
		new:  func(specs []string) Demo { return &eye{open: 1, rng: rand.New(rand.NewSource(7))} },
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
	g := gaugesFor(p, 1)
	if l, err := ss.Subscribe("light", 0); err == nil {
		g = append(g, gaugesFor(l, 1)...)
	}
	if o, err := ss.Subscribe("game_rotation_vector", 30); err == nil {
		g = append(g, gaugesFor(o, 3)...)
	}
	ss.Subscribe("accelerometer", 30) // gaze follows gravity; not shown
	return g, nil
}

func (e *eye) Help() []string {
	return []string{
		"A 3D eye lit by a lamp above you: tilt or turn the phone and the light and the glint shift across it.",
		"Cover the top of the phone (near the earpiece) and it closes; tilt it and it looks toward the low side; it blinks on its own.",
		"Brighter light: brighter eye, smaller pupil; direct sun makes it squint.",
		"space  blink",
	}
}

func (e *eye) Key(k byte) {
	if k == ' ' {
		e.nextBlink = 0
	}
}

const (
	eyeBall = iota // sclera, iris and pupil, told apart when shading
	eyeSkin        // face and eyelids
	eyeCaruncle
)

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// smin blends two distances smoothly over k (polynomial smooth minimum).
func smin(a, b, k float64) float64 {
	h := clamp01(0.5 + 0.5*(b-a)/k)
	return b + (a-b)*h - k*h*(1-h)
}

func smax(a, b, k float64) float64 { return -smin(-a, -b, k) }

// almond is the eye opening: negative inside, narrower toward the corners.
func almond(x, y float64) float64 {
	const a, b = 0.95, 0.5
	yy := y / (b * (1 - 0.3*x*x/(a*a)))
	return (math.Hypot(x/a, yy) - 1) * b
}

// faceHeight is the skin surface around the eye, as a height above the
// eyeball's center: a gentle dome, the brow ridge above, the cheek below,
// and the crease above the upper lid.
func faceHeight(x, y float64) float64 {
	h := 0.66 - 0.06*x*x - 0.04*y*y
	h += 0.24 * math.Exp(-(y-1.1)*(y-1.1)/0.09) * clamp01(1-0.2*x*x) // brow
	h += 0.12 * math.Exp(-((y+1.05)*(y+1.05)/0.1 + x*x/1.6))         // cheek
	yc := 0.66 + 0.2*(1-x*x/1.2)                                     // crease arc
	if math.Abs(x) < 1.1 {
		h -= 0.08 * math.Exp(-(y-yc)*(y-yc)/0.006) * (1 - x*x/1.21)
	}
	return h
}

func (e *eye) ball(p Vec3) float64 {
	b := p.Len() - 1
	c := p.Sub(e.gaze.Scale(0.42)).Len() - 0.62 // the cornea bulges out
	return smin(b, c, 0.12)
}

func (e *eye) scene(p Vec3) (float64, int) {
	d, m := e.ball(p), eyeBall

	// Face: the height field, rolled smoothly into the socket at the opening.
	skin := (p[2] - faceHeight(p[0], p[1])) * 0.75
	skin = smax(skin, -almond(p[0], p[1]), 0.12)

	// Lids: a shell over the eyeball above the upper edge angle and below
	// the lower one, with rounded margins, blended into the face so they
	// fold into it.
	shell := math.Abs(p.Len()-1.07) - 0.055
	up := Vec3{0, math.Cos(e.thU), -math.Sin(e.thU)}
	lo := Vec3{0, -math.Cos(e.thL), math.Sin(e.thL)}
	lids := math.Min(smax(shell, -p.Dot(up), 0.05), smax(shell, -p.Dot(lo), 0.05))
	skin = smin(skin, lids, 0.1)
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

// eyeFade is 1 around the eye, falling smoothly to 0 by about half again
// its size, so the face dissolves into black.
func eyeFade(x, y float64) float64 {
	r := math.Hypot(x/1.3, y/1.0)
	f := clamp01((1.55 - r) / 0.65)
	return f * f * (3 - 2*f)
}

// camera frames the eye and some face around it; returns the pixel
// direction scales.
func eyeCamera(w, h int) (camZ, tanX, tanY float64) {
	camZ = 6
	aspect := float64(2*h) / float64(w)
	tanX = 1.8 / (camZ - 0.6)
	if tanX*aspect < 1.3/(camZ-0.6) { // wide screens: fit the height
		tanX = 1.3 / (camZ - 0.6) / aspect
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
			g := toScreen(ss, r.V) // gravity in screen coords is (-ax, +ay)
			tx, ty := math.Max(-1, math.Min(1, -g[0]/6)), math.Max(-1, math.Min(1, (g[1]-6)/5))
			e.lookX += (tx - e.lookX) * math.Min(1, dt*6)
			e.lookY += (ty - e.lookY) * math.Min(1, dt*6)
		}
	}

	e.gaze = Vec3{e.lookX * 0.5, -e.lookY * 0.4, 1}.Norm()
	// Lid edge angles; closed, the upper lid reaches past the lower one.
	e.thU = -0.18 + (0.5+0.18)*e.open
	e.thL = -0.1 + (-0.42+0.1)*e.open
	e.pupilA = 0.2 - 0.11*e.light

	// The lamp hangs above you in the room; in eye coordinates it moves as
	// the phone turns. A soft fixed light keeps the change gentle.
	lamp := Vec3{-0.35, 0.55, 0.75}.Norm()
	if o := ss.Get("game_rotation_vector"); o != nil {
		if r := o.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				lamp = R.Mul(screenFrame(ss)).T().Apply(Vec3{0.25, -0.35, 1}.Norm())
			}
		}
	}
	fill := Vec3{-0.4, 0.45, 0.8}.Norm()
	strength := 0.4 + 0.75*e.light

	camZ, tanX, tanY := eyeCamera(v.W, v.H)
	cam := Vec3{0, 0, camZ}
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * tanX
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * tanY
			dir := Vec3{u, w, -1}.Norm()
			e.shade(v, x, y, cam, dir, lamp, fill, strength)
		}
	}
	e.lashes(v, camZ, tanX, tanY)
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
				shininess, wet = 90, 1
				switch {
				case ang < e.pupilA:
					cols = pupilCol
					lum *= 0.35
				case ang < 0.44:
					cols = irisCol
					side := n.Sub(e.gaze.Scale(cosA))
					phi := math.Atan2(side[1], side[0])
					fiber := 0.75 + 0.25*math.Sin(phi*23+math.Sin(phi*7)*2)
					rim := 1 - 0.55*math.Pow(math.Max(0, (ang-0.34)/0.1), 2)
					lum *= fiber * rim
				default:
					cols = scleraCol
					// shadowed near the lids, as they overhang the eyeball
					lum *= 0.55 + 0.45*clamp01(1-math.Abs(p[1])/0.55)
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
			lum = math.Min(1, (lum+spec*0.5)*fade)
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

// lashes draws eyelashes along the upper lid margin, fanning outward, and
// drooping along the seam when the eye is shut.
func (e *eye) lashes(v *View, camZ, tanX, tanY float64) {
	project := func(p Vec3) (int, int) {
		s := camZ - p[2]
		return int((p[0]/s/tanX + 1) / 2 * float64(v.W)), int((1 - p[1]/s/tanY) / 2 * float64(v.H))
	}
	const r = 1.12
	lastX := -1
	for x := -0.82; x <= 0.82; x += 0.03 {
		rr := math.Sqrt(r*r - x*x)
		sx, sy := project(Vec3{x, rr * math.Sin(e.thU), rr * math.Cos(e.thU)})
		if sx == lastX || sx%2 != 0 {
			continue // about every other column
		}
		lastX = sx
		c := byte('|')
		switch {
		case x < -0.3:
			c = '\\'
		case x > 0.3:
			c = '/'
		}
		dy := -1
		if e.open < 0.2 { // shut: lashes hang down over the seam
			dy = 1
			c = map[byte]byte{'\\': '/', '/': '\\', '|': '|'}[c]
		}
		v.Set(sx, sy+dy, c, 234)
	}
}
