package main

import (
	"fmt"
	"math"
)

func init() {
	register(entry{
		name: "navball",
		desc: "a 3D compass ball, as in aircraft: heading, pitch and roll of the phone at a glance",
		uses: []string{"geomagnetic_rotation_vector"},
		new:  func(specs []string) Demo { return &navball{view: viewGlobe} },
	})
}

// navball draws a 3D compass ball: a sphere fixed to the world (sky above, ground
// below, a horizon, meridians labeled N/E/S/W and parallels labeled with
// their elevation), seen from the phone. The ball's center is where the
// phone's long axis points, so turning the phone brings other directions to
// the center, tipping it up brings the sky, and rolling it tilts the horizon,
// like an aircraft's attitude indicator. Orientation comes from the rotation
// vector, which Android fuses from the magnetometer, gyroscope and
// accelerometer.
type navball struct {
	view int // viewGlobe; the others are kept for tests and experiments
	has  bool
	pose Mat3 // smoothed screen-to-world matrix
	// per-cell sky directions of the last frame, for grid edge detection
	az, el []float64
	in     []bool
}

func (c *navball) Setup(ss *Streams) ([]*Gauge, error) {
	if _, err := ss.Subscribe("rotation_vector", 30); err != nil {
		return nil, err
	}
	mag, err := subscribeMagnetics(ss)
	if err != nil {
		return nil, err
	}
	return gaugesFor(mag, 3), nil
}

func (c *navball) Help() []string {
	return []string{
		"The ball is a globe of directions sitting in the room, fixed to the world like the donut: sky on top, ground below, N E S W around it, elevation lines every 30 degrees.",
		"You see it from outside, through the phone: lay the phone flat and you look down on its top, like a compass rose; hold it up and you see its side. Turn the phone and the ball stays put.",
		"The readout is the phone's own heading (its long axis, or its back when upright), pitch and roll.",
		"If it drifts, wave the phone in a figure 8 to recalibrate the magnetometer; magnets and metal nearby bend it too.",
	}
}

const (
	viewWindow = iota // center = where the back of the phone faces, seen from inside
	viewGlobe         // the same ball seen from outside, like an object in the room
	viewNose          // center = where the long axis points (aircraft style)
)

var viewNames = []string{"window", "globe", "nose"}

func (c *navball) Key(k byte) {}

// camera returns the ball's viewing basis in world coordinates. By default
// it is the person's view: forward is out of the back of the phone and up is
// the screen's up, so a phone held upright shows sky above and ground below
// with the direction you face in the center. In nose view, forward is the
// long axis and up is out of the screen (the aircraft convention, natural
// with the phone flat). Either way (right, up, forward) is left-handed, as in
// a camera: the ball is seen from the inside, so east is right of north.
func camera(R Mat3, view int) (right, up, fwd Vec3) {
	if view == viewNose {
		return R.Apply(Vec3{1, 0, 0}), R.Apply(Vec3{0, 0, 1}), R.Apply(Vec3{0, 1, 0})
	}
	return R.Apply(Vec3{1, 0, 0}), R.Apply(Vec3{0, 1, 0}), R.Apply(Vec3{0, 0, -1})
}

func (c *navball) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	if r := ss.Get("rotation_vector").Read(); r.OK {
		if R, ok := FromRotationVector(r.V); ok {
			R = R.Mul(screenFrame(ss))
			if !c.has {
				c.pose, c.has = R, true
			}
			c.pose = blendRotation(c.pose, R, math.Min(1, dt*10))
		}
	}
	if !c.has {
		msg := "waiting for the rotation vector..."
		v.Text((v.W-len(msg))/2, v.H/2, msg, 244)
		return
	}
	right, up, fwd := camera(c.pose, c.view)
	decl, _ := declination(ss) // 0 without a fix: magnetic north
	// Globe view: the ball is seen from outside, so the side facing you
	// shows the directions toward you, and it turns the opposite way.
	depth := 1.0
	if c.view == viewGlobe {
		depth = -1
	}

	// Ball geometry: rows are twice as tall as columns.
	rr := math.Min(float64(v.H-4)/2, float64(v.W)/4-1) // radius in rows
	cx, cy := float64(v.W)/2, rr+1
	n := v.W * v.H
	if len(c.az) != n {
		c.az, c.el, c.in = make([]float64, n), make([]float64, n), make([]bool, n)
	}

	// First pass: the world direction behind every cell of the disc.
	parallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			i := y*v.W + x
			px := (float64(x) + 0.5 - cx) / (2 * rr)
			py := (cy - float64(y) - 0.5) / rr
			r2 := px*px + py*py
			c.in[i] = r2 < 1
			if !c.in[i] {
				continue
			}
			pz := math.Sqrt(1 - r2)
			d := right.Scale(px).Add(up.Scale(py)).Add(fwd.Scale(pz * depth))
			c.el[i] = math.Asin(math.Max(-1, math.Min(1, d[2]))) * 180 / math.Pi
			c.az[i] = math.Mod(math.Atan2(d[0], d[1])*180/math.Pi+decl+720, 360) // true azimuth
		}
	})

	// Second pass: shade the sphere and draw grid lines where a cell and
	// its right or lower neighbor fall on different sides of one.
	sky := []uint8{17, 18, 19, 20, 25, 26, 31, 32, 38, 39}
	ground := []uint8{52, 52, 94, 94, 130, 130, 136, 137, 173, 179}
	fill := []byte(".::-==++**")
	el := func(i int) float64 { return c.el[i] }
	az := func(i int) float64 { return c.az[i] }
	parallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			i := y*v.W + x
			if !c.in[i] {
				continue
			}
			px := (float64(x) + 0.5 - cx) / (2 * rr)
			py := (cy - float64(y) - 0.5) / rr
			lit := math.Sqrt(math.Max(0, 1-px*px-py*py)) // brighter toward the center
			k := min(int(lit*float64(len(fill))), len(fill)-1)
			ch, col := fill[k], sky[k]
			if c.el[i] < 0 {
				col = ground[k]
			}
			switch {
			case c.crosses(v, x, y, el, 0, 0):
				ch, col = '=', 231 // horizon
			case c.crosses(v, x, y, el, 30, 0):
				ch, col = '-', 250 // parallels every 30 degrees
			case math.Abs(c.el[i]) < 80 && c.crosses(v, x, y, az, 30, 360):
				ch, col = '|', 250 // meridians every 30 degrees
				if c.az[i] < 15 || c.az[i] > 345 {
					col = 196 // north's meridian in red
				}
			}
			v.Set(x, y, ch, col)
		}
	})

	// Labels, where their directions face the viewer.
	place := func(az, el float64, text string, col uint8) {
		d := skyDir(az-decl, el) // true azimuth to the magnetic frame of the pose
		if d.Dot(fwd)*depth < 0.25 {
			return // behind the ball, or too close to its rim to read
		}
		x := int(cx + d.Dot(right)*2*rr + 0.5)
		y := int(cy - d.Dot(up)*rr + 0.5)
		v.Text(x-len(text)/2, y, text, col)
	}
	// Cardinal letters sit well above the horizon, so they stay readable
	// whether the ball is seen from the side or from the top.
	labelEl := 6.0
	if c.view == viewGlobe {
		labelEl = 35
	}
	for i, p := range points {
		col := uint8(231)
		if p == "N" {
			col = 196
		}
		place(float64(i)*45, labelEl, p, col)
	}
	center := fwd.Scale(depth)
	nose := math.Mod(math.Atan2(center[0], center[1])*180/math.Pi+decl+360, 360)
	for _, e := range []float64{-60, -30, 30, 60} {
		place(nose+18, e, fmt.Sprintf("%+.0f", e), 229)
	}

	// Fixed marker at the center: where the phone points.
	v.Text(int(cx)-2, int(cy), "-=o=-", 226)

	// Readout under the ball.
	// Heading and pitch are the phone's (long axis, or the back when held
	// upright), the same as the compass, whatever the view shows.
	heading := math.Mod(math.Round(Heading(c.pose)*180/math.Pi+decl)+720, 360)
	// Pitch and roll of the phone itself: how far its long axis tips up,
	// and how far it is rolled around it (right side down is positive).
	pr, pu, pf := camera(c.pose, viewNose)
	pitch := math.Asin(math.Max(-1, math.Min(1, pf[2]))) * 180 / math.Pi
	roll := math.Atan2(-pr[2], pu[2]) * 180 / math.Pi
	// Round, and drop the sign of negative zero so it doesn't print "-00".
	pitch, roll = math.Round(pitch)+0, math.Round(roll)+0
	if pitch == 0 {
		pitch = 0
	}
	if roll == 0 {
		roll = 0
	}
	name := points[int(math.Mod(heading+22.5, 360)/45)%len(points)]
	line := fmt.Sprintf("heading %03.0f %-2s   pitch %+03.0f   roll %+04.0f", heading, name, pitch, roll)
	if len(line) > v.W {
		line = fmt.Sprintf("%03.0f %s  %+.0f  %+.0f", heading, name, pitch, roll)
	}
	ly := min(int(cy+rr+1.5), v.H-2)
	v.Text((v.W-len(line))/2, ly, line, 250)
	status, warn := magStatus(ss, v.W)
	col := uint8(244)
	if warn {
		col = 203
	}
	v.Text(max(0, (v.W-len(status))/2), ly+1, status[:min(len(status), v.W)], col)
}

// crosses reports whether a grid line of the given step (0: only the value
// 0) lies between cell (x, y) and its right or lower neighbor. wrap is the
// period of the value (360 for azimuth), or 0.
func (c *navball) crosses(v *View, x, y int, val func(int) float64, step, wrap float64) bool {
	i := y*v.W + x
	a := val(i)
	for _, j := range []int{i + 1, i + v.W} {
		if j >= len(c.in) || (j == i+1 && x+1 >= v.W) || !c.in[j] {
			continue
		}
		b := val(j)
		if wrap > 0 && math.Abs(b-a) > wrap/2 { // across the 0/360 seam
			if b > a {
				b -= wrap
			} else {
				b += wrap
			}
		}
		if step == 0 {
			if (a < 0) != (b < 0) {
				return true
			}
			continue
		}
		if math.Floor(a/step) != math.Floor(b/step) {
			return true
		}
	}
	return false
}

// blendRotation moves rotation matrix a a fraction f of the way to b and
// re-orthonormalizes it, to smooth sensor jitter.
func blendRotation(a, b Mat3, f float64) Mat3 {
	var m Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			m[i][j] = a[i][j] + (b[i][j]-a[i][j])*f
		}
	}
	// Gram-Schmidt on the columns.
	t := m.T()
	x := t[0].Norm()
	y := t[1].Sub(x.Scale(x.Dot(t[1]))).Norm()
	z := x.Cross(y)
	return Mat3{x, y, z}.T()
}
