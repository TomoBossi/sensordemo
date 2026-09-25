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

// eye ray-marches a 3D model: an eyeball with a corneal bulge, iris and
// pupil, two eyelid shells and the skin around the eye opening. The eyeball
// turns to look toward the low side when you tilt the phone; the lids close
// when the proximity sensor is covered, on blinks, and partly in bright light.
// A light fixed in the room (above you) lights it, so turning the phone moves
// the shading and the wet glint; the light sensor sets its strength and the
// pupil size.
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
		"A 3D eye lit by a lamp above you: tilt or turn the phone and the light and the glint move across it.",
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
	eyeLid
	eyeSkin
)

// almond is the eye opening in the skin: negative inside.
func almond(x, y float64) float64 {
	const a, b = 0.9, 0.52 // the ball's cross-section at the skin is ~0.87
	// narrower toward the corners than an ellipse
	yy := y / (b * (1 - 0.25*x*x/(a*a)))
	return (math.Hypot(x/a, yy) - 1) * b
}

func (e *eye) scene(p Vec3) (float64, int) {
	// Eyeball with a corneal bulge, blended smoothly.
	ball := p.Len() - 1
	cornea := p.Sub(e.gaze.Scale(0.42)).Len() - 0.62
	h := math.Max(0, math.Min(1, 0.5+0.5*(cornea-ball)/0.12))
	ball = cornea*(1-h) + ball*h - 0.12*h*(1-h)
	d, m := ball, eyeBall

	// Lids: a shell around the eyeball, covering what lies above the upper
	// edge angle or below the lower one (angles in the y-z plane).
	shell := math.Abs(p.Len()-1.08) - 0.06
	up := Vec3{0, math.Cos(e.thU), -math.Sin(e.thU)}
	lo := Vec3{0, -math.Cos(e.thL), math.Sin(e.thL)}
	lid := math.Min(math.Max(shell, -p.Dot(up)), math.Max(shell, -p.Dot(lo)))
	if lid < d {
		d, m = lid, eyeLid
	}

	// Skin: the plane z = 0.5 with the almond opening cut out.
	skin := math.Max(p[2]-0.5, -almond(p[0], p[1]))
	if skin < d {
		d, m = skin, eyeSkin
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
	eyeRamp   = []byte(".,:-=+*#%@")
	scleraCol = []uint8{237, 239, 241, 244, 247, 249, 251, 253, 255, 231}
	irisCol   = []uint8{22, 28, 34, 64, 70, 106, 142, 148, 184, 186}
	pupilCol  = []uint8{16, 16, 232, 233, 234, 235, 236, 237, 238, 239}
	skinCol   = []uint8{52, 94, 95, 131, 137, 173, 174, 180, 181, 223}
)

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

	// Per-frame model state.
	e.gaze = Vec3{e.lookX * 0.5, -e.lookY * 0.4, 1}.Norm()
	// Lid edge angles; closed, the upper lid reaches past the lower one so
	// they overlap with no gap.
	e.thU = -0.18 + (0.5+0.18)*e.open
	e.thL = -0.1 + (-0.42+0.1)*e.open
	e.pupilA = 0.2 - 0.11*e.light

	// The lamp hangs above you in the room; in eye coordinates it moves as
	// the phone turns. Without orientation, light from the upper left.
	lamp := Vec3{-0.35, 0.55, 0.75}.Norm()
	if o := ss.Get("game_rotation_vector"); o != nil {
		if r := o.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				lamp = R.Mul(screenFrame(ss)).T().Apply(Vec3{0.25, -0.35, 1}.Norm())
			}
		}
	}
	strength := 0.25 + 0.9*e.light

	// Camera in front of the eye, framing the opening.
	const camZ = 6.0
	aspect := float64(2*v.H) / float64(v.W)
	tanX := 1.3 / (camZ - 0.5)
	if tanX*aspect < 0.8/(camZ-0.5) { // wide screens: fit the height
		tanX = 0.8 / (camZ - 0.5) / aspect
	}
	tanY := tanX * aspect
	cam := Vec3{0, 0, camZ}
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * tanX
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * tanY
			dir := Vec3{u, w, -1}.Norm()
			e.shade(v, x, y, cam, dir, lamp, strength)
		}
	}
}

func (e *eye) shade(v *View, x, y int, cam, dir, lamp Vec3, strength float64) {
	t := 0.0
	for i := 0; i < 70; i++ {
		p := cam.Add(dir.Scale(t))
		d, mat := e.scene(p)
		if d < 0.002 {
			n := e.normal(p)
			view := dir.Scale(-1)
			half := lamp.Add(view).Norm()
			diffuse := math.Max(0, n.Dot(lamp))
			lum := (0.1+0.2*e.light)*1.0 + diffuse*strength*0.85
			cols := skinCol
			shininess, wet := 12.0, 0.15
			if mat == eyeBall {
				// Which part of the ball: angle from the gaze direction.
				cosA := n.Dot(e.gaze)
				ang := math.Acos(math.Max(-1, math.Min(1, cosA)))
				shininess, wet = 90, 1
				switch {
				case ang < e.pupilA:
					cols = pupilCol
					lum *= 0.35
				case ang < 0.44:
					cols = irisCol
					// radial fibers and a darker rim (limbus)
					side := n.Sub(e.gaze.Scale(cosA))
					phi := math.Atan2(side[1], side[0])
					fiber := 0.75 + 0.25*math.Sin(phi*23+math.Sin(phi*7)*2)
					rim := 1 - 0.55*math.Pow(math.Max(0, (ang-0.34)/0.1), 2)
					lum *= fiber * rim
				default:
					cols = scleraCol
					// a little shadow under the upper lid
					lum *= 0.75 + 0.25*math.Max(0, math.Min(1, (e.thU-math.Atan2(p[1], p[2]))*4))
				}
			} else if mat == eyeLid {
				shininess, wet = 20, 0.3
			}
			spec := math.Pow(math.Max(0, n.Dot(half)), shininess) * wet * strength
			if spec > 0.45 && mat == eyeBall {
				c := byte('O')
				if spec > 0.8 {
					c = '@'
				}
				v.Set(x, y, c, 231) // the wet glint
				return
			}
			lum = math.Min(1, lum+spec*0.6)
			k := min(int(lum*float64(len(eyeRamp))), len(eyeRamp)-1)
			v.Set(x, y, eyeRamp[k], cols[k])
			return
		}
		t += d
		if t > 12 {
			return
		}
	}
}
