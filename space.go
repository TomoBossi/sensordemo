package main

import (
	"math"
)

func init() {
	register(entry{
		name: "space",
		desc: "a window into a 3D world: turn the phone to look around, walk to move",
		uses: []string{"game_rotation_vector", "rotation_vector", "step_detector"},
		new:  func(specs []string) Demo { return &space{} },
	})
}

// space ray-marches a scene (one ray per character cell) through the phone:
// the view direction is the back of the phone (device -z) in world
// coordinates, so the screen behaves like a window. Each real step, from the
// step detector, walks the camera forward along where the phone faces.
type space struct {
	pos, target   Vec3 // camera position; target is where movement is heading
	steps         int  // pending steps from keys or the step detector
	stepsSeen     int
	stepMode      bool // p: real steps move you
	glide         bool // g: glide forward at walking speed
	stepErr       string
	pendingToggle bool // step mode changed; (un)subscribe on the next frame
}

// start is between pillars (the grid puts one at the origin).
var start = Vec3{pillarGap / 2, pillarGap / 2, eyeHeight}

const (
	eyeHeight = 1.6
	stepLen   = 0.75 // meters per step
	walkSpeed = 1.3  // meters per second when gliding
	pillarGap = 6.0  // pillar grid spacing
)

func (s *space) Setup(ss *Streams) ([]*Gauge, error) {
	s.pos, s.target = start, start
	st, err := ss.Subscribe("game_rotation_vector", 60)
	if err != nil {
		return nil, err
	}
	g := gaugesFor(st, 3)
	// Shown only while step mode is on (the stream exists only then).
	g = append(g, &Gauge{Spec: "step_detector", Label: "steps", Pulse: true})
	return g, nil
}

func (s *space) Help() []string {
	return []string{
		"The screen is a window: hold the phone up and turn around to look at the world behind it.",
		"",
		"w / s  step forward / back",
		"g      glide forward at walking speed (toggle)",
		"p      step mode: real steps move you (toggle)",
		"r      back to the start",
	}
}

func (s *space) Key(k byte) {
	switch k {
	case 'w':
		s.steps++
	case 's':
		s.steps--
	case 'g':
		s.glide = !s.glide
	case 'p':
		s.stepMode = !s.stepMode
		s.pendingToggle = true
	case 'r':
		s.pos, s.target = start, start
	}
}

// sdf is the distance from p to the scene, and which surface is nearest:
// 0 floor, 1 pillar, 2 sphere.
func sdf(p Vec3) (float64, int) {
	floor := p[2]
	// Repeat pillars on a grid by folding x and y into one cell.
	qx := math.Mod(math.Mod(p[0]+pillarGap/2, pillarGap)+pillarGap, pillarGap) - pillarGap/2
	qy := math.Mod(math.Mod(p[1]+pillarGap/2, pillarGap)+pillarGap, pillarGap) - pillarGap/2
	pillar := math.Max(math.Hypot(qx, qy)-0.45, p[2]-3)
	sphere := math.Sqrt(qx*qx+qy*qy+(p[2]-3.9)*(p[2]-3.9)) - 0.75
	d, id := floor, 0
	if pillar < d {
		d, id = pillar, 1
	}
	if sphere < d {
		d, id = sphere, 2
	}
	return d, id
}

func normal(p Vec3) Vec3 {
	const e = 0.01
	d := func(q Vec3) float64 { v, _ := sdf(q); return v }
	return Vec3{
		d(p.Add(Vec3{e, 0, 0})) - d(p.Sub(Vec3{e, 0, 0})),
		d(p.Add(Vec3{0, e, 0})) - d(p.Sub(Vec3{0, e, 0})),
		d(p.Add(Vec3{0, 0, e})) - d(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

var (
	shade     = []byte(".,:-=+*#%@")
	floorCols = []uint8{236, 238, 240, 242, 244, 246, 248, 250, 252, 254}
	pillarCol = []uint8{24, 25, 31, 32, 38, 39, 45, 81, 117, 159}
	sphereCol = []uint8{88, 124, 160, 196, 202, 208, 214, 220, 226, 229}
)

func (s *space) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 8 || v.H < 4 {
		return
	}
	R := Identity()
	if r := ss.Get("game_rotation_vector").Read(); r.OK {
		R, _ = FromRotationVector(r.V)
		R = R.Mul(screenFrame(ss)) // look through the screen as it is displayed
	} else {
		// no orientation yet: stand upright looking north, slowly turning
		R = RotZ(t * 0.2).Mul(RotX(math.Pi / 2))
	}

	// Step mode subscribes to the step detector only while it is on, so the
	// sensor is off the rest of the time.
	if s.pendingToggle {
		s.pendingToggle = false
		if s.stepMode {
			if st, err := ss.Subscribe("step_detector", 0); err != nil {
				s.stepMode, s.stepErr = false, "step mode unavailable: "+err.Error()
			} else {
				s.stepsSeen, s.stepErr = st.Read().Count, ""
			}
		} else {
			ss.Unsubscribe("step_detector")
		}
	}
	if s.stepMode {
		if st := ss.Get("step_detector"); st != nil {
			if r := st.Read(); r.Count > s.stepsSeen {
				s.steps += r.Count - s.stepsSeen
				s.stepsSeen = r.Count
			}
		}
	}

	// Movement is along the horizontal part of where the phone faces;
	// the camera glides toward the target.
	fwd := R.Apply(Vec3{0, 0, -1}) // out of the back of the screen
	if math.Hypot(fwd[0], fwd[1]) < 0.3 {
		fwd = R.Apply(Vec3{0, 1, 0}) // phone flat: use the screen's top edge
	}
	fwd = Vec3{fwd[0], fwd[1], 0}.Norm()
	if s.steps != 0 {
		s.target = s.target.Add(fwd.Scale(stepLen * float64(s.steps)))
		s.steps = 0
	}
	if s.glide {
		s.target = s.target.Add(fwd.Scale(walkSpeed * dt))
	}
	s.pos = s.pos.Add(s.target.Sub(s.pos).Scale(math.Min(1, dt*4)))

	sun := Vec3{0.4, 0.3, 0.85}.Norm()
	fov := math.Tan(35 * math.Pi / 180)
	aspect := float64(v.W) / float64(2*v.H)
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * fov * aspect
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * fov
			dir := R.Apply(Vec3{u, w, -1}.Norm())
			s.cast(v, x, y, dir, sun, t)
		}
	}
	if s.stepErr != "" {
		v.Text(1, v.H-1, s.stepErr, 203)
	}
}

func (s *space) cast(v *View, x, y int, dir, sun Vec3, t float64) {
	const maxDist = 45.0
	dist := 0.0
	for i := 0; i < 64; i++ {
		p := s.pos.Add(dir.Scale(dist))
		d, id := sdf(p)
		if d < 0.01 {
			n := normal(p)
			lum := math.Max(0, n.Dot(sun))*0.8 + 0.2
			fog := 1 - dist/maxDist
			lum *= fog * fog
			cols := floorCols
			switch id {
			case 0: // checkered floor, 1 m tiles
				if (int(math.Floor(p[0]))+int(math.Floor(p[1])))&1 == 0 {
					lum *= 0.55
				}
			case 1:
				cols = pillarCol
			case 2:
				cols = sphereCol
				lum = math.Min(1, lum*1.2)
			}
			k := min(int(lum*float64(len(shade))), len(shade)-1)
			if lum < 0.04 {
				return
			}
			v.Set(x, y, shade[k], cols[k])
			return
		}
		dist += d
		if dist > maxDist || p[2] > 12 {
			break
		}
	}
	// Sky: sparse stars fixed to directions, twinkling.
	if dir[2] > 0 {
		h := math.Sin(dir[0]*412.3+dir[1]*929.1+dir[2]*173.7) * 43758.5
		h -= math.Floor(h)
		if h > 0.985 {
			c := byte('.')
			if math.Sin(t*3+h*100) > 0.6 {
				c = '*'
			}
			v.Set(x, y, c, 229)
		}
	}
}
