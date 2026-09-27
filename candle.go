package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "candle",
		desc: "a 3D candle, lit by its own flame; turn the phone to look around it, cover the top to snuff it",
		uses: []string{"game_rotation_vector", "linear_acceleration", "proximity"},
		new:  func(specs []string) Demo { return &candle{} },
	})
}

// candle is a burning candle in 3D, ray marched from distance functions
// and framed on its top: the flame, and the wax that burns around it.
// The rim stands uneven where the wax burned down around a hollow; in the
// hollow lies a pool of liquid wax that mirrors the flame; dried drips run
// down the sides and end in beads. The wax is lit by the flame itself, a
// point light, and glows from within near the top and through the thin
// rim, as wax does. The flame is a volume: each ray gathers its light, a
// blue root, a white core, orange, red tips, unless the wax is in front.
//
// Turning the phone swings the view around the candle, as in the eye:
// within a range, easing into its limits. Moving the phone makes the
// flame lag and sway; a shake makes it gutter. Covering the proximity
// sensor snuffs it, leaving a curl of smoke and a fading ember;
// uncovering lights it again.
//
// Scene units: the candle's radius is 1, y is up its axis, its rim near
// y = 0.
type candle struct {
	drips   []wdrip
	rng     *rand.Rand
	sway    [2]float64 // the flame tip's offset in x and z
	swayV   [2]float64
	power   float64 // 0 out .. 1 burning
	lit     bool
	covered float64
	gutter  float64
	ember   float64
	melt    float64 // the pool: 1 liquid .. 0 set
	smoke   []puff

	ref        Mat3
	hasRef     bool
	firstCount int
	eye        Vec3 // toward the camera from the target, smoothed

	H, t float64 // this frame's flame height, and time

	// The wax's irregularity, precomputed: the column's radius by angle
	// and height, and the rim's height by angle.
	radiusT [candleTA * candleTY]float64
	rimT    [candleTA]float64
}

const (
	candleTA   = 128 // angles in the tables
	candleTY   = 64  // heights
	candleYMin = -4.0
	candleYMax = 1.0
)

func (c *candle) tables() {
	for i := 0; i < candleTA; i++ {
		th := float64(i) / candleTA * 2 * math.Pi
		ct, st := math.Cos(th), math.Sin(th)
		c.rimT[i] = 0.1 + 0.16*noise3(ct*1.4+3, st*1.4, 0.7) + 0.05*noise3(ct*4, st*4, 2.1)
		// Each drip ran over a low notch in the rim.
		for _, dr := range c.drips {
			da := math.Remainder(th-dr.a, 2*math.Pi)
			c.rimT[i] -= 0.1 * math.Exp(-da*da/(0.16*0.16))
		}
		for j := 0; j < candleTY; j++ {
			y := candleYMin + (candleYMax-candleYMin)*float64(j)/(candleTY-1)
			c.radiusT[j*candleTA+i] = 1 + 0.04*noise3(ct*1.8, st*1.8, y*1.2) + 0.02*noise3(ct*5, st*5, y*3)
		}
	}
}

// lookup reads a table around the angle th (wrapping) and, for the radius,
// the height y.
func (c *candle) radiusAt(th, y float64) float64 {
	a := (th/(2*math.Pi) + 1) * candleTA
	i0 := int(a) % candleTA
	fa := a - math.Floor(a)
	i1 := (i0 + 1) % candleTA
	yy := clamp01((y-candleYMin)/(candleYMax-candleYMin)) * (candleTY - 1)
	j0 := min(int(yy), candleTY-2)
	fy := yy - float64(j0)
	g := func(i, j int) float64 { return c.radiusT[j*candleTA+i] }
	return (g(i0, j0)*(1-fa)+g(i1, j0)*fa)*(1-fy) + (g(i0, j0+1)*(1-fa)+g(i1, j0+1)*fa)*fy
}

func (c *candle) rimAt(th float64) float64 {
	a := (th/(2*math.Pi) + 1) * candleTA
	i0 := int(a) % candleTA
	fa := a - math.Floor(a)
	return c.rimT[i0]*(1-fa) + c.rimT[(i0+1)%candleTA]*fa
}

type wdrip struct {
	a, len, r, wig float64
	ca, sa         float64 // where it runs, as a direction
}

type puff struct {
	p, v      Vec3
	age, life float64
}

const (
	candleWickTop = 0.38
	candlePool    = -0.13 // the liquid wax's surface
	candleDist    = 9.0
	candleElev    = 27 * math.Pi / 180
)

var (
	waxRamp   = []uint8{232, 233, 52, 88, 124, 166, 173, 215, 222, 223, 230} // warm, glowing paraffin
	flameRamp = []uint8{52, 88, 124, 160, 196, 202, 208, 214, 220, 221, 228, 229, 230, 231}
	rootRamp  = []uint8{17, 18, 19, 20, 26, 33, 39}
	wallRamp  = []uint8{233, 52, 52, 88, 94, 130, 166}
	poolRamp  = []uint8{232, 52, 88, 130, 166, 172, 214, 220, 221} // molten wax, amber
)

func (c *candle) Setup(ss *Streams) ([]*Gauge, error) {
	c.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("game_rotation_vector", 60)
	if err != nil {
		if st, err = ss.Subscribe("rotation_vector", 60); err != nil {
			return nil, err
		}
	}
	ss.Subscribe("linear_acceleration", 60)
	ss.Subscribe("proximity", 0)
	c.lit, c.power, c.melt = true, 1, 1
	c.eye = Vec3{0, math.Sin(candleElev), math.Cos(candleElev)}
	for i := 0; i < 9; i++ {
		c.drips = append(c.drips, wdrip{
			a:   float64(i)*2*math.Pi/9 + (c.rng.Float64()-0.5)*0.6,
			len: 0.5 + 1.6*c.rng.Float64(),
			r:   0.12 + 0.1*c.rng.Float64(),
			wig: c.rng.Float64() * 6,
		})
		dr := &c.drips[i]
		dr.ca, dr.sa = math.Cos(dr.a), math.Sin(dr.a)
	}
	c.tables()
	return gaugesFor(st, 3), nil
}

func (c *candle) Help() []string {
	return []string{
		"Turn the phone to look at the candle from another side (within a range). Move the phone and the flame lags and sways; shake it and it gutters.",
		"Cover the top of the phone (the proximity sensor) to snuff it; uncover to light it again.",
		"space  snuff or light    r  face it again",
	}
}

func (c *candle) Key(k byte) {
	switch k {
	case ' ':
		c.lit = !c.lit
		if !c.lit {
			c.snuffed()
		}
	case 'r':
		c.hasRef = false
		c.firstCount = 0
	}
}

// snuffed sends up a column of smoke for several seconds and leaves the
// wick's ember glowing a while.
func (c *candle) snuffed() {
	c.ember = 1
	for i := 0; i < 90; i++ {
		c.smoke = append(c.smoke, puff{p: Vec3{0.05, candleWickTop + 0.1, 0}, v: Vec3{0, 0.9 + 0.4*c.rng.Float64(), 0}, life: 4 + 2.5*c.rng.Float64(), age: -float64(i) * 0.05})
	}
}

// look turns the phone's rotation since the reference into a view around
// the candle, easing into limits: 50 degrees either side, and from just
// above the rim to well above it.
func (c *candle) look(ss *Streams, dt float64) {
	spec := "game_rotation_vector"
	if ss.Get(spec) == nil {
		spec = "rotation_vector"
	}
	e := Vec3{0, 0, 1}
	if s := ss.Get(spec); s != nil {
		if r := s.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				R = R.Mul(screenFrame(ss))
				if c.firstCount == 0 {
					c.firstCount = r.Count
				}
				if !c.hasRef && r.Count-c.firstCount >= 20 {
					c.ref, c.hasRef = R, true
				}
				if c.hasRef {
					e = R.T().Apply(c.ref.Apply(Vec3{0, 0, 1}))
				}
			}
		}
	}
	c.eye = c.eye.Add(candleView(e).Sub(c.eye).Scale(math.Min(1, dt*10))).Norm()
}

// candleView maps where the phone's screen faces now, in the reference
// pose's frame (e), to where the camera looks from, easing into the limits.
func candleView(e Vec3) Vec3 {
	const yawLim, pitchLim = 50 * math.Pi / 180, 38 * math.Pi / 180
	yaw := yawLim * math.Tanh(math.Atan2(e[0], e[2])*1.2/yawLim)
	pitch := math.Asin(math.Max(-1, math.Min(1, e[1])))
	el := candleElev + pitchLim*math.Tanh(pitch*1.2/pitchLim)
	el = 0.03 + 0.1*math.Log1p(math.Exp((el-0.03)/0.1)) // never from below the rim
	return Vec3{math.Sin(yaw) * math.Cos(el), math.Sin(el), math.Cos(yaw) * math.Cos(el)}
}

func (c *candle) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	dt = math.Min(dt, 0.1)
	c.look(ss, dt)
	// The flame lags the phone's moves and swings back.
	var acc Vec3
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			acc = toScreen(ss, r.V)
			if m := acc.Len(); m > 6 {
				c.gutter = math.Min(1, c.gutter+(m-6)*dt*0.25)
			}
		}
	}
	c.gutter = math.Max(0, c.gutter-dt*0.5)
	// Screen x is the scene's across; screen z (toward you) its depth.
	for k, a := range [2]float64{acc[0], acc[2]} {
		c.swayV[k] += (-40*c.sway[k] - 5*c.swayV[k] - a*0.16) * dt
		c.sway[k] = math.Max(-0.9, math.Min(0.9, c.sway[k]+c.swayV[k]*dt))
	}
	if s := ss.Get("proximity"); s != nil {
		if r := s.Read(); r.OK && len(r.V) > 0 {
			near := r.V[0] < 3
			switch {
			case near && c.lit:
				c.lit = false
				c.snuffed()
			case !near && !c.lit && c.covered > 1:
				c.lit = true
			}
			if near == !c.lit {
				c.covered += dt
			} else {
				c.covered = 0
			}
		}
	}
	c.ember = math.Max(0, c.ember-dt*0.35)
	if c.lit {
		c.power = math.Min(1, c.power+dt*0.8)
	} else {
		c.power = math.Max(0, c.power-dt*3)
	}
	// The pool melts again under the flame, and sets slowly without it.
	if c.power > 0.5 {
		c.melt = math.Min(1, c.melt+dt*0.3)
	} else {
		c.melt = math.Max(0, c.melt-dt*0.12)
	}
	if c.lit && (c.gutter > 0.4 || c.power < 0.6) && c.rng.Float64() < dt*10 {
		c.smoke = append(c.smoke, puff{p: Vec3{c.sway[0] * 0.5, candleWickTop + 1.6, c.sway[1] * 0.5}, v: Vec3{0, 1, 0}, life: 2})
	}
	for i := 0; i < len(c.smoke); i++ {
		p := &c.smoke[i]
		p.age += dt
		if p.age < 0 {
			continue
		}
		if p.age > p.life {
			c.smoke = append(c.smoke[:i], c.smoke[i+1:]...)
			i--
			continue
		}
		// Rising and curling, and breaking up as it goes.
		p.v[0] += (math.Sin(p.p[1]*1.8+t*1.3)*0.6 + (c.rng.Float64()-0.5)*p.age*2) * dt
		p.v[2] += (math.Cos(p.p[1]*1.5+t*1.1)*0.6 + (c.rng.Float64()-0.5)*p.age*2) * dt
		p.p = p.p.Add(p.v.Scale(dt))
	}
	c.render(v, t)
}

// waxSDF is the candle's wax: an irregular column whose top burned down
// into a hollow inside an uneven rim, with drips down its side; and the
// liquid pool in the hollow, and the wick. It returns the distance and
// the material: 1 wax, 2 liquid wax, 3 wick.
func (c *candle) waxSDF(p Vec3) (float64, int) {
	r := math.Sqrt(p[0]*p[0] + p[2]*p[2])
	th := math.Atan2(p[2], p[0])
	// The column, not quite round.
	side := r - c.radiusAt(th, p[1])
	// The top: an uneven rim around a hollow.
	rim := c.rimAt(th)
	bowl := 0.0
	if r < 0.86 {
		u := r / 0.86
		bowl = -0.34 * (1 - u*u)
	}
	top := rim*smoothstep(0.5, 0.95, r) + bowl
	d := smax(side, (p[1]-top)*0.7, 0.06)
	m := 1
	// Drips: runs of dried wax down the side, a bead at the end.
	for _, dr := range c.drips {
		// Too far to matter here (past the blend): skip it.
		dx, dz, reach := p[0]-dr.ca*0.97, p[2]-dr.sa*0.97, d+0.17+1.7*dr.r
		if reach < 0 || dx*dx+dz*dz > reach*reach || p[1] > 0.45+1.7*dr.r {
			continue
		}
		a := dr.a + 0.07*math.Sin(p[1]*2.3+dr.wig) + 0.04*math.Sin(p[1]*5.1+dr.wig*2)
		base := Vec3{math.Cos(a) * 0.97, 0, math.Sin(a) * 0.97}
		y0 := rim*0.9 - 0.02
		y1 := y0 - dr.len
		yy := math.Max(y1, math.Min(y0, p[1]))
		// Spread where it spilled over, thin down the run, lumpy, and
		// thicker toward the bead.
		along := (y0 - yy) / dr.len
		rr := dr.r * (0.55 + 0.45*along + 0.5*math.Exp(-(y0-yy)/0.12) + 0.18*math.Sin(yy*7+dr.wig))
		q := Vec3{p[0] - base[0], p[1] - yy, p[2] - base[2]}
		dd := q.Len() - rr
		bead := Vec3{p[0] - base[0]*1.02, p[1] - y1, p[2] - base[2]*1.02}.Len() - dr.r*1.35
		d = smin(d, math.Min(dd, bead), 0.05)
	}
	// The liquid pool, filling the hollow up to its surface.
	if liq := math.Max(p[1]-candlePool, r-0.84); liq < d {
		d, m = liq, 2
	}
	// The wick: a thin strand, bent near its top.
	bend := math.Max(0, p[1]-0.15) * 0.25
	wick := math.Max(math.Hypot(p[0]-bend, p[2])-0.035, math.Max(candlePool-0.1-p[1], p[1]-candleWickTop))
	if wick < d {
		d, m = wick, 3
	}
	return d, m
}

// waxNormal samples the distance at a tetrahedron's corners around p.
func (c *candle) waxNormal(p Vec3) Vec3 {
	const e = 0.004
	var n Vec3
	for _, k := range [4]Vec3{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}} {
		d, _ := c.waxSDF(p.Add(k.Scale(e)))
		n = n.Add(k.Scale(d))
	}
	return n.Norm()
}

// cylSpan is the stretch of a ray inside a vertical cylinder (axis at x,
// z; radius r; heights y0..y1), as distances along it.
func cylSpan(o, d Vec3, x, z, r, y0, y1 float64) (float64, float64, bool) {
	ox, oz := o[0]-x, o[2]-z
	a := d[0]*d[0] + d[2]*d[2]
	b := 2 * (ox*d[0] + oz*d[2])
	cc := ox*ox + oz*oz - r*r
	tin, tout := math.Inf(-1), math.Inf(1)
	if a > 1e-12 {
		disc := b*b - 4*a*cc
		if disc < 0 {
			return 0, 0, false
		}
		sq := math.Sqrt(disc)
		tin, tout = (-b-sq)/(2*a), (-b+sq)/(2*a)
	} else if cc > 0 {
		return 0, 0, false
	}
	if d[1] != 0 {
		ta, tb := (y0-o[1])/d[1], (y1-o[1])/d[1]
		if ta > tb {
			ta, tb = tb, ta
		}
		tin, tout = math.Max(tin, ta), math.Min(tout, tb)
	} else if o[1] < y0 || o[1] > y1 {
		return 0, 0, false
	}
	tin = math.Max(tin, 0)
	return tin, tout, tin < tout
}

// flameRay gathers the flame's light along a ray, up to maxT: how bright,
// and how much of it is the blue root.
func (c *candle) flameRay(o, dir Vec3, maxT float64) (float64, float64) {
	glow, rootG := 0.0, 0.0
	if c.power <= 0 {
		return 0, 0
	}
	f0, f1, ok := cylSpan(o, dir, 0.06, 0, 0.55+math.Hypot(c.sway[0], c.sway[1]), candleWickTop-0.2, candleWickTop+c.H*1.2)
	for ft := f0; ok && ft < math.Min(maxT, f1); ft += 0.035 {
		b, root := c.flameAt(o.Add(dir.Scale(ft)), c.H)
		glow += b * 0.035 * math.Min(1, c.power*1.5)
		rootG += b * root * 0.035
	}
	return glow, rootG
}

// flameGlyph draws gathered flame light.
func flameGlyph(glow, rootG float64) (byte, uint8) {
	b := clamp01(glow * 0.9)
	const ramp = ".:+*#%@@"
	ch := ramp[min(len(ramp)-1, int(math.Sqrt(b)*float64(len(ramp)-1)+0.5))]
	if rootG > glow*0.5 {
		return ch, mzPick(rootRamp, b*1.3)
	}
	return ch, mzPick(flameRamp, b)
}

// flameAt is the flame's glow at p: its brightness per unit of path, and
// how much of it is the blue root.
func (c *candle) flameAt(p Vec3, H float64) (float64, float64) {
	s := (p[1] - candleWickTop) / H
	if s < -0.08 || s > 1.15 {
		return 0, 0
	}
	sc := clamp01(s)
	ax := Vec3{0.06 + c.sway[0]*sc*sc, 0, c.sway[1] * sc * sc}
	width := 0.78 * math.Pow(sc+0.04, 0.45) * math.Pow(1.05-sc, 0.9) * (1 - 0.3*c.gutter)
	if s < 0 {
		width = 0.16
	}
	q := math.Hypot(p[0]-ax[0], p[2]-ax[2]) / width
	if q > 1.25 {
		return 0, 0
	}
	edge := 1 - smoothstep(0.5, 1.25, q)
	core := (1 - smoothstep(0, 0.6, q)) * (1 - smoothstep(0.15, 0.75, sc)) * smoothstep(0.02, 0.1, sc)
	b := edge * (0.45 + 0.7*core) * (1 - 0.75*smoothstep(0.6, 1.1, sc)) / width
	root := (1 - smoothstep(0, 0.16, sc)) * (1 - core)
	return b, root
}

func (c *candle) render(v *View, t float64) {
	w, h := v.W, v.H
	E := c.eye
	target := Vec3{0, 0.9, 0}
	F := E.Scale(-1)
	Rt := F.Cross(Vec3{0, 1, 0}).Norm()
	U := Rt.Cross(F)
	origin := target.Add(E.Scale(candleDist))
	tanY := 2.9 / candleDist
	tanX := tanY * float64(w) / (2 * float64(h))

	flick := 0.08*noise3(t*3.1, 0, 0) + 0.05*math.Sin(t*11)*noise3(t*1.7, 5, 0)
	flick += c.gutter * (0.25*math.Sin(t*23) + 0.2*noise3(t*9, 3, 0))
	H := 2.0 * (0.25 + 0.75*c.power) * (1 + flick) * (1 - 0.35*c.gutter)
	c.H, c.t = H, t
	light := c.power * (1 - 0.3*c.gutter) * (1 + 0.5*flick)
	lamp := Vec3{0.06 + c.sway[0]*0.25, candleWickTop + H*0.35, c.sway[1] * 0.25} // the flame's light
	// Where the flame shows on screen, for the glow on the wall behind.
	fc := lamp.Sub(origin)
	fx := fc.Dot(Rt) / fc.Dot(F) / tanX
	fy := fc.Dot(U) / fc.Dot(F) / tanY

	parallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(h)*2
			dir := F.Add(Rt.Scale(u * tanX)).Add(U.Scale(vv * tanY)).Norm()
			// The wax, ray marched only where the ray is inside its bounding
			// cylinder.
			hitT, m := math.Inf(1), 0
			t0, t1, ok := cylSpan(origin, dir, 0, 0, 1.45, -4.5, 0.6)
			tt := t0
			for i := 0; ok && i < 90 && tt < t1; i++ {
				p := origin.Add(dir.Scale(tt))
				if p[1] < -4 {
					break
				}
				d, mm := c.waxSDF(p)
				if d < 0.004 {
					hitT, m = tt, mm
					break
				}
				tt += math.Max(d, 0.006)
			}
			// The flame's light along the ray, in front of the wax.
			glow, rootG := c.flameRay(origin, dir, hitT)
			var ch byte
			var col uint8
			if m != 0 {
				ch, col = c.shade(origin.Add(dir.Scale(hitT)), dir, m, lamp, light)
			} else {
				// The wall: the flame's glow, in sparse dots.
				dx, dy := (u-fx)*tanX*candleDist, (vv-fy)*tanY*candleDist
				g := light * 0.5 / (1 + (dx*dx+dy*dy)*0.18)
				if (x+y)%2 == 0 && g > 0.05 {
					ch = ".:"[boolIdx(g > 0.2)]
					col = mzPick(wallRamp, math.Round(g*8)/8)
				}
			}
			if glow > 0.06 {
				ch, col = flameGlyph(glow, rootG)
			}
			if ch != 0 {
				v.Set(x, y, ch, col)
			}
		}
	})
	c.drawSmoke(v, origin, F, Rt, U, tanX, tanY)
}

// drawSmoke spreads each puff of smoke over the cells it covers, growing
// and thinning as it rises, and draws the sum as wisps.
func (c *candle) drawSmoke(v *View, origin, F, Rt, U Vec3, tanX, tanY float64) {
	if len(c.smoke) == 0 {
		return
	}
	w, h := v.W, v.H
	dens := make([]float64, w*h)
	for _, p := range c.smoke {
		if p.age < 0 {
			continue
		}
		d := p.p.Sub(origin)
		z := d.Dot(F)
		if z <= 0 {
			continue
		}
		sx := (d.Dot(Rt)/z/tanX + 1) / 2 * float64(w)
		sy := (1 - d.Dot(U)/z/tanY) / 2 * float64(h)
		rad := 0.04 + 0.1*p.age + 0.06*p.age*p.age
		rx := math.Max(0.7, rad/(z*tanX)*float64(w)/2)
		ry := math.Max(0.7, rad/(z*tanY)*float64(h)/2)
		amt := (1 - p.age/p.life) * 0.75 / math.Sqrt(rx*ry)
		for y := max(0, int(sy-ry)); y <= min(h-1, int(sy+ry)); y++ {
			for x := max(0, int(sx-rx)); x <= min(w-1, int(sx+rx)); x++ {
				qx, qy := (float64(x)+0.5-sx)/rx, (float64(y)+0.5-sy)/ry
				if q := qx*qx + qy*qy; q < 1 {
					dens[y*w+x] += amt * (1 - q)
				}
			}
		}
	}
	const wisps = ".,:;~"
	for i, dn := range dens {
		if dn < 0.12 {
			continue
		}
		b := clamp01(dn * 0.8)
		v.Set(i%w, i/w, wisps[min(len(wisps)-1, int(b*float64(len(wisps))))], 236+uint8(b*12))
	}
}

// shade lights the wax, the liquid pool and the wick at p, by the flame
// at lamp: soft light on wax, glowing through near the top and the rim,
// the flame mirrored in the pool.
func (c *candle) shade(p, dir Vec3, m int, lamp Vec3, light float64) (byte, uint8) {
	n := c.waxNormal(p)
	toL := lamp.Sub(p)
	dist := toL.Len()
	L := toL.Scale(1 / dist)
	atten := 1 / (1 + 0.1*dist*dist)
	switch m {
	case 3: // the wick: dark, its tip glowing
		if p[1] > candleWickTop-0.12 {
			return '|', mzPick(flameRamp, 0.4*math.Max(c.power, c.ember))
		}
		return '|', mzPick(waxRamp, 0.04)
	case 2: // liquid wax: glossy, lit, the flame mirrored in it
		if c.melt < 0.35 {
			break // set: wax again
		}
		// The surface trembles with the flame.
		wob := 0.03 + 0.08*c.gutter
		n = Vec3{wob * noise3(p[0]*5, p[2]*5, c.t*2), 1, wob * noise3(p[0]*5+7, p[2]*5, c.t*2)}.Norm()
		refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
		i := 0.25*light + 0.45*light*atten + 0.1*c.melt
		if g, root := c.flameRay(p.Add(refl.Scale(0.01)), refl, math.Inf(1)); g*0.7 > 0.06 {
			return flameGlyph(g*0.7, root*0.7)
		}
		const solid = "=+*#%@"
		return solid[int(clamp01(i)*5+0.5)], mzPick(poolRamp, i)
	}
	// Wax: soft, wrapped light, and light through it near the top and the
	// thin rim.
	diff := math.Max(0, (n.Dot(L)+0.35)/1.35)
	r := math.Hypot(p[0], p[2])
	through := light * 0.35 * math.Exp(-math.Max(0, -p[1])*1.3)
	// Drips are thin, and glow further down.
	if bulge := r - c.radiusAt(math.Atan2(p[2], p[0]), p[1]); bulge > 0.01 {
		through += light * 0.3 * smoothstep(0.01, 0.07, bulge) * math.Exp(-math.Max(0, -p[1])*0.6)
	}
	if r > 0.8 && p[1] > -0.35 {
		through += light * 0.12 // the thin rim
	}
	spec := math.Pow(math.Max(0, n.Dot(L.Sub(dir).Norm())), 12) * 0.35 * light // drips and bumps catch it
	// A faint fill from the front left, fading down the column: the
	// drips and bumps near the top show their form, the rest of the
	// column fades into the dark.
	fill := 0.14 * math.Max(0, n.Dot(Vec3{-0.6, 0.3, 0.75}.Norm())) * math.Exp(math.Min(0, p[1])*0.9)
	i := 0.03 + light*diff*atten*0.95 + through + spec + fill
	const solid = "=+*#%@"
	return solid[int(clamp01(i)*5+0.5)], mzPick(waxRamp, i)
}
