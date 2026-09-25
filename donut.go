package main

import "math"

func init() {
	register(entry{
		name: "donut",
		desc: "the classic spinning ASCII donut, held still in space by the phone's orientation",
		uses: []string{"game_rotation_vector", "rotation_vector", "gyroscope"},
		new:  func(specs []string) Demo { return &donut{specs: specs, zoom: 1} },
	})
}

// donut renders a torus the way donut.c does (sample the surface, z-buffer,
// shade by the normal), except that its pose comes from the phone: it stays
// fixed in the room, so turning the phone turns your view of it.
type donut struct {
	specs  []string
	source string // orientation stream
	gyro   bool

	ref        Mat3 // orientation at start or last recenter
	hasRef     bool
	firstCount int // stream count at the first reading
	spin       bool
	angle      float64
	zoom       float64
	zbuf       []float64
}

// Shading ramp from donut.c, dark to bright, and matching warm colors.
const ramp = ".,-~:;=!*#$@"

var rampColor = [len(ramp)]uint8{94, 94, 130, 136, 172, 178, 214, 220, 221, 222, 223, 230}

func (d *donut) Setup(ss *Streams) ([]*Gauge, error) {
	d.source = "game_rotation_vector"
	for _, sp := range d.specs {
		if s, ok := ss.Lookup(sp); ok {
			switch s.Type {
			case "rotation_vector":
				d.source = sp // absolute: north-referenced
			case "gyroscope":
				d.gyro = true
			}
		}
	}
	if _, err := ss.Subscribe(d.source, 60); err != nil {
		return nil, err
	}
	g := []*Gauge{
		{Spec: d.source, Index: 0, Label: "quat x", Scale: &Symmetric{Max: 1}},
		{Spec: d.source, Index: 1, Label: "quat y", Scale: &Symmetric{Max: 1}},
		{Spec: d.source, Index: 2, Label: "quat z", Scale: &Symmetric{Max: 1}},
	}
	if d.gyro {
		if _, err := ss.Subscribe("gyroscope", 30); err != nil {
			return nil, err
		}
		for i, ax := range []string{"x", "y", "z"} {
			g = append(g, &Gauge{Spec: "gyroscope", Index: i, Label: "gyro " + ax, Unit: "rad/s", Scale: &Symmetric{Max: 3}})
		}
	}
	return g, nil
}

func (d *donut) Help() []string {
	return []string{
		"Turn the phone: the donut stays put in the room,",
		"so you see it from other sides.",
		"",
		"r  recenter (current orientation = front view)",
		"a  toggle auto-spin",
		"+ -  zoom",
	}
}

func (d *donut) Key(k byte) {
	switch k {
	case 'r':
		d.hasRef = false
	case 'a':
		d.spin = !d.spin
	case '+', '=':
		d.zoom = math.Min(d.zoom*1.15, 4)
	case '-', '_':
		d.zoom = math.Max(d.zoom/1.15, 0.3)
	}
}

func (d *donut) Draw(v *View, ss *Streams, t, dt float64) {
	w, h := v.W, v.H
	if w < 4 || h < 4 {
		return
	}

	// Pose of the donut in device coordinates: undo the phone's rotation
	// since the reference, so the donut keeps its place in the world.
	pose := Identity()
	live := false
	if r := ss.Get(d.source).Read(); r.OK {
		if R, ok := FromRotationVector(r.V); ok {
			// Sensor fusion needs a moment to converge after the sensor
			// powers on; take the reference from a settled reading.
			if d.firstCount == 0 {
				d.firstCount = r.Count
			}
			if !d.hasRef && r.Count-d.firstCount >= 30 {
				d.ref, d.hasRef = R, true
			}
			if d.hasRef {
				pose = R.T().Mul(d.ref)
				live = true
			}
		}
	}
	if d.spin || !live {
		d.angle += dt * 0.8
	}
	pose = pose.Mul(RotX(-0.6)).Mul(RotY(d.angle))

	if len(d.zbuf) != w*h {
		d.zbuf = make([]float64, w*h)
	}
	for i := range d.zbuf {
		d.zbuf[i] = 0
	}

	const R1, R2 = 1.0, 2.0
	K2 := 6.0 / d.zoom                    // camera distance
	size := float64(min(w, 2*h))          // cells are about twice as tall as wide
	K1 := size * K2 * 0.4 / (R1 + R2)     // projection scale: fill ~80%
	light := Vec3{-0.3, 1, 1}.Norm()      // above, slightly left, toward the viewer
	nTheta := int(math.Max(90, size*1.3)) // denser sampling on bigger screens
	nPhi := int(math.Max(300, size*4))

	for i := 0; i < nTheta; i++ {
		th := 2 * math.Pi * float64(i) / float64(nTheta)
		ct, st := math.Cos(th), math.Sin(th)
		for j := 0; j < nPhi; j++ {
			ph := 2 * math.Pi * float64(j) / float64(nPhi)
			cp, sp := math.Cos(ph), math.Sin(ph)
			ring := R2 + R1*ct
			p := pose.Apply(Vec3{ring * cp, R1 * st, -ring * sp})
			depth := K2 - p[2]
			if depth <= 0.1 {
				continue
			}
			ooz := 1 / depth
			x := int(float64(w)/2 + K1*ooz*p[0])
			y := int(float64(h)/2 - K1*ooz*p[1]/2)
			if x < 0 || y < 0 || x >= w || y >= h || ooz <= d.zbuf[y*w+x] {
				continue
			}
			d.zbuf[y*w+x] = ooz
			n := pose.Apply(Vec3{ct * cp, st, -ct * sp})
			L := n.Dot(light)
			k := 0
			if L > 0 {
				k = min(int(L*float64(len(ramp))), len(ramp)-1)
			}
			v.Set(x, y, ramp[k], rampColor[k])
		}
	}
	if !live {
		v.Text(1, h-1, "waiting for orientation to settle...", 244)
	}
}
