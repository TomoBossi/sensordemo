package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "kaleidoscope",
		desc: "turn the phone like a kaleidoscope: colored glass tumbles in its chamber between three mirrors",
		uses: []string{"gravity", "linear_acceleration"},
		new:  func(specs []string) Demo { return &kaleidoscope{} },
	})
}

// kaleidoscope is a three-mirror kaleidoscope. Pieces of colored glass
// (shards, diamonds, beads) lie in a round chamber and tumble under the
// phone's real gravity, bumping each other and the chamber's wall. The
// mirrors form an equilateral triangle: every point of the screen is
// folded into it by reflecting across its sides, so the triangle's view of
// the chamber tiles the screen with mirror symmetry. Turning the phone as
// you would turn a kaleidoscope makes the glass tumble and the pattern
// flow; a shake jolts it.
//
// The chamber is lit from behind, like glass held to the light: pieces
// glow in their colors with dark edges (like lead in stained glass), and
// where two overlap the color deepens.
type kaleidoscope struct {
	pieces []shard
	rng    *rand.Rand
	auto   bool    // turn by itself
	turn   float64 // the auto turn's angle
	kick   float64
	down   [2]float64 // smoothed gravity in the screen plane, unit-ish
}

type shard struct {
	p, v  [2]float64
	a, w  float64      // angle and spin
	r     float64      // radius, for collisions
	shape [][2]float64 // corners around the center; nil = a round bead
	color int
}

const (
	kalR     = 10.0 // chamber radius
	kalG     = 28.0 // gravity in the chamber, units/s^2: glass in oil, slow
	kalShake = 5.0  // per m/s^2 of shake
)

var kalColors = [][]uint8{
	{52, 88, 124, 160, 196, 203, 210},  // ruby
	{17, 18, 19, 20, 27, 33, 75},       // sapphire
	{22, 28, 34, 40, 41, 83, 120},      // emerald
	{53, 54, 90, 91, 128, 135, 177},    // amethyst
	{94, 130, 166, 172, 208, 214, 222}, // amber
	{58, 100, 142, 184, 220, 226, 229}, // citrine
	{23, 30, 37, 44, 45, 87, 123},      // turquoise
	{89, 125, 161, 162, 205, 211, 218}, // rose
}

var kalLight = []uint8{240, 244, 247, 250, 252, 254, 255, 231}

func (k *kaleidoscope) Setup(ss *Streams) ([]*Gauge, error) {
	k.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("gravity", 60)
	if err != nil {
		if st, err = ss.Subscribe("accelerometer", 60); err != nil {
			return nil, err
		}
	}
	ss.Subscribe("linear_acceleration", 60)
	k.fill()
	return gaugesFor(st, 2), nil
}

func (k *kaleidoscope) Help() []string {
	return []string{
		"Hold the phone up and turn it slowly, as you would turn a kaleidoscope: the glass tumbles to the new bottom and the pattern flows. Shake it for a jolt.",
		"a  turn by itself (for a phone lying flat)    space  jolt    r  new glass",
	}
}

func (k *kaleidoscope) Key(c byte) {
	switch c {
	case 'a':
		k.auto = !k.auto
	case ' ':
		k.kick = 0.3
	case 'r':
		k.fill()
	}
}

// fill puts in a fresh handful of glass: enough to pack the chamber, the
// flat pieces overlapping.
func (k *kaleidoscope) fill() {
	k.pieces = k.pieces[:0]
	area := 0.0
	for area < 1.05*math.Pi*kalR*kalR {
		s := shard{r: 1.1 + 1.6*k.rng.Float64(), color: k.rng.Intn(len(kalColors)), a: k.rng.Float64() * 6.3}
		switch k.rng.Intn(4) {
		case 0: // a bead
		case 1: // a triangle shard
			for i := 0; i < 3; i++ {
				a := float64(i)*2*math.Pi/3 + (k.rng.Float64()-0.5)*0.6
				s.shape = append(s.shape, [2]float64{s.r * math.Cos(a), s.r * math.Sin(a)})
			}
		case 2: // a diamond
			s.shape = [][2]float64{{s.r, 0}, {0, s.r * 0.55}, {-s.r, 0}, {0, -s.r * 0.55}}
		default: // an irregular shard
			n := 4 + k.rng.Intn(2)
			for i := 0; i < n; i++ {
				a := float64(i) * 2 * math.Pi / float64(n)
				rr := s.r * (0.7 + 0.3*k.rng.Float64())
				s.shape = append(s.shape, [2]float64{rr * math.Cos(a), rr * math.Sin(a)})
			}
		}
		for tries := 0; tries < 50; tries++ { // somewhere free-ish
			a, rr := k.rng.Float64()*2*math.Pi, (kalR-s.r)*math.Sqrt(k.rng.Float64())
			s.p = [2]float64{rr * math.Cos(a), rr * math.Sin(a)}
			ok := true
			for _, o := range k.pieces {
				if math.Hypot(o.p[0]-s.p[0], o.p[1]-s.p[1]) < 0.8*(o.r+s.r) {
					ok = false
					break
				}
			}
			if ok {
				break
			}
		}
		k.pieces = append(k.pieces, s)
		area += math.Pi * s.r * s.r * 0.7
	}
}

func (k *kaleidoscope) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 6 {
		return
	}
	dt = math.Min(dt, 0.05)
	// Gravity in the screen's plane (y down); lying flat, it fades out.
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	var gx, gy float64
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 3 {
		a := toScreen(ss, r.V)
		gx, gy = -a[0]/9.8, a[1]/9.8
	}
	if k.auto {
		k.turn += dt * 0.35
		gx, gy = math.Sin(k.turn), math.Cos(k.turn)
	}
	k.down[0] += (gx - k.down[0]) * math.Min(1, dt*6)
	k.down[1] += (gy - k.down[1]) * math.Min(1, dt*6)
	var shake [2]float64
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			la := toScreen(ss, r.V)
			shake = [2]float64{-la[0] * kalShake, la[1] * kalShake}
		}
	}
	if k.kick > 0 {
		k.kick -= dt
		shake = [2]float64{80 * math.Sin(t*47), 80 * math.Cos(t*39)}
	}
	for s := 0; s < 4; s++ {
		k.step(dt/4, shake)
	}
	k.render(v)
}

// step moves the glass: gravity and shakes, the oil's drag, the chamber's
// wall, and knocks between pieces (which set them spinning).
func (k *kaleidoscope) step(h float64, shake [2]float64) {
	g := [2]float64{k.down[0]*kalG + shake[0], k.down[1]*kalG + shake[1]}
	for i := range k.pieces {
		p := &k.pieces[i]
		p.v[0] += g[0] * h
		p.v[1] += g[1] * h
		f := math.Max(0, 1-1.2*h)
		p.v[0], p.v[1], p.w = p.v[0]*f, p.v[1]*f, p.w*math.Max(0, 1-1.5*h)
		p.p[0] += p.v[0] * h
		p.p[1] += p.v[1] * h
		p.a += p.w * h
		// The wall: rolling along it turns the piece.
		if d := math.Hypot(p.p[0], p.p[1]); d > kalR-p.r {
			nx, ny := p.p[0]/d, p.p[1]/d
			p.p[0], p.p[1] = nx*(kalR-p.r), ny*(kalR-p.r)
			if vn := p.v[0]*nx + p.v[1]*ny; vn > 0 {
				p.v[0] -= 1.3 * vn * nx
				p.v[1] -= 1.3 * vn * ny
			}
			tang := -p.v[0]*ny + p.v[1]*nx
			p.w += (tang/p.r - p.w) * math.Min(1, 4*h)
		}
	}
	for i := range k.pieces {
		for j := i + 1; j < len(k.pieces); j++ {
			a, b := &k.pieces[i], &k.pieces[j]
			dx, dy := b.p[0]-a.p[0], b.p[1]-a.p[1]
			d := math.Hypot(dx, dy)
			min := 0.6 * (a.r + b.r) // flat pieces slip over each other
			if d >= min || d < 1e-9 {
				continue
			}
			nx, ny := dx/d, dy/d
			push := (min - d) / 2
			a.p[0], a.p[1] = a.p[0]-nx*push, a.p[1]-ny*push
			b.p[0], b.p[1] = b.p[0]+nx*push, b.p[1]+ny*push
			if vn := (b.v[0]-a.v[0])*nx + (b.v[1]-a.v[1])*ny; vn < 0 {
				j := -1.3 * vn / 2
				a.v[0], a.v[1] = a.v[0]-nx*j, a.v[1]-ny*j
				b.v[0], b.v[1] = b.v[0]+nx*j, b.v[1]+ny*j
				tang := -(b.v[0]-a.v[0])*ny + (b.v[1]-a.v[1])*nx
				a.w -= tang * 0.1
				b.w += tang * 0.1
			}
		}
	}
}

// sample colors a point of the chamber: the glass covering it, from the
// top; its edges dark; two layers deepen the color. It returns the
// brightness and the color ramp, or the light behind (nil ramp).
func (k *kaleidoscope) sample(x, y float64) (float64, []uint8, bool) {
	layers, top, edge := 0, -1, false
	for i := len(k.pieces) - 1; i >= 0; i-- {
		p := &k.pieces[i]
		dx, dy := x-p.p[0], y-p.p[1]
		if dx*dx+dy*dy > p.r*p.r*1.2 {
			continue
		}
		// Into the piece's own frame.
		c, s := math.Cos(-p.a), math.Sin(-p.a)
		lx, ly := dx*c-dy*s, dx*s+dy*c
		in, near := false, false
		if p.shape == nil {
			d := math.Hypot(lx, ly)
			in, near = d < p.r*0.8, d > p.r*0.8-0.28
		} else {
			in, near = polyInside(p.shape, lx, ly)
		}
		if !in {
			continue
		}
		layers++
		if top < 0 {
			top, edge = i, near
		}
	}
	if top < 0 {
		return 0, nil, false
	}
	b := 0.8
	if layers > 1 {
		b = 0.62 // seen through another piece, deeper
	}
	if edge {
		b = 0.12
	}
	return b, kalColors[k.pieces[top].color], edge
}

// polyInside reports whether (x, y) is inside a convex polygon, and
// whether it's near its edge.
func polyInside(pts [][2]float64, x, y float64) (bool, bool) {
	sign, nearest := 0.0, math.Inf(1)
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		ex, ey := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(ex, ey)
		cross := (ex*(y-a[1]) - ey*(x-a[0])) / l // signed distance from the edge
		if sign == 0 {
			sign = math.Copysign(1, cross)
		}
		if cross*sign < 0 {
			return false, false
		}
		nearest = math.Min(nearest, math.Abs(cross))
	}
	return true, nearest < 0.28
}

// render folds every character into the mirror triangle, and looks at the
// chamber through it.
func (k *kaleidoscope) render(v *View) {
	w, h := float64(v.W), float64(v.H)
	S := math.Min(w, 2*h) * 0.62 // the triangle's side, in columns: mirrored a few times across
	cx, cy := w/2, h/2
	// The triangle, centered on the screen: corners, and inward edge normals.
	r := S / math.Sqrt(3)
	var corner [3][2]float64
	for i := range corner {
		a := -math.Pi/2 + float64(i)*2*math.Pi/3
		corner[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	type edge struct{ p, n [2]float64 }
	var edges [3]edge
	for i := range edges {
		a, b := corner[i], corner[(i+1)%3]
		nx, ny := -(b[1] - a[1]), b[0]-a[0]
		l := math.Hypot(nx, ny)
		nx, ny = nx/l, ny/l
		if nx*(-a[0])+ny*(-a[1]) < 0 { // toward the center
			nx, ny = -nx, -ny
		}
		edges[i] = edge{a, [2]float64{nx, ny}}
	}
	scale := kalR * 0.6 / r // the triangle sees the middle of the chamber, so pieces show large
	parallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			px, py := float64(x)+0.5-cx, (float64(y)+0.5-cy)*2
			// Reflect across whichever mirror the point is behind, until
			// it's inside the triangle.
			for it := 0; it < 64; it++ {
				moved := false
				for _, e := range edges {
					if d := (px-e.p[0])*e.n[0] + (py-e.p[1])*e.n[1]; d < 0 {
						px -= 2 * d * e.n[0]
						py -= 2 * d * e.n[1]
						moved = true
					}
				}
				if !moved {
					break
				}
			}
			b, ramp, edge := k.sample(px*scale, py*scale)
			// The eyepiece: darker toward the corners.
			vx, vy := (float64(x)+0.5-cx)/cx, (float64(y)+0.5-cy)/cy
			vig := 1 - 0.45*smoothstep(0.55, 1.35, math.Hypot(vx, vy))
			if ramp == nil { // the light behind the glass
				i := 0.75 * vig
				v.Set(x, y, ":."[boolIdx(i < 0.55)], mzPick(kalLight, i))
				continue
			}
			i := b * vig
			ch := "%#"[boolIdx(b < 0.6)]
			if edge {
				ch = '+'
			}
			v.Set(x, y, ch, mzPick(ramp, i))
		}
	})
}
