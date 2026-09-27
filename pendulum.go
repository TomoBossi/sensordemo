package main

import (
	"fmt"
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "pendulum",
		desc: "a sand pendulum drawing harmonograph figures in a round tray; move the phone to push it",
		uses: []string{"linear_acceleration"},
		new:  func(specs []string) Demo { return &pendulum{} },
	})
}

// pendulum is a funnel of sand on a long cord over a round tray, seen from
// above at a slant. It swings with one period across and another along
// (a Blackburn pendulum: the cord splits in a Y below the ceiling), so its
// path is a Lissajous figure that slowly turns as the two drift apart, and
// shrinks as the swing dies down. Sand leaks from the funnel's tip and
// builds up along the path: ridges where it passed, heaps at the turns
// where it lingered, sand sliding down wherever it piles steeper than sand
// can stand. A lamp above lights the relief.
//
// Moving the phone moves the tray and the cord's anchor with it; the
// funnel lags and swings. A hard shake levels the sand.
//
// Scene units: the tray's radius is 1, y is up, the sand's floor at y = 0.
type pendulum struct {
	rng    *rand.Rand
	h      []float64  // sand height on the grid
	p, v   [2]float64 // the funnel's offset from rest (x, z), and its velocity
	ratio  int        // index into pendRatios
	sand   float64    // what's left in the funnel, 0..1
	shake  float64
	stream float64    // how hard sand is falling, eased
	acc    [2]float64 // the tray's acceleration, smoothed

	// Where sand fell since it last settled, in grid cells.
	dirty              bool
	di0, di1, dj0, dj1 int
}

const (
	pendN      = 256  // the sand grid, across the tray
	pendExtent = 1.02 // the grid covers -pendExtent..pendExtent
	pendCell   = 2 * pendExtent / pendN
	pendWall   = 0.12 // the tray's inner wall
	pendRim    = 0.1  // the rim's width
	pendTip    = 0.24 // the funnel's tip above the floor
	pendCone   = 0.34 // the funnel's height
	pendMouth  = 0.17 // its radius at the top
	pendCord   = 7.5  // from the funnel up to the anchor
	pendSigma  = 0.014
	pendFlow   = 0.0006 // sand per second, in scene volume
	pendRepose = 0.62   // the steepest slope sand holds
)

var pendRatios = []struct {
	name string
	r    float64
}{
	{"2:3", 1.5}, {"3:4", 4.0 / 3}, {"1:2", 2}, {"1:1", 1}, {"3:5", 5.0 / 3}, {"4:5", 1.25},
}

var (
	pendSlate = []uint8{232, 233, 234, 235, 236, 237}
	pendSand  = []uint8{235, 237, 239, 137, 180, 223, 230}
	pendLight = Vec3{-0.5, 0.8, -0.35}.Norm()
)

func (s *pendulum) Setup(ss *Streams) ([]*Gauge, error) {
	s.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("linear_acceleration", 60)
	if err != nil {
		return nil, err
	}
	s.h = make([]float64, pendN*pendN)
	s.sand = 1
	s.swing()
	// Open on a figure already begun.
	for i := 0; i < 6*240; i++ {
		s.step(1.0/240, [2]float64{})
	}
	s.relaxAll()
	return gaugesFor(st, 3), nil
}

func (s *pendulum) Help() []string {
	return []string{
		"Move the phone to push the pendulum: the tray moves, the funnel lags and swings. Shake hard to level the sand.",
		"space  a fresh swing    f  next ratio    s  smooth the sand and refill",
	}
}

func (s *pendulum) Key(k byte) {
	switch k {
	case ' ':
		s.swing()
	case 'f':
		s.ratio = (s.ratio + 1) % len(pendRatios)
		s.swing()
	case 's':
		clear(s.h)
		s.sand = 1
	}
}

// swing starts a wide swing, out to one side and moving along, the two
// swings in some phase: each phase draws another figure.
func (s *pendulum) swing() {
	_, wz := s.omegas()
	ax, az := 0.8+0.05*s.rng.Float64(), 0.7+0.1*s.rng.Float64()
	side := float64(1 - 2*s.rng.Intn(2))
	ph := (0.1 + 0.8*s.rng.Float64()) * math.Pi
	s.p = [2]float64{side * ax, az * math.Cos(ph)}
	s.v = [2]float64{0, az * wz * math.Sin(ph)}
	s.fit(0.84)
}

// fit scales the swing so that, as it will go, it never reaches further
// out than reach: a Lissajous figure's corners come out further than
// either swing alone, and the funnel would hit the wall.
func (s *pendulum) fit(reach float64) {
	for try := 0; try < 8; try++ { // friction isn't proportional: iterate
		p, v := s.p, s.v
		far := 0.0
		for i := 0; i < 240*40; i++ {
			s.move(1.0/240, [2]float64{})
			far = math.Max(far, math.Hypot(s.p[0], s.p[1]))
		}
		s.p, s.v = p, v
		if far <= reach {
			return
		}
		k := reach / far * 0.995
		s.p = [2]float64{p[0] * k, p[1] * k}
		s.v = [2]float64{v[0] * k, v[1] * k}
	}
}

func (s *pendulum) omegas() (float64, float64) {
	wx := 2 * math.Pi / 2.4
	return wx, wx * pendRatios[s.ratio].r * 1.006 // a touch off: the figure turns
}

// step moves the funnel under the push and drops its sand.
func (s *pendulum) step(dt float64, push [2]float64) {
	s.move(dt, push)
	// The funnel closes as the swing dies away (a finger over its tip),
	// or the middle would fill with sand.
	if open := smoothstep(0.1, 0.22, s.swingSize()); s.sand > 0 && open > 0 {
		s.sand = math.Max(0, s.sand-dt*open/180)
		s.deposit(s.p[0], s.p[1], pendFlow*dt*open)
	}
}

// move swings the funnel under the push (the tray's acceleration, which
// it lags).
func (s *pendulum) move(dt float64, push [2]float64) {
	wx, wz := s.omegas()
	// Friction where the cords hang, nearly constant, so the swing shrinks
	// by the same step each time round and the lines lie evenly spaced;
	// stronger for the quicker swing, so both shrink alike and the figure
	// keeps its shape. And a little drag.
	const fric, drag = 0.13, 0.01
	w := [2]float64{wx, wz}
	for k := 0; k < 2; k++ {
		f := -drag*s.v[k] - fric*w[k]/wx*s.v[k]/math.Max(math.Abs(s.v[k]), 0.05)
		s.v[k] += (-w[k]*w[k]*s.p[k] + f - push[k]) * dt
		s.p[k] += s.v[k] * dt
	}
	// Pushed too far, the funnel meets the tray's wall: softly, a stiff
	// cushion turning it back, not a bounce.
	if r := math.Hypot(s.p[0], s.p[1]); r > 0.86 {
		nx, nz := s.p[0]/r, s.p[1]/r
		in := (r - 0.86) * 400
		if vn := s.v[0]*nx + s.v[1]*nz; vn > 0 {
			in += vn * 25
		}
		s.v[0] -= nx * in * dt
		s.v[1] -= nz * in * dt
	}
}

// swingSize is how wide the funnel swings, from its energy.
func (s *pendulum) swingSize() float64 {
	wx, wz := s.omegas()
	ex := s.p[0]*s.p[0] + s.v[0]*s.v[0]/(wx*wx)
	ez := s.p[1]*s.p[1] + s.v[1]*s.v[1]/(wz*wz)
	return math.Sqrt(math.Max(ex, ez))
}

// deposit drops amt of sand around (x, z), spread like the falling stream,
// and lets it settle nearby.
func (s *pendulum) deposit(x, z, amt float64) {
	gi := (x + pendExtent) / pendCell
	gj := (z + pendExtent) / pendCell
	sg := pendSigma / pendCell
	rad := int(sg*2.5) + 1
	i0, j0 := int(gi), int(gj)
	var wsum float64
	var wts [16][16]float64
	for dj := -rad; dj <= rad; dj++ {
		for di := -rad; di <= rad; di++ {
			fx, fz := float64(i0+di)+0.5-gi, float64(j0+dj)+0.5-gj
			wv := math.Exp(-(fx*fx + fz*fz) / (2 * sg * sg))
			wts[dj+rad][di+rad] = wv
			wsum += wv
		}
	}
	per := amt / (pendCell * pendCell) / wsum
	for dj := -rad; dj <= rad; dj++ {
		for di := -rad; di <= rad; di++ {
			i, j := i0+di, j0+dj
			if i >= 0 && j >= 0 && i < pendN && j < pendN {
				s.h[j*pendN+i] += per * wts[dj+rad][di+rad]
			}
		}
	}
	s.relax(i0-rad-3, i0+rad+3, j0-rad-3, j0+rad+3, 2)
	if !s.dirty {
		s.di0, s.di1, s.dj0, s.dj1, s.dirty = i0, i0, j0, j0, true
	}
	s.di0, s.di1 = min(s.di0, i0), max(s.di1, i0)
	s.dj0, s.dj1 = min(s.dj0, j0), max(s.dj1, j0)
}

// settle lets the sand slide where it fell lately, and around it (a
// heap's slope spreads a cell a pass), rather than over the whole tray.
func (s *pendulum) settle() {
	if !s.dirty {
		return
	}
	const m = 24
	s.relax(s.di0-m, s.di1+m, s.dj0-m, s.dj1+m, 1)
	s.dirty = false
}

// inTray reports whether grid cell i, j lies on the tray's floor.
func inTray(i, j int) bool {
	x := (float64(i)+0.5)*pendCell - pendExtent
	z := (float64(j)+0.5)*pendCell - pendExtent
	return x*x+z*z < 1
}

// relax lets sand slide wherever neighbors differ by more than it can
// hold, over the grid box i0..i1, j0..j1.
func (s *pendulum) relax(i0, i1, j0, j1, passes int) {
	i0, j0 = max(0, i0), max(0, j0)
	i1, j1 = min(pendN-2, i1), min(pendN-2, j1)
	const hold = pendRepose * pendCell
	h := s.h
	for pass := 0; pass < passes; pass++ {
		for j := j0; j <= j1; j++ {
			for i := i0; i <= i1; i++ {
				a := j*pendN + i
				for _, b := range [2]int{a + 1, a + pendN} {
					d := h[a] - h[b]
					if d > hold || d < -hold {
						if !inTray(i, j) || !inTray(b%pendN, b/pendN) {
							continue
						}
						m := (d - math.Copysign(hold, d)) * 0.4
						h[a] -= m
						h[b] += m
					}
				}
			}
		}
	}
}

func (s *pendulum) relaxAll() { s.relax(0, pendN, 0, pendN, 1) }

// level spreads the sand out, as a shaken tray does: each cell eases toward
// its neighbors' mean, sand kept within the tray.
func (s *pendulum) level(k float64) {
	h := s.h
	out := make([]float64, len(h))
	copy(out, h)
	for j := 1; j < pendN-1; j++ {
		for i := 1; i < pendN-1; i++ {
			if !inTray(i, j) {
				continue
			}
			a := j*pendN + i
			for _, b := range [4]int{a - 1, a + 1, a - pendN, a + pendN} {
				if inTray(b%pendN, b/pendN) {
					m := (h[b] - h[a]) * k * 0.25
					out[a] += m
				}
			}
		}
	}
	copy(h, out)
}

func (s *pendulum) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	dt = math.Min(dt, 0.1)
	// The tray's acceleration, in its own plane: screen x is across, screen
	// y (up the screen) is away, which is -z. The funnel feels the opposite.
	var push [2]float64
	if st := ss.Get("linear_acceleration"); st != nil {
		if r := st.Read(); r.OK && len(r.V) >= 3 {
			a := toScreen(ss, r.V)
			// Smoothed a little: the sensor's jitter, and the jolts it
			// reads when the phone turns, aren't pushes.
			k := math.Min(1, dt/0.06)
			s.acc[0] += (a[0] - s.acc[0]) * k
			s.acc[1] += (a[1] - s.acc[1]) * k
			m := math.Hypot(s.acc[0], s.acc[1])
			if m > 0.45 { // below that, it's the hand's tremor
				g := (m - 0.45) / m * 5 // metres to tray radii (about 20 cm)
				push = [2]float64{s.acc[0] * g, -s.acc[1] * g}
			}
			if raw := math.Hypot(a[0], a[1]); raw > 12 {
				s.shake += (raw - 12) * dt
			}
		}
	}
	const sub = 8
	for i := 0; i < sub; i++ {
		s.step(dt/sub, push)
	}
	s.settle()
	// A push is brief; a shake goes on, and levels the sand.
	if s.shake > 0.2 {
		s.level(math.Min(0.8, s.shake-0.2))
		s.relaxAll()
	}
	s.shake = math.Max(0, s.shake-dt)
	flow := 0.0
	if s.sand > 0 {
		flow = smoothstep(0.1, 0.22, s.swingSize())
	}
	s.stream += (flow - s.stream) * math.Min(1, dt*6)
	area := v.H - 1
	s.render(v, area, t)
	line := pendRatios[s.ratio].name + fmt.Sprintf("  sand %d%%", int(s.sand*100+0.5))
	if s.sand == 0 {
		line = pendRatios[s.ratio].name + "  the funnel is empty: s to refill"
	}
	v.Text(max(0, (v.W-len(line))/2), area, line, 244)
}

// heightAt reads the sand's height at x, z, and its slope.
func (s *pendulum) heightAt(x, z float64) (float64, float64, float64) {
	gi := (x+pendExtent)/pendCell - 0.5
	gj := (z+pendExtent)/pendCell - 0.5
	i, j := int(math.Floor(gi)), int(math.Floor(gj))
	if i < 1 || j < 1 || i >= pendN-2 || j >= pendN-2 {
		return 0, 0, 0
	}
	fx, fz := gi-float64(i), gj-float64(j)
	h := s.h
	at := func(i, j int) float64 { return h[j*pendN+i] }
	bil := func(i, j int) float64 {
		return (at(i, j)*(1-fx)+at(i+1, j)*fx)*(1-fz) + (at(i, j+1)*(1-fx)+at(i+1, j+1)*fx)*fz
	}
	c := bil(i, j)
	dx := (bil(i+1, j) - bil(i-1, j)) / (2 * pendCell)
	dz := (bil(i, j+1) - bil(i, j-1)) / (2 * pendCell)
	return c, dx, dz
}

// funnelSDF is the funnel, hollow, around its own axis: local y from its
// tip up. It returns the distance and whether p is on the sand inside.
func (s *pendulum) funnelSDF(q Vec3) (float64, bool) {
	r := math.Hypot(q[0], q[2])
	// A cone from the tip's small hole to the mouth.
	slope := (pendMouth - 0.012) / pendCone
	edge := (r - (0.012 + slope*q[1])) / math.Sqrt(1+slope*slope)
	outer := math.Max(edge, math.Max(-q[1], q[1]-pendCone))
	shell := math.Max(outer, -(edge + 0.012))
	shell = math.Min(shell, math.Max(math.Abs(q[1]-pendCone)-0.012, math.Abs(r-pendMouth-0.004)-0.012)) // the rolled lip
	// Sand inside, level with what's left.
	lvl := 0.02 + (pendCone-0.06)*s.sand
	in := math.Max(edge+0.012, q[1]-lvl)
	if s.sand > 0 && in < shell {
		return in, true
	}
	return shell, false
}

// pendView is one frame's camera and the shadows on the sand, shared by
// the rows as they render.
type pendView struct {
	origin, F, Rt, U Vec3
	tanX, tanY       float64
	el               float64
	colW, rowW       float64 // how much floor a cell spans, across and front to back
	bob              Vec3
	fsx, fsz         float64 // the funnel's shadow
	csx, csz         float64 // where the cord's shadow starts
	cux, cuz         float64 // and which way it runs
}

func (s *pendulum) render(v *View, area int, t float64) {
	w := v.W
	var pv pendView
	pv.el = 72 * math.Pi / 180
	E := Vec3{0, math.Sin(pv.el), math.Cos(pv.el)}
	const dist = 7.0
	aspect := float64(w) / (2 * float64(area))
	base := 1.18 / dist
	pv.tanX, pv.tanY = base, base/aspect
	if aspect > 1 {
		pv.tanX, pv.tanY = base*aspect, base
	}
	// On a tall screen, the tray sits low and the cord rises above it.
	lift := math.Max(0, pv.tanY*dist-1.2) * 0.8
	target := Vec3{0, lift * math.Cos(pv.el), -lift * math.Sin(pv.el)}
	pv.F = E.Scale(-1)
	pv.Rt = pv.F.Cross(Vec3{0, 1, 0}).Norm()
	pv.U = pv.Rt.Cross(pv.F)
	pv.origin = target.Add(E.Scale(dist))
	pv.colW = 2 * pv.tanX * dist / float64(w)
	pv.rowW = 2 * pv.tanY * dist / float64(area) / math.Sin(pv.el)
	L := pendLight

	pv.bob = Vec3{s.p[0], pendTip, s.p[1]}
	shadowOf := func(p Vec3) (float64, float64) {
		k := p[1] / L[1]
		return p[0] - L[0]*k, p[2] - L[2]*k
	}
	pv.fsx, pv.fsz = shadowOf(pv.bob.Add(Vec3{0, pendCone * 0.6, 0}))
	top := pv.bob.Add(Vec3{0, pendCone + 0.3, 0})
	pv.csx, pv.csz = shadowOf(top)
	cl := math.Hypot(L[0], L[2])
	pv.cux, pv.cuz = -L[0]/cl, -L[2]/cl

	parallelRows(area, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(area)*2
			dir := pv.F.Add(pv.Rt.Scale(u * pv.tanX)).Add(pv.U.Scale(vv * pv.tanY)).Norm()
			if ch, col := s.shadeRay(&pv, dir, x, y); ch != 0 {
				v.Set(x, y, ch, col)
			}
		}
	})
	// The cords, and the falling sand, drawn over the scene.
	proj := func(p Vec3) (float64, float64, bool) {
		d := p.Sub(pv.origin)
		z := d.Dot(pv.F)
		if z <= 0 {
			return 0, 0, false
		}
		return (d.Dot(pv.Rt)/z/pv.tanX + 1) / 2 * float64(w), (1 - d.Dot(pv.U)/z/pv.tanY) / 2 * float64(area), true
	}
	line := func(a, b Vec3, col uint8, dotted bool) {
		ax, ay, ok1 := proj(a)
		bx, by, ok2 := proj(b)
		if !ok1 || !ok2 {
			return
		}
		c := lineGlyph(bx-ax, by-ay)
		n := int(math.Max(math.Abs(bx-ax), math.Abs(by-ay))) + 1
		for i := 0; i <= n; i++ {
			f := float64(i) / float64(n)
			px, py := ax+(bx-ax)*f, ay+(by-ay)*f
			if py < 0 || int(py) >= area {
				continue
			}
			if dotted {
				if s.rng.Float64() > 0.6*s.stream {
					continue
				}
				v.Set(int(px), int(py), ".:'"[s.rng.Intn(3)], mzPick(pendSand, 0.75+0.25*s.rng.Float64()))
				continue
			}
			v.Set(int(px), int(py), c, col)
		}
	}
	bob := pv.bob
	// Three short cords from the lip to a knot, then one up to the Y.
	for k := 0; k < 3; k++ {
		a := float64(k)*2*math.Pi/3 + 0.5
		lip := bob.Add(Vec3{math.Cos(a) * pendMouth, pendCone, math.Sin(a) * pendMouth})
		line(lip, top, 250, false)
	}
	anchor := Vec3{0, pendTip + pendCone + pendCord, 0}
	line(top, top.Add(anchor.Sub(top).Norm().Scale(6)), 248, false)
	// The stream, from the tip down to the sand.
	if s.stream > 0.05 {
		h, _, _ := s.heightAt(bob[0], bob[2])
		line(bob, Vec3{bob[0], h, bob[2]}, 0, true)
	}
}

// lineGlyph is the character for a line running dx across, dy down (in
// cells, which are twice as tall as wide).
func lineGlyph(dx, dy float64) byte {
	a := math.Atan2(dy*2, dx) // in square units
	if a < 0 {
		a += math.Pi
	}
	switch {
	case a < math.Pi/8 || a > 7*math.Pi/8:
		return '-'
	case a < 3*math.Pi/8:
		return '\\'
	case a < 5*math.Pi/8:
		return '|'
	}
	return '/'
}

// shadeRay finds what a ray from the camera meets first, and shades it:
// the funnel, the tray's rim and walls, the sand, or the table.
func (s *pendulum) shadeRay(pv *pendView, d Vec3, px, py int) (byte, uint8) {
	const solid = "=+#%@"
	o, L := pv.origin, pendLight
	// The funnel, marched within a sphere around it.
	fc := pv.bob.Add(Vec3{0, pendCone / 2, 0})
	rel := fc.Sub(o)
	along := rel.Dot(d)
	if miss := rel.Dot(rel) - along*along; miss < 0.25*0.25 {
		tt := along - 0.25
		for i := 0; i < 48; i++ {
			q := o.Add(d.Scale(tt)).Sub(pv.bob)
			dd, sandy := s.funnelSDF(q)
			if dd < 0.002 {
				if sandy {
					return '#', mzPick(pendSand, 0.45+0.4*s.sand)
				}
				n := s.funnelNormal(q)
				diff := math.Max(0, n.Dot(L))
				spec := math.Pow(math.Max(0, n.Dot(L.Sub(d).Norm())), 24)
				i := 0.12 + 0.6*diff + 0.5*spec
				return solid[int(clamp01(i)*4+0.5)], mzPick(artBrassRamp, i)
			}
			tt += dd
			if tt > along+0.25 {
				break
			}
		}
	}
	best, what := math.Inf(1), 0
	var bn Vec3
	try := func(tt float64, n Vec3, kind int, ok func(p Vec3) bool) {
		if tt > 0 && tt < best {
			if ok(o.Add(d.Scale(tt))) {
				best, bn, what = tt, n, kind
			}
		}
	}
	// The floor (the sand is shaded on it: its relief is small).
	try(-o[1]/d[1], Vec3{0, 1, 0}, 1, func(p Vec3) bool { return p[0]*p[0]+p[2]*p[2] < 1 })
	// The rim's top.
	try((pendWall-o[1])/d[1], Vec3{0, 1, 0}, 2, func(p Vec3) bool {
		r2 := p[0]*p[0] + p[2]*p[2]
		return r2 >= 1 && r2 < (1+pendRim)*(1+pendRim)
	})
	// The inner wall (seen from inside: the far root) and the outer wall.
	if _, t1, ok := cylSpan(o, d, 0, 0, 1, 0, pendWall); ok {
		p := o.Add(d.Scale(t1))
		try(t1, Vec3{-p[0], 0, -p[2]}, 3, func(Vec3) bool { return true })
	}
	if t0, _, ok := cylSpan(o, d, 0, 0, 1+pendRim, -0.06, pendWall); ok {
		p := o.Add(d.Scale(t0))
		if math.Abs(math.Hypot(p[0], p[2])-(1+pendRim)) < 1e-3 {
			try(t0, Vec3{p[0], 0, p[2]}.Scale(1/(1+pendRim)), 3, func(Vec3) bool { return true })
		}
	}
	// The table.
	try((-0.06-o[1])/d[1], Vec3{0, 1, 0}, 4, func(p Vec3) bool { return true })
	p := o.Add(d.Scale(best))
	switch what {
	case 1:
		return s.shadeSand(pv, p, px, py)
	case 2, 3:
		diff := math.Max(0, bn.Dot(L))
		i := 0.12 + 0.7*diff
		if what == 2 {
			i += 0.08 + 0.1*noise3(math.Atan2(p[2], p[0])*6, 0, 0)
		}
		return solid[int(clamp01(i)*4+0.5)], mzPick(artWoodRamp, i)
	case 4:
		// A dark table, lit a little around the tray.
		r := math.Hypot(p[0], p[2])
		g := 0.35 / (1 + (r-1)*(r-1)*3)
		if (px+py)%2 == 0 && g > 0.08 {
			return '.', mzPick(artEndRamp, g)
		}
	}
	return 0, 0
}

func (s *pendulum) funnelNormal(q Vec3) Vec3 {
	const e = 0.002
	var n Vec3
	for _, k := range [4]Vec3{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}} {
		d, _ := s.funnelSDF(q.Add(k.Scale(e)))
		n = n.Add(k.Scale(d))
	}
	return n.Norm()
}

// shadeSand lights the sand at p on the floor: its relief under the lamp,
// the funnel's and cord's shadows; bare floor where there's none. A cell
// covers a stretch of floor, so it samples a few points over it, not to
// lose thin lines between rows; and where a thin line of sand runs through
// the cell, it's drawn with a character along it.
func (s *pendulum) shadeSand(pv *pendView, p Vec3, px, py int) (byte, uint8) {
	var cover, lit, peak float64
	var txx, txz, tzz float64 // the slopes' structure: which way lines run
	for k := -1; k <= 1; k++ {
		x, z := p[0], p[2]+float64(k)*pv.rowW/3
		h, dx, dz := s.heightAt(x, z)
		c := smoothstep(0.003, 0.006, h)
		cover = math.Max(cover, c)
		peak = math.Max(peak, h)
		n := Vec3{-dx, 1, -dz}.Norm()
		lit += math.Max(0, n.Dot(pendLight)) * c
		for _, off := range [2]float64{-0.4, 0.4} {
			_, gx, gz := s.heightAt(x+off*pv.colW, z)
			txx += gx * gx
			txz += gx * gz
			tzz += gz * gz
		}
		txx += dx * dx
		txz += dx * dz
		tzz += dz * dz
	}
	if cover > 0 {
		lit /= 3 * math.Max(cover, 1e-9)
	}
	// The lamp's pool of light, and shadows.
	r2 := p[0]*p[0] + p[2]*p[2]
	lamp := 1 - 0.3*r2
	shade := 1.0
	if dx, dz := p[0]-pv.fsx, p[2]-pv.fsz; dx*dx+dz*dz < 0.13*0.13 {
		shade = 0.4
	}
	if rx, rz := p[0]-pv.csx, p[2]-pv.csz; rx*pv.cux+rz*pv.cuz > 0 && math.Abs(rx*pv.cuz-rz*pv.cux) < 0.012 {
		shade = math.Min(shade, 0.55)
	}
	if cover < 0.3 {
		// Bare slate, faintly grained.
		g := (0.3 + 0.25*noise3(p[0]*9, p[2]*9, 0)) * lamp * shade
		ch := byte('.')
		if (px*7+py*3)%5 == 0 {
			ch = ':'
		}
		return ch, mzPick(pendSlate, g)
	}
	// Thin sand is sparse grains on the dark; heaped, it's bright.
	heap := smoothstep(0.012, 0.04, peak)
	i := (0.3 + 0.55*lit*(0.75+0.25*heap)) * lamp * shade
	col := mzPick(pendSand, i+0.15*heap)
	// A line: slopes mostly across it, and not a heap.
	tr, det := txx+tzz, txx*tzz-txz*txz
	disc := math.Sqrt(math.Max(0, tr*tr/4-det))
	l1, l2 := tr/2+disc, tr/2-disc
	if heap < 0.5 && l1 > 1e-6 && (l1-l2)/(l1+l2) > 0.6 {
		// The slope's main direction, and the line across it, on screen.
		gx, gz := txz, l1-txx
		if math.Abs(gx)+math.Abs(gz) < 1e-12 {
			gx, gz = 1, 0
		}
		return lineGlyph(-gz/pv.colW, gx/pv.rowW), col
	}
	return "=+#%@"[int(clamp01(i)*4+0.5)], col
}
