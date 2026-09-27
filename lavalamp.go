package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "lavalamp",
		desc: "a 3D lava lamp: glowing wax rises and falls; turn the phone to walk around it",
		uses: []string{"game_rotation_vector", "linear_acceleration"},
		new:  func(specs []string) Demo { return &lavalamp{} },
	})
}

// lavalamp is a lava lamp in 3D, ray marched every frame. Like the donut,
// it stays put in the room while the phone moves: turn the phone and you
// see it from another side, without limit. The room's light stays where it
// is, so the chrome's shine and the glass's reflections shift as you go
// around.
//
// The wax pools on the bulb; every few seconds a blob pinches off, hot and
// buoyant, rises to the top, cools, lingers and sinks, and melts back into
// the pool. Rays that reach the glass pick up its reflection (strong at
// grazing angles), then travel through the liquid, which glows (most near
// the bulb and near hot wax, where each blob has a halo) until they meet
// the wax, drawn as metaballs, shaded from below by the bulb, colored by
// its temperature. What the lamp emits also lights the room behind it: the
// glow around the lamp follows its content.
//
// Lamp coordinates: y up along the lamp's axis, from -1 (the foot of the
// base) to 1 (the top of the cap).
type lavalamp struct {
	blobs []blob
	pool  float64 // wax in the pool on the bulb, as radius squared
	next  float64 // when the next blob pinches off
	rng   *rand.Rand
	kick  float64

	ref        Mat3 // the phone's orientation when the view was centered
	hasRef     bool
	firstCount uint64
	recenter   bool
	pose       Mat3 // lamp to screen

	emit []([3]float64) // what each character of the lamp gave off, for the room's glow
	w, h int
}

type blob struct {
	x, y, z    float64
	vx, vy, vz float64
	r          float64
	temp       float64 // 0 cold .. 1 hot
}

// lavaT is the metaball field's value at a lone blob's radius: (3/4)^3.
const lavaT = 0.421875

const (
	lavaBaseTop = -0.78 // the base's top, the bottle's bottom
	lavaCapBot  = 0.76  // the bottle's top, the cap's bottom
	lavaGlass   = 0.018 // the glass's thickness
	lavaDist    = 3.6   // the camera's distance
)

var (
	lavaRoom  = Vec3{-0.6, 0.55, 0.6}.Norm() // the room's light: up, left, in front
	lavaFluid = [3]float64{0.62, 0.1, 0.7}   // the liquid's glow: violet
)

func (l *lavalamp) Setup(ss *Streams) ([]*Gauge, error) {
	l.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("game_rotation_vector", 60)
	if err != nil {
		if st, err = ss.Subscribe("rotation_vector", 60); err != nil {
			return nil, err
		}
	}
	ss.Subscribe("linear_acceleration", 30)
	l.pose = Identity()
	l.fillWax()
	return gaugesFor(st, 3), nil
}

func (l *lavalamp) Help() []string {
	return []string{
		"The bulb warms the wax until a blob pinches off and rises; at the top it cools and sinks back into the pool.",
		"The lamp stays put in the room: turn the phone to walk around it and watch the light play on the glass. Shake to stir.",
		"r  face it again    n  fresh wax    space  stir",
	}
}

func (l *lavalamp) Key(k byte) {
	switch k {
	case 'r':
		l.recenter = true
	case 'n':
		l.fillWax()
	case ' ':
		l.kick = 0.4
	}
}

// radius is the lamp's outline: the chrome base (a short cone with a lip),
// the bottle (widest low down, tapering), the chrome cap. part: 1 base, 2
// bottle, 3 cap, 0 none.
func lavaRadius(y float64) (float64, int) {
	switch {
	case y < -1 || y > 1:
		return 0, 0
	case y < lavaBaseTop:
		u := (y + 1) / (lavaBaseTop + 1)
		r := 0.54 - 0.16*u
		if u > 0.85 {
			r = 0.4 // the lip
		}
		return r, 1
	case y < lavaCapBot:
		u := (y - lavaBaseTop) / (lavaCapBot - lavaBaseTop)
		return 0.36 + 0.11*math.Sin(math.Min(u/0.3, 1)*math.Pi/2) - 0.3*smoothstep(0.3, 1, u), 2
	default:
		u := (y - lavaCapBot) / (1 - lavaCapBot)
		r := 0.19 - 0.07*u
		if u > 0.75 {
			r *= math.Sqrt(math.Max(0, 1-math.Pow((u-0.75)/0.25, 2)))
		}
		return r, 3
	}
}

// lampSDF is a (conservative) distance to the lamp's outside, and which
// part is nearest.
func lampSDF(p Vec3) (float64, int) {
	rr := math.Hypot(p[0], p[2])
	r, part := lavaRadius(p[1])
	d := (rr - r) * 0.8
	// Above and below it, the distance to its ends.
	if p[1] > 1 {
		d = math.Max(d, p[1]-1)
	} else if p[1] < -1 {
		d = math.Max(d, -1-p[1])
	}
	if part == 0 {
		d = math.Max(d, math.Min(math.Abs(p[1]-1), math.Abs(p[1]+1)))
		part = 1
	}
	return d, part
}

func lampNormal(p Vec3) Vec3 {
	const e = 0.003
	f := func(q Vec3) float64 { d, _ := lampSDF(q); return d }
	return Vec3{
		f(p.Add(Vec3{e, 0, 0})) - f(p.Sub(Vec3{e, 0, 0})),
		f(p.Add(Vec3{0, e, 0})) - f(p.Sub(Vec3{0, e, 0})),
		f(p.Add(Vec3{0, 0, e})) - f(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

// inLiquid reports whether p is inside the bottle's liquid.
func inLiquid(p Vec3) bool {
	if p[1] <= lavaBaseTop || p[1] >= lavaCapBot {
		return false
	}
	r, _ := lavaRadius(p[1])
	return math.Hypot(p[0], p[2]) < r-lavaGlass
}

// fillWax pours fresh wax: most of it in the pool, two blobs on their way.
func (l *lavalamp) fillWax() {
	l.blobs = l.blobs[:0]
	l.pool = 6 * 0.1 * 0.1
	for i := 0; i < 2; i++ {
		a := l.rng.Float64() * 2 * math.Pi
		l.blobs = append(l.blobs, blob{
			x: 0.12 * math.Cos(a), z: 0.12 * math.Sin(a), y: -0.2 + 0.5*float64(i),
			r: 0.08 + 0.04*l.rng.Float64(), temp: 0.9 - 0.2*float64(i),
		})
	}
	l.next = 0
}

// poolBlobs is the pool: three low blobs around the axis, sized by the
// wax in it, swelling gently.
func (l *lavalamp) poolBlobs(t float64) [3]blob {
	rp := math.Sqrt(l.pool/2.4) + 0.02
	var out [3]blob
	for i := range out {
		a := float64(i)*2*math.Pi/3 + 0.3
		out[i] = blob{
			x: 0.13 * math.Cos(a), z: 0.13 * math.Sin(a),
			y:    lavaBaseTop + rp*0.35 + 0.01*math.Sin(t*0.7+float64(i)*2),
			r:    rp * (0.92 + 0.08*math.Sin(t*0.5+float64(i))),
			temp: 1,
		}
	}
	return out
}

func (l *lavalamp) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	dt = math.Min(dt, 0.1)
	l.look(ss)
	// A shake stirs, in the lamp's frame.
	var stir Vec3
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			stir = l.pose.T().Apply(toScreen(ss, r.V)).Scale(-0.05)
		}
	}
	if l.kick > 0 {
		l.kick -= dt
		stir = Vec3{0.8 * math.Sin(t*31), 0.5 * math.Cos(t*23), 0.8 * math.Cos(t*29)}
	}
	for s := 0; s < 3; s++ {
		l.step(dt/3, stir, t)
	}
	l.render(v, t)
}

// look sets the pose from the phone's orientation, as the donut does: the
// lamp keeps its place in the room.
func (l *lavalamp) look(ss *Streams) {
	spec := "game_rotation_vector"
	if ss.Get(spec) == nil {
		spec = "rotation_vector"
	}
	s := ss.Get(spec)
	if s == nil {
		return
	}
	r := s.Read()
	R, ok := FromRotationVector(r.V)
	if !r.OK || !ok {
		return
	}
	R = R.Mul(screenFrame(ss))
	if l.firstCount == 0 {
		l.firstCount = uint64(r.Count)
	}
	// Fusion needs a moment to settle after the sensor powers on.
	if (!l.hasRef && uint64(r.Count)-l.firstCount >= 30) || l.recenter {
		l.ref, l.hasRef, l.recenter = R, true, false
	}
	if l.hasRef {
		l.pose = R.T().Mul(l.ref)
	}
}

// step pinches blobs off the pool, lifts and cools them, and melts the
// ones that sink back into it. Heat comes only from the bulb, so there's
// no height where a blob could balance and park.
func (l *lavalamp) step(h float64, stir Vec3, t float64) {
	pool := l.poolBlobs(t)
	top := lavaBaseTop + pool[0].r*0.9
	span := lavaCapBot - lavaBaseTop
	if t >= l.next && l.pool > 0.012 {
		r := 0.07 + 0.05*l.rng.Float64()
		if r*r < l.pool*0.6 {
			l.pool -= r * r
			a := l.rng.Float64() * 2 * math.Pi
			l.blobs = append(l.blobs, blob{x: 0.1 * math.Cos(a), z: 0.1 * math.Sin(a), y: top - r*0.2, r: r, temp: 1})
		}
		l.next = t + 3 + 4*l.rng.Float64()
	}
	for i := 0; i < len(l.blobs); i++ {
		b := &l.blobs[i]
		u := (b.y - lavaBaseTop) / span
		b.temp -= b.temp * h * (0.012 + 0.05*u*u) // the liquid cools it, most up high
		lift := (b.temp - 0.45) * 0.42            // warm wax is lighter than the liquid
		b.vy += (lift + stir[1]) * h
		b.vx += (stir[0] + 0.02*math.Sin(t*0.3+float64(i)*1.7)) * h
		b.vz += (stir[2] + 0.02*math.Cos(t*0.27+float64(i)*2.3)) * h
		drag := math.Exp(-1.6 * h)
		b.vx, b.vy, b.vz = b.vx*drag, b.vy*drag, b.vz*drag
		b.x += b.vx * h
		b.y += b.vy * h
		b.z += b.vz * h
		if b.vy < 0 && b.temp < 0.45 && b.y < top+b.r*0.2 { // sunk to the pool: it melts in
			l.pool += b.r * b.r
			l.blobs = append(l.blobs[:i], l.blobs[i+1:]...)
			i--
		}
	}
	// Blobs make room for each other, softly.
	for i := range l.blobs {
		for j := i + 1; j < len(l.blobs); j++ {
			a, b := &l.blobs[i], &l.blobs[j]
			d := Vec3{b.x - a.x, b.y - a.y, b.z - a.z}
			dist := d.Len()
			min := 0.7 * (a.r + b.r)
			if dist < min && dist > 1e-6 {
				f := (min - dist) / min * 0.4 * h / dist
				a.vx, a.vy, a.vz = a.vx-d[0]*f, a.vy-d[1]*f, a.vz-d[2]*f
				b.vx, b.vy, b.vz = b.vx+d[0]*f, b.vy+d[1]*f, b.vz+d[2]*f
			}
		}
	}
	// The glass: blobs squeeze against it, rest at the top and bottom.
	for i := range l.blobs {
		b := &l.blobs[i]
		b.y = math.Max(lavaBaseTop+b.r*0.4, math.Min(lavaCapBot-b.r*0.5, b.y))
		if b.y >= lavaCapBot-b.r*0.5 {
			b.vy = math.Min(b.vy, 0)
		}
		r, _ := lavaRadius(b.y)
		lim := math.Max(0, r-lavaGlass-b.r*0.6)
		if rr := math.Hypot(b.x, b.z); rr > lim {
			b.x, b.z = b.x*lim/rr, b.z*lim/rr
			b.vx, b.vz = b.vx*0.3, b.vz*0.3
		}
	}
}

// field is the wax's metaball field at p (lavaT at a lone blob's surface),
// and its temperature there.
func waxField(blobs []blob, p Vec3) (float64, float64) {
	f, heat := 0.0, 0.0
	for i := range blobs {
		b := &blobs[i]
		dx, dy, dz := p[0]-b.x, p[1]-b.y, p[2]-b.z
		q := (dx*dx + dy*dy + dz*dz) / (4 * b.r * b.r)
		if q >= 1 {
			continue
		}
		k := 1 - q
		k = k * k * k
		f += k
		heat += k * b.temp
	}
	if f > 0 {
		heat /= f
	}
	return f, heat
}

func (l *lavalamp) render(v *View, t float64) {
	w, h := v.W, v.H
	if len(l.emit) != w*h {
		l.emit = make([][3]float64, w*h)
	}
	l.w, l.h = w, h
	wax := append(append([]blob(nil), l.blobs...), func() []blob { p := l.poolBlobs(t); return p[:] }()...)
	// The camera, in lamp coordinates.
	inv := l.pose.T()
	right, up, back := inv.Apply(Vec3{1, 0, 0}), inv.Apply(Vec3{0, 1, 0}), inv.Apply(Vec3{0, 0, 1})
	eye := back.Scale(lavaDist)
	tanY := 1.18 / lavaDist
	tanX := tanY * float64(w) / (2 * float64(h))
	cells := make([]artCell, w*h)
	parallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(h)*2
			dir := back.Scale(-1).Add(right.Scale(u * tanX)).Add(up.Scale(vv * tanY)).Norm()
			rgb, emitted, hit := l.trace(eye, dir, wax, t)
			i := y*w + x
			l.emit[i] = emitted
			if hit {
				cells[i] = lavaCell(rgb)
			}
		}
	})
	// The room behind: lit by what the lamp gives off, blurred.
	glow := l.roomGlow()
	for i := range cells {
		if cells[i].ch == 0 {
			g := glow[i]
			if lum := 0.3*g[0] + 0.55*g[1] + 0.15*g[2]; lum > 0.02 && (i%w+i/w)%2 == 0 {
				cells[i] = artCell{".:"[boolIdx(lum > 0.09)], rgb256(quantize(g)), false}
			}
		}
		if cells[i].ch != 0 && cells[i].ch != ' ' {
			v.Set(i%w, i/w, cells[i].ch, cells[i].col)
		}
	}
}

// quantize snaps a color to a few levels, so the glow's gradient changes
// color seldom (the terminal draws each change).
func quantize(c [3]float64) [3]float64 {
	for i := range c {
		c[i] = math.Round(clamp01(c[i])*8) / 8
	}
	return c
}

func lavaCell(rgb [3]float64) artCell {
	lum := 0.3*rgb[0] + 0.55*rgb[1] + 0.15*rgb[2]
	// Dots and crosses, not dashes: a glowing volume shouldn't read as
	// stripes.
	const ramp = " .:;+x*#%@"
	ch := ramp[min(len(ramp)-1, int(math.Sqrt(clamp01(lum))*float64(len(ramp)-1)+0.5))]
	if ch == ' ' {
		ch = '.'
	}
	return artCell{ch, rgb256(rgb), false}
}

// roomGlow blurs what the lamp gave off across the screen: the light it
// throws on the room around it.
func (l *lavalamp) roomGlow() [][3]float64 {
	w, h := l.w, l.h
	cw, ch := (w+3)/4, (h+1)/2 // a coarse grid, cells twice as wide as tall, like the screen's
	grid := make([][3]float64, cw*ch)
	for i, e := range l.emit {
		g := &grid[(i/w)/2*cw+(i%w)/4]
		for c := 0; c < 3; c++ {
			g[c] += e[c] / 8
		}
	}
	blur := func(src [][3]float64, dx, dy, radius int) [][3]float64 {
		out := make([][3]float64, len(src))
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				var s [3]float64
				n := 0
				for k := -radius; k <= radius; k++ {
					xx, yy := x+k*dx, y+k*dy
					if xx < 0 || yy < 0 || xx >= cw || yy >= ch {
						continue
					}
					for c := 0; c < 3; c++ {
						s[c] += src[yy*cw+xx][c]
					}
					n++
				}
				for c := 0; c < 3; c++ {
					out[y*cw+x][c] = s[c] / float64(n)
				}
			}
		}
		return out
	}
	for pass := 0; pass < 2; pass++ {
		grid = blur(grid, 1, 0, 4)
		grid = blur(grid, 0, 1, 4)
	}
	out := make([][3]float64, w*h)
	for i := range out {
		g := grid[(i/w)/2*cw+(i%w)/4]
		for c := 0; c < 3; c++ {
			out[i][c] = g[c] * 4.5
		}
	}
	return out
}

// env is the room as the chrome and glass mirror it: a bright window up
// left, a lit ceiling, a dark floor.
func env(r Vec3) float64 {
	e := 0.12 + 0.25*smoothstep(-0.2, 0.8, r[1])
	e += 1.2 * math.Pow(math.Max(0, r.Dot(lavaRoom)), 24)
	return e
}

// trace follows a ray: the chrome, or the glass's reflection and then the
// glowing liquid and the wax. It returns the color, what the lamp itself
// emitted along the ray (for the room's glow), and whether it hit the lamp.
func (l *lavalamp) trace(eye, dir Vec3, wax []blob, t float64) ([3]float64, [3]float64, bool) {
	var none [3]float64
	tt := lavaDist - 1.2
	var p Vec3
	part := 0
	for i := 0; i < 90; i++ {
		p = eye.Add(dir.Scale(tt))
		d, pt := lampSDF(p)
		if d < 0.002 {
			part = pt
			break
		}
		tt += math.Max(d, 0.003)
		if tt > lavaDist+1.3 {
			return none, none, false
		}
	}
	if part == 0 {
		return none, none, false
	}
	n := lampNormal(p)
	refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
	if part != 2 { // chrome: the room, mirrored, and a little of the lamp's glow below the cap
		e := env(refl) + 0.35*math.Max(0, n.Dot(lavaRoom))
		c := [3]float64{0.8 * e, 0.82 * e, 0.9 * e}
		if part == 1 && p[1] > -0.93 && p[1] < -0.84 && math.Mod(math.Atan2(p[2], p[0])*9+20, 1) < 0.4 {
			c = [3]float64{0.25, 0.08, 0.02} // vents, the bulb glowing through
		}
		return c, none, true
	}
	// Glass: its reflection, strong at grazing angles.
	cos := math.Abs(n.Dot(dir))
	fres := 0.05 + 0.95*math.Pow(1-cos, 4)
	e := env(refl)
	var rgb [3]float64
	for c := 0; c < 3; c++ {
		rgb[c] = fres * e * 0.9
	}
	// Through the liquid.
	var emitted [3]float64
	trans := 1.0
	const ds = 0.022
	q := p.Add(dir.Scale(lavaGlass * 1.5))
	for i := 0; i < 70; i++ {
		q = q.Add(dir.Scale(ds))
		if !inLiquid(q) {
			if i > 2 {
				break
			}
			continue
		}
		f, heat := waxField(wax, q)
		if f >= lavaT { // the wax
			c := l.shadeWax(q, dir, wax, heat)
			for k := 0; k < 3; k++ {
				emitted[k] += trans * c[k]
				rgb[k] += trans * c[k]
			}
			return rgb, emitted, true
		}
		// The liquid glows: most by the bulb, and around hot wax.
		u := (q[1] - lavaBaseTop) / (lavaCapBot - lavaBaseTop)
		bulb := math.Exp(-u * 3.2)
		aura := smoothstep(0.02, lavaT, f) * (0.4 + heat)
		g := (0.95 + 1.8*bulb) * ds
		a := aura * 2.2 * ds
		for k := 0; k < 3; k++ {
			add := trans * (lavaFluid[k]*g + [3]float64{1, 0.42, 0.08}[k]*a)
			emitted[k] += add
			rgb[k] += add
		}
		trans *= math.Exp(-1.1 * ds)
	}
	return rgb, emitted, true
}

// shadeWax colors the wax where a ray meets it: lit from below by the
// bulb, a little by the room, glossy, glowing with its heat.
func (l *lavalamp) shadeWax(p, dir Vec3, wax []blob, heat float64) [3]float64 {
	const e = 0.01
	f := func(q Vec3) float64 { v, _ := waxField(wax, q); return v }
	n := Vec3{
		f(p.Sub(Vec3{e, 0, 0})) - f(p.Add(Vec3{e, 0, 0})),
		f(p.Sub(Vec3{0, e, 0})) - f(p.Add(Vec3{0, e, 0})),
		f(p.Sub(Vec3{0, 0, e})) - f(p.Add(Vec3{0, 0, e})),
	}.Norm()
	u := (p[1] - lavaBaseTop) / (lavaCapBot - lavaBaseTop)
	bulb := math.Max(0, n.Dot(Vec3{0, -1, 0}))*(0.9*math.Exp(-u*1.5)) + 0.15
	room := 0.3 * math.Max(0, n.Dot(lavaRoom))
	spec := math.Pow(math.Max(0, n.Dot(lavaRoom.Sub(dir).Norm())), 30)
	glow := 0.35 + 0.5*heat // wax glows with its heat
	hot := [3]float64{1, 0.62, 0.12}
	cool := [3]float64{0.85, 0.12, 0.05}
	var c [3]float64
	for k := 0; k < 3; k++ {
		base := cool[k] + (hot[k]-cool[k])*heat
		c[k] = base*(bulb+room+glow*0.6) + 0.6*spec
	}
	return c
}
