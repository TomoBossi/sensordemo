package main

import (
	"fmt"
	"math"
	"time"
)

func init() {
	register(entry{
		name: "sundial",
		desc: "a brass sundial set for your latitude, facing true north, its shadow cast by the real sun; turn the phone to walk around it",
		uses: []string{"rotation_vector", "location"},
		new:  func(specs []string) Demo { return &sundial{speed: 1} },
	})
}

// sundial is a horizontal garden sundial on a stone pedestal, ray marched
// from distance functions. It is laid out for where you are: its gnomon's
// edge (the style) rises at your latitude toward the celestial pole, and
// its hour lines fan out at the angles your latitude gives them. It stands
// in the world facing true north, and the Sun, placed from your location
// and the time, lights it and casts the gnomon's shadow: the shadow reads
// the sun's time, which differs from the clock by your longitude within
// your time zone and the equation of time. The phone's orientation places
// the camera: point it somewhere and you look at the dial from that side;
// hold it flat to look down on it.
//
// Scene units: the dial's radius is 1, x east, y up, z south.
type sundial struct {
	speed   float64 // time-lapse factor
	offset  time.Duration
	simTime time.Time
	last    time.Time
	az, el  float64 // which way the camera looks (east of north) and from how high, smoothed
	hasView bool

	lat, lon float64
	sun      Vec3 // toward the Sun
	hourAng  float64

	// Today's sunrise, noon and sunset (zero when the sun doesn't rise or
	// set), for the day they were found.
	day                 string
	rise, noon, set     time.Time
	gnomonLen, gnomonHt float64
}

const (
	dialTop   = 0.05 // the plate's face
	dialRoot  = 0.62 // the gnomon's root, from the middle toward the equator
	dialSlab  = -0.16
	dialFloor = -3.2
)

var (
	dialStone  = []uint8{234, 236, 238, 240, 242, 244, 246, 248, 250}
	dialGrass  = []uint8{232, 22, 22, 28, 28, 34, 70}
	dialBronze = []uint8{232, 52, 88, 124, 130, 166, 173, 215} // the gnomon, redder than the plate
)

func (s *sundial) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("rotation_vector", 30)
	if err != nil {
		return nil, err
	}
	ss.Subscribe("location", 0.1)
	return gaugesFor(st, 3), nil
}

func (s *sundial) Help() []string {
	return []string{
		"The dial faces true north; point the phone somewhere to look at it from that side, hold it flat to look down on it.",
		"The shadow shows the sun's time here. + and - speed time up or slow it down; [ and ] go a month back or ahead, to see the seasons.",
		"space  back to now",
	}
}

func (s *sundial) Key(k byte) {
	i := 0
	for i < len(lapseSpeeds) && lapseSpeeds[i] < s.speed {
		i++
	}
	switch k {
	case '+', '=':
		s.speed = lapseSpeeds[min(i+1, len(lapseSpeeds)-1)]
	case '-', '_':
		s.speed = lapseSpeeds[max(i-1, 0)]
	case ' ':
		s.speed, s.offset, s.simTime = 1, 0, time.Time{}
	case ']':
		s.offset += 30 * 24 * time.Hour
	case '[':
		s.offset -= 30 * 24 * time.Hour
	}
}

// sunAt is the direction toward the Sun at time tm (x east, y up, z
// south) and its hour angle in degrees (negative before noon).
func sunAt(tm time.Time, lat, lon float64) (Vec3, float64) {
	jd := julian(tm)
	ra, dec := eclipticToRADec(heliocentric(2, jd).Scale(-1))
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545), 360)
	lst := gmst + lon
	enu := skyVector(ra, dec, lat, lst)
	return Vec3{enu[0], enu[2], -enu[1]}, math.Remainder(lst-ra, 360)
}

// events finds the day's sunrise and sunset (the Sun's top at the
// horizon, refraction included) and its noon, in local time.
func (s *sundial) events(tm time.Time) {
	loc := tm.Location()
	d0 := time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, loc)
	s.day = d0.Format("2006-01-02")
	s.rise, s.noon, s.set = time.Time{}, time.Time{}, time.Time{}
	h0 := math.Sin(-0.833 * math.Pi / 180)
	alt := func(t time.Time) float64 { v, _ := sunAt(t, s.lat, s.lon); return v[1] - h0 }
	ha := func(t time.Time) float64 { _, h := sunAt(t, s.lat, s.lon); return h }
	bisect := func(a, b time.Time, f func(time.Time) float64) time.Time {
		fa := f(a)
		for i := 0; i < 20; i++ {
			m := a.Add(b.Sub(a) / 2)
			if fm := f(m); (fm > 0) == (fa > 0) {
				a, fa = m, fm
			} else {
				b = m
			}
		}
		return a.Add(b.Sub(a) / 2)
	}
	const step = 10 * time.Minute
	for t := d0; t.Before(d0.Add(24 * time.Hour)); t = t.Add(step) {
		n := t.Add(step)
		a0, a1 := alt(t), alt(n)
		if a0 <= 0 && a1 > 0 {
			s.rise = bisect(t, n, alt)
		}
		if a0 > 0 && a1 <= 0 {
			s.set = bisect(t, n, alt)
		}
		if h0, h1 := ha(t), ha(n); h0 < 0 && h1 >= 0 && h1-h0 < 90 {
			s.noon = bisect(t, n, ha)
		}
	}
}

// shadowDir is where the gnomon's shadow points for an hour angle (in
// degrees): along the plate, as east and toward-the-pole parts. This is
// the horizontal dial's rule, tan(angle) = sin(latitude) tan(hour angle).
func shadowDir(hourAngle, lat float64) (float64, float64) {
	h := hourAngle * math.Pi / 180
	th := math.Atan2(math.Sin(math.Abs(lat)*math.Pi/180)*math.Sin(h), math.Cos(h))
	return math.Sin(th), math.Cos(th)
}

// dialLat is the latitude the dial is built for: a horizontal dial's
// gnomon vanishes at the equator, so it's kept a little off it.
func dialLat(lat float64) float64 {
	if math.Abs(lat) < 8 {
		return math.Copysign(8, lat+1e-9)
	}
	return lat
}

// pole is the direction, along the ground, toward the raised celestial
// pole: north in the north, south in the south.
func (s *sundial) pole() Vec3 {
	if s.lat >= 0 {
		return Vec3{0, 0, -1}
	}
	return Vec3{0, 0, 1}
}

func (s *sundial) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	lr := ss.Get("location").Read()
	if !lr.OK || len(lr.V) < 2 {
		msg := "waiting for a location fix (the dial is laid out for where you are)..."
		if len(msg) > v.W {
			msg = "waiting for a location fix..."
		}
		v.Text(max(0, (v.W-len(msg))/2), v.H/2, msg, 244)
		return
	}
	s.lat, s.lon = lr.V[0], lr.V[1]
	decl := 0.0
	if d, ok := declination(ss); ok {
		decl = d
	}
	now := time.Now()
	if s.simTime.IsZero() {
		s.simTime, s.last = now, now
	}
	s.simTime = s.simTime.Add(time.Duration(float64(now.Sub(s.last)) * s.speed))
	s.last = now
	tm := s.simTime.Add(s.offset)
	s.sun, s.hourAng = sunAt(tm, s.lat, s.lon)
	if d := tm.Format("2006-01-02"); d != s.day {
		s.events(tm)
	}
	s.look(ss, decl, dt)
	area := v.H - 2
	s.render(v, area)
	s.caption(v, area, tm)
}

// look places the camera from the phone's orientation: its heading, the
// way its top (or, held up, its back) points, and from how high: flat, from
// above; upright, from lower.
func (s *sundial) look(ss *Streams, decl, dt float64) {
	az, el := 0.0, 55*math.Pi/180 // without it: looking north
	if r := ss.Get("rotation_vector").Read(); r.OK {
		if R, ok := FromRotationVector(r.V); ok {
			R = R.Mul(screenFrame(ss))
			top, back := R.Apply(Vec3{0, 1, 0}), R.Apply(Vec3{0, 0, -1})
			f := top.Add(back)
			if math.Hypot(f[0], f[1]) > 0.1 {
				az = math.Atan2(f[0], f[1]) + decl*math.Pi/180 // east of true north
			}
			up := R.Apply(Vec3{0, 0, 1})[2] // the screen's normal, upward
			el = (35 + 45*clamp01(up)) * math.Pi / 180
		}
	}
	if !s.hasView {
		s.az, s.el, s.hasView = az, el, true
	}
	k := math.Min(1, dt*5)
	s.az += math.Remainder(az-s.az, 2*math.Pi) * k
	s.el += (el - s.el) * k
}

// sdf is the scene's distance at p and its material: 1 the plate, 2 stone,
// 3 the gnomon.
func (s *sundial) sdf(p Vec3) (float64, int) {
	r := math.Sqrt(p[0]*p[0] + p[2]*p[2])
	cyl := func(R, y0, y1 float64) float64 {
		dx, dy := r-R, math.Max(y0-p[1], p[1]-y1)
		ox, oy := math.Max(dx, 0), math.Max(dy, 0)
		return math.Min(math.Max(dx, dy), 0) + math.Sqrt(ox*ox+oy*oy)
	}
	// The plate, its raised rim.
	plate := cyl(1, 0, dialTop) - 0.004
	rim := math.Max(cyl(1, dialTop-0.01, dialTop+0.03), -(r - 0.95))
	d, m := math.Min(plate, rim), 1
	// The pedestal: a round capital, a neck with a ring, the shaft.
	cap := cyl(1.18, dialSlab, -0.004) - 0.02
	neck := cyl(0.72, dialSlab-0.35, dialSlab)
	ring := len2(r-0.74, p[1]-(dialSlab-0.12)) - 0.07
	shaft := cyl(0.55, dialFloor, dialSlab-0.3)
	if st := math.Min(math.Min(cap, neck), math.Min(ring, shaft)); st < d {
		d, m = st, 2
	}
	// The gnomon, only when its box is nearer than what's found.
	P := s.pole()
	u := p[2]*P[2] + dialRoot
	bx := math.Max(math.Abs(p[0])-0.02, math.Max(math.Max(-u, u-s.gnomonLen), math.Max(dialTop-p[1], p[1]-dialTop-s.gnomonHt)))
	if bx < d {
		if g := s.gnomonSDF(p); g < d {
			d, m = g, 3
		}
	}
	return d, m
}

func len2(x, y float64) float64 { return math.Sqrt(x*x + y*y) }

// gnomonSDF is the gnomon: a triangular fin standing on the noon line, its
// top edge rising from the root at the latitude's angle, with a round
// cut-out.
func (s *sundial) gnomonSDF(p Vec3) float64 {
	P := s.pole()
	root := P.Scale(-dialRoot)
	u := p.Sub(root).Dot(P) // along the noon line, poleward
	w := p[1] - dialTop
	L, H := s.gnomonLen, s.gnomonHt
	// The triangle (0,0), (L,0), (L,H) in (u, w).
	var tri float64
	{
		n := Vec3{-H, L, 0}.Norm() // outward from the sloped edge
		e := math.Max(math.Max(-w, u-L), u*n[0]+w*n[1])
		tri = e
		if e > 0 { // outside: the true distance to the corners and edges
			d := math.Inf(1)
			seg := func(ax, ay, bx, by float64) {
				dx, dy := bx-ax, by-ay
				t := clamp01(((u-ax)*dx + (w-ay)*dy) / (dx*dx + dy*dy))
				d = math.Min(d, len2(u-ax-dx*t, w-ay-dy*t))
			}
			seg(0, 0, L, 0)
			seg(L, 0, L, H)
			seg(0, 0, L, H)
			tri = d
		}
	}
	// A round hole, for grace.
	hole := len2(u-L*0.7, w-H*0.28) - H*0.16
	tri = math.Max(tri, -hole)
	x := math.Abs(p[0]) - 0.018
	return math.Min(math.Max(tri, x), 0) + len2(math.Max(tri, 0), math.Max(x, 0))
}

func (s *sundial) normal(p Vec3) Vec3 {
	const e = 0.002
	var n Vec3
	for _, k := range [4]Vec3{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}} {
		d, _ := s.sdf(p.Add(k.Scale(e)))
		n = n.Add(k.Scale(d))
	}
	return n.Norm()
}

// shadowed is how much of the Sun p sees, softly, through the scene.
func (s *sundial) lit(p, n Vec3) float64 {
	S := s.sun
	if S[1] <= 0 {
		return 0
	}
	o := p.Add(n.Scale(0.006))
	res, t := 1.0, 0.01
	for i := 0; i < 48 && t < 8; i++ {
		q := o.Add(S.Scale(t))
		if q[1] > 1.6 {
			break // above everything
		}
		d, _ := s.sdf(q)
		if d < 0.0005 {
			return 0
		}
		res = math.Min(res, 14*d/t)
		t += math.Max(d, 0.004)
	}
	return clamp01(res)
}

func (s *sundial) camera(w, area int) (origin, F, Rt, U Vec3, tanX, tanY float64) {
	const dist = 6.5
	E := Vec3{-math.Sin(s.az) * math.Cos(s.el), math.Sin(s.el), math.Cos(s.az) * math.Cos(s.el)}
	target := Vec3{0, -0.15, 0}
	F = E.Scale(-1)
	Rt = F.Cross(Vec3{0, 1, 0}).Norm()
	U = Rt.Cross(F)
	origin = target.Add(E.Scale(dist))
	aspect := float64(w) / (2 * float64(area))
	base := 1.32 / dist
	tanX, tanY = base, base/aspect
	if aspect > 1 {
		tanX, tanY = base*aspect, base
	}
	return
}

func (s *sundial) render(v *View, area int) {
	w := v.W
	lat := dialLat(s.lat)
	s.gnomonLen = dialRoot + 0.62
	s.gnomonHt = s.gnomonLen * math.Tan(math.Abs(lat)*math.Pi/180)
	origin, F, Rt, U, tanX, tanY := s.camera(w, area)
	sunUp := smoothstep(-0.02, 0.06, s.sun[1])
	// Which cells show the plate's face, and how bright, for the
	// engraving drawn over it.
	face := make([]float64, w*area)
	parallelRows(area, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(area)*2
			dir := F.Add(Rt.Scale(u * tanX)).Add(U.Scale(vv * tanY)).Norm()
			ch, col, fb := s.shadeRay(origin, dir, sunUp)
			face[y*w+x] = fb
			if ch != 0 {
				v.Set(x, y, ch, col)
			}
		}
	})
	// The gnomon's outline, so it stands out from the plate behind it.
	for y := 0; y < area; y++ {
		for x := 0; x < w; x++ {
			if face[y*w+x] != -1 {
				continue
			}
			for _, o := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+o[0], y+o[1]
				if nx >= 0 && ny >= 0 && nx < w && ny < area && face[ny*w+nx] != -1 {
					v.Set(x, y, '#', dialBronze[1])
					break
				}
			}
		}
	}
	proj := func(p Vec3) (float64, float64) {
		d := p.Sub(origin)
		z := d.Dot(F)
		return (d.Dot(Rt)/z/tanX + 1) / 2 * float64(w), (1 - d.Dot(U)/z/tanY) / 2 * float64(area)
	}
	// Engrave a segment of the face: darker than the brass around it,
	// drawn along its run.
	engrave := func(a, b Vec3) {
		ax, ay := proj(a)
		bx, by := proj(b)
		c := lineGlyph(bx-ax, by-ay)
		n := int(math.Max(math.Abs(bx-ax), math.Abs(by-ay))) + 1
		for i := 0; i <= n; i++ {
			f := float64(i) / float64(n)
			x, y := int(ax+(bx-ax)*f), int(ay+(by-ay)*f)
			if x < 0 || y < 0 || x >= w || y >= area {
				continue
			}
			if fb := face[y*w+x]; fb > 0 {
				v.Set(x, y, c, mzPick(artBrassRamp, fb*0.4))
			}
		}
	}
	P := s.pole()
	E := Vec3{1, 0, 0}
	root := P.Scale(-dialRoot)
	root[1] = dialTop
	// Where a line from the root, along dir, meets a circle about the
	// middle.
	meet := func(dir Vec3, r float64) (Vec3, bool) {
		b := root[0]*dir[0] + root[2]*dir[2]
		c := root[0]*root[0] + root[2]*root[2] - r*r
		disc := b*b - c
		if disc < 0 {
			return Vec3{}, false
		}
		return root.Add(dir.Scale(-b + math.Sqrt(disc))), true
	}
	// The chapter rings.
	for _, r := range []float64{0.74, 0.9} {
		const n = 160
		for i := 0; i < n; i++ {
			a0, a1 := float64(i)/n*2*math.Pi, float64(i+1)/n*2*math.Pi
			engrave(Vec3{r * math.Cos(a0), dialTop, r * math.Sin(a0)}, Vec3{r * math.Cos(a1), dialTop, r * math.Sin(a1)})
		}
	}
	// The hours from the root out to the rings, half hours between them,
	// and the hours' numbers.
	for hh := 10; hh <= 38; hh++ { // 5:00 to 19:00, as long as the sun can be up
		sx, sp := shadowDir(float64(hh-24)*7.5, lat)
		dir := E.Scale(sx).Add(P.Scale(sp))
		outer, ok := meet(dir, 0.9)
		if !ok {
			continue
		}
		if hh%2 == 1 {
			if inner, ok := meet(dir, 0.74); ok {
				engrave(inner, outer)
			}
			continue
		}
		engrave(root.Add(dir.Scale(0.14)), outer)
		// The number, between the rings, if the face shows there.
		q, ok := meet(dir, 0.82)
		if !ok || -F[1] < 0.3 {
			continue
		}
		px, py := proj(q)
		label := fmt.Sprint(hh / 2)
		x0 := int(px) - len(label)/2
		show := int(py) >= 0 && int(py) < area
		for i := range label {
			if show && (x0+i < 0 || x0+i >= w || face[int(py)*w+x0+i] <= 0) {
				show = false
			}
		}
		if show {
			fb := face[int(py)*w+int(px)]
			v.Text(x0, int(py), label, mzPick(artBrassRamp, fb*0.3))
		}
	}
}

// march finds the first surface along a ray, up to maxT.
func (s *sundial) march(o, d Vec3, maxT float64) (float64, int) {
	t0, _, ok := cylSpan(o, d, 0, 0, 1.3, dialFloor, 1.5)
	if !ok {
		return math.Inf(1), 0
	}
	t := t0 // start near the dial: the camera is far off
	for i := 0; i < 120 && t < maxT; i++ {
		p := o.Add(d.Scale(t))
		if p[1] < dialFloor-0.01 {
			break
		}
		dd, m := s.sdf(p)
		if dd < 0.001 {
			return t, m
		}
		t += dd
	}
	return math.Inf(1), 0
}

// shadeRay shades what a ray from the camera meets, and for the plate's
// face, returns how bright it is there (else 0).
func (s *sundial) shadeRay(o, d Vec3, sunUp float64) (byte, uint8, float64) {
	const solid = "=+#%@"
	S := s.sun
	t, m := s.march(o, d, 40)
	if m == 0 {
		// The lawn, at the pedestal's foot.
		if d[1] >= 0 {
			return 0, 0, 0
		}
		tg := (dialFloor - o[1]) / d[1]
		p := o.Add(d.Scale(tg))
		li := s.lit(p, Vec3{0, 1, 0}) * sunUp
		g := noise3(p[0]*3, p[2]*3, 1)
		b := (0.18 + 0.45*li*math.Max(0, S[1])) * (0.8 + 0.4*g)
		chars := `.,'"`
		return chars[int(clamp01(g)*3.99)], mzPick(dialGrass, b), 0
	}
	p := o.Add(d.Scale(t))
	n := s.normal(p)
	li := s.lit(p, n) * sunUp
	diff := math.Max(0, n.Dot(S)) * li
	spec := math.Pow(math.Max(0, n.Dot(S.Sub(d).Norm())), 40) * li
	amb := 0.1 + 0.05*sunUp
	switch m {
	case 1: // brass
		i := (amb + 0.5*diff + 0.3*spec) * 0.9
		fb := 0.0
		if n[1] > 0.9 && math.Hypot(p[0], p[2]) < 0.94 {
			fb = math.Max(i, 1e-3)
		}
		return solid[int(clamp01(i)*4+0.5)], mzPick(artBrassRamp, i), fb
	case 3: // the gnomon: bronze, darker than the plate
		i := amb + 0.6*diff + 0.4*spec
		return solid[int(clamp01(i)*4+0.5)], mzPick(dialBronze, i), -1
	}
	// Stone, grained.
	g := noise3(p[0]*6, p[1]*6, p[2]*6)
	i := (amb + 0.65*diff) * (0.85 + 0.3*g)
	return solid[int(clamp01(i)*4+0.5)], mzPick(dialStone, i), 0
}

func (s *sundial) caption(v *View, area int, tm time.Time) {
	solar := 12 + s.hourAng/15
	solar = math.Mod(solar+24, 24)
	sh, sm := int(solar), int(math.Mod(solar*60, 60))
	alt := math.Asin(math.Max(-1, math.Min(1, s.sun[1]))) * 180 / math.Pi
	var l1 string
	if alt > -0.833 {
		l1 = fmt.Sprintf("sun time %02d:%02d   clock %s   sun %.0f° up", sh, sm, tm.Format("15:04"), alt)
		if len(l1) > v.W {
			l1 = fmt.Sprintf("sun %02d:%02d  clock %s", sh, sm, tm.Format("15:04"))
		}
	} else {
		l1 = fmt.Sprintf("the sun is down   clock %s", tm.Format("15:04"))
	}
	if s.speed > 1 {
		l1 += fmt.Sprintf("  x%.0f", s.speed)
	}
	if s.offset != 0 {
		l1 += "  " + tm.Format("Jan 2")
	}
	hm := func(t time.Time) string {
		if t.IsZero() {
			return "--:--"
		}
		return t.Format("15:04")
	}
	l2 := fmt.Sprintf("sunrise %s   noon %s   sunset %s", hm(s.rise), hm(s.noon), hm(s.set))
	if len(l2) > v.W {
		l2 = fmt.Sprintf("up %s  noon %s  down %s", hm(s.rise), hm(s.noon), hm(s.set))
	}
	v.Text(max(0, (v.W-len(l1))/2), area, l1, 220)
	v.Text(max(0, (v.W-len(l2))/2), area+1, l2, 244)
}
