package main

import (
	"math"
)

func init() {
	register(entry{
		name: "space",
		desc: "a window into a 3D world: turn the phone to look around, glide or walk to move",
		uses: []string{"game_rotation_vector", "rotation_vector", "step_detector"},
		new:  func(specs []string) Demo { return &space{stepMode: true, pendingToggle: true} },
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
	pendingToggle bool   // step mode changed; (un)subscribe on the next frame
	sky           []bool // per cell: the ray saw empty sky (stars may go there)
}

// start is between pillars (the grid puts one at the origin).
var start = Vec3{pillarGap / 2, pillarGap / 2, eyeHeight}

const (
	eyeHeight = 1.6
	stepLen   = 0.85 // meters per step
	walkSpeed = 1.6  // meters per second when gliding
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
		"p      step mode: real steps move you (on at start; toggle)",
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

// shade is the brightness ramp for surfaces, dark to bright.
var shade = []byte(".,:-=+*#%@")

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

	fov := math.Tan(35 * math.Pi / 180)
	aspect := float64(v.W) / float64(2*v.H)
	if len(s.sky) != v.W*v.H {
		s.sky = make([]bool, v.W*v.H)
	}
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * fov * aspect
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * fov
			dir := R.Apply(Vec3{u, w, -1}.Norm())
			s.sky[y*v.W+x] = s.cast(v, x, y, dir)
		}
	}

	// Stars come from a fixed catalog, projected into the cells that show
	// empty sky, so the sky holds still however the phone jitters.
	Rt := R.T()
	for _, st := range stars {
		c := Rt.Apply(st.dir)
		if c[2] > -0.05 {
			continue // behind the camera
		}
		x := int((c[0]/(-c[2])/(fov*aspect) + 1) / 2 * float64(v.W))
		y := int((1 - c[1]/(-c[2])/fov) / 2 * float64(v.H))
		if x >= 0 && y >= 0 && x < v.W && y < v.H && s.sky[y*v.W+x] {
			v.Set(x, y, st.c, st.fg)
		}
	}
	if s.stepErr != "" {
		v.Text(1, v.H-1, s.stepErr, 203)
	}
}

// cast renders one cell's ray and reports whether it saw empty sky.
func (s *space) cast(v *View, x, y int, dir Vec3) bool {
	const maxDist = 60.0
	dist := 0.0
	for i := 0; i < 80; i++ {
		p := s.pos.Add(dir.Scale(dist))
		d, mat := scene(p)
		if d < 0.01 {
			n := sceneNormal(p)
			diffuse := n.Dot(sunDir)
			light := 0.0
			if diffuse > 0 && !inShadow(p.Add(n.Scale(0.02))) {
				light = diffuse
			}
			lum := 0.25 + 1.0*light
			switch mat {
			case matFloor:
				// The sun is low, so a flat floor catches little of it;
				// scale by the sun's height so lit floor is bright again
				// (shadows stay dark). Checkered, 1 m tiles.
				lum = 0.25 + 0.8*light/sunDir[2]
				if (int(math.Floor(p[0]))+int(math.Floor(p[1])))&1 == 0 {
					lum *= 0.65
				}
			case matCrystal: // a glint where the sun reflects
				r := dir.Sub(n.Scale(2 * dir.Dot(n)))
				lum += 0.6 * math.Pow(math.Max(0, r.Dot(sunDir)), 12)
			case matMonolith:
				lum *= 0.5
			}
			fog := 1 - dist/maxDist
			lum = math.Min(1, lum*fog*(0.4+0.6*fog))
			if lum < 0.03 {
				return false
			}
			k := min(int(lum*float64(len(shade))), len(shade)-1)
			v.Set(x, y, shade[k], matColors[mat][k])
			return false
		}
		dist += d
		if dist > maxDist || p[2] > 15 {
			break
		}
	}
	if dir[2] <= 0 {
		return false // below the horizon but past the fog: stays dark
	}
	if c, fg, ok := shadeSky(dir); ok {
		if c != ' ' {
			v.Set(x, y, c, fg)
		}
		return false
	}
	return true
}
