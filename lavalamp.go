package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "lavalamp",
		desc: "a lava lamp: warm wax rises, cools and sinks, rising toward the real up as you tilt the phone",
		uses: []string{"gravity", "linear_acceleration"},
		new:  func(specs []string) Demo { return &lavalamp{} },
	})
}

// lavalamp is a lava lamp: a chrome base and cap and a tapered glass
// bottle, rendered once per screen size as surfaces of revolution, with
// the wax drawn inside every frame. Hot wax pools on the bulb; every few
// seconds a blob pinches off it, hot and buoyant, rises, slowly cools,
// lingers at the top and sinks, and melts back into the pool. (Heat that
// merely fades with height would let blobs find a height where they
// balance, and park there.) They're drawn as
// metaballs, so they merge and pinch apart, shaded glossy from the field's
// slope and lit from below by the bulb, in liquid that glows brightest
// near it. Buoyancy works along the real up, so tilting the phone tilts
// the wax's path; a shake stirs it.
//
// Units are columns; y is up from the bottom of the view (a row is two
// units tall).
type lavalamp struct {
	w, h  int
	art   []artCell // the lamp, rendered once
	inner []bool    // where the liquid shows
	cx    float64
	Hh    float64 // lamp height, units
	y0    float64 // the bottle's bottom and top
	y1    float64
	rmax  float64 // the bottle's widest radius
	blobs []blob
	rng   *rand.Rand
	up    [2]float64
	kick  float64
	pool  float64 // wax in the pool on the bulb, as radius squared
	next  float64 // when the next blob pinches off
}

// lavaT is the metaball field's value at a lone blob's radius: (3/4)^3.
const lavaT = 0.421875

type blob struct {
	x, y, vx, vy float64
	r            float64
	temp         float64 // 0 cold .. 1 hot
}

var (
	lavaWax    = []uint8{52, 88, 124, 160, 196, 202, 208, 214, 220, 221}
	lavaLiquid = []uint8{17, 17, 54, 54, 55, 56, 92, 93, 99}
	lavaChrome = []uint8{234, 236, 238, 240, 243, 246, 249, 252, 255, 231}
)

func (l *lavalamp) Setup(ss *Streams) ([]*Gauge, error) {
	l.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	st, err := ss.Subscribe("gravity", 30)
	if err != nil {
		if st, err = ss.Subscribe("accelerometer", 30); err != nil {
			return nil, err
		}
	}
	ss.Subscribe("linear_acceleration", 30)
	l.up = [2]float64{0, 1}
	return gaugesFor(st, 2), nil
}

func (l *lavalamp) Help() []string {
	return []string{
		"The bulb in the base warms the wax until it rises; at the top it cools and sinks. Watch the blobs merge and split.",
		"Tilt the phone and the wax rises toward the real up. Shake it to stir.",
		"space  stir    r  fresh wax",
	}
}

func (l *lavalamp) Key(k byte) {
	switch k {
	case ' ':
		l.kick = 0.4
	case 'r':
		l.blobs = nil
	}
}

// profile is the lamp's radius at height y, and which part it is: base
// (a chrome cone), bottle (glass, widest low down, tapering), cap.
func (l *lavalamp) profile(y float64) (float64, int) {
	H := l.Hh
	W := l.rmax
	switch t := y / H; {
	case t < 0:
		return 0, artNone
	case t < 0.24: // the base: a cone widening downward, a lip at the top
		u := t / 0.24
		r := W*1.45 - u*W*0.62
		if u > 0.9 {
			r = W * 0.88
		}
		return r, 1
	case t < 0.86: // the bottle
		u := (t - 0.24) / 0.62
		r := W * (0.8 + 0.2*math.Sin(math.Min(u/0.35, 1)*math.Pi/2) - 0.58*smoothstep(0.35, 1, u))
		return r, 2
	case t < 1: // the cap: a small cone with a rounded top
		u := (t - 0.86) / 0.14
		r := W * (0.44 - 0.2*u)
		if u > 0.8 {
			r *= math.Sqrt(math.Max(0, 1-math.Pow((u-0.8)/0.2, 2)))
		}
		return r, 3
	}
	return 0, artNone
}

func (l *lavalamp) layout(w, h int) {
	l.w, l.h = w, h
	l.cx = float64(w) / 2
	l.Hh = float64(h)*2 - 2
	l.rmax = math.Min(float64(w)*0.28, l.Hh*0.16)
	l.y0, l.y1 = 0.24*l.Hh, 0.86*l.Hh
	l.art = make([]artCell, w*h)
	l.inner = make([]bool, w*h)
	light := Vec3{-0.55, 0.45, 0.7}.Norm() // x right, y up, z out
	for cy := 0; cy < h; cy++ {
		y := float64(h-1-cy)*2 + 1 // row center, up from the bottom
		r, part := l.profile(y)
		for cx := 0; cx < w; cx++ {
			x := float64(cx) + 0.5 - l.cx
			i := cy*w + cx
			if part == artNone || math.Abs(x) > r {
				// The lamp's light on the wall behind: a faint warm glow.
				gd := math.Hypot(x, (y-l.Hh*0.45)*0.6) / l.Hh
				if g := 0.5 - gd*1.6; g > 0 && (cx+cy)%2 == 0 {
					l.art[i] = artCell{'.', mzPick([]uint8{52, 53, 89, 125}, g*1.6), false}
				}
				continue
			}
			// The surface's normal: around the axis, and tilted by its slope.
			r2, _ := l.profile(y + 1)
			r1, _ := l.profile(y - 1)
			slope := (r2 - r1) / 2
			nx := x / r
			nz := math.Sqrt(math.Max(0, 1-nx*nx))
			n := Vec3{nx, -slope * nz, nz}.Norm()
			diff := math.Max(0, n.Dot(light))
			spec := math.Pow(math.Max(0, n.Dot(light.Add(Vec3{0, 0, 1}).Norm())), 20)
			switch part {
			case 1, 3: // chrome: mostly reflections, bright and dark bands
				env := 0.5 + 0.5*math.Sin(nx*5+1)
				c := 0.15 + 0.35*diff + 0.3*env + 0.7*spec
				ch := byte("=+#%@"[int(clamp01(c)*4+0.5)])
				if part == 1 && y > l.Hh*0.07 && y < l.Hh*0.17 && math.Abs(nx) < 0.7 && int(x+100)%3 == 0 {
					ch, c = '|', 0.05 // vents
				}
				l.art[i] = artCell{ch, mzPick(lavaChrome, c), false}
			case 2: // glass: the liquid shows; the edge catches light
				if math.Abs(x) > r-1 {
					l.art[i] = artCell{"|/\\"[boolIdx(slope != 0)*(1+boolIdx(slope*math.Copysign(1, x) > 0))], 110, false}
					continue
				}
				l.inner[i] = true
			}
		}
	}
}

// fillWax pours fresh wax: most of it in the pool, two blobs on their way.
func (l *lavalamp) fillWax() {
	l.blobs = l.blobs[:0]
	unit := l.rmax * l.rmax * 0.07 // one blob's worth, as radius squared
	l.pool = 6 * unit
	for i := 0; i < 2; i++ {
		r := l.rmax * (0.22 + 0.08*l.rng.Float64())
		l.blobs = append(l.blobs, blob{
			x: (l.rng.Float64()*2 - 1) * l.rmax * 0.3, y: l.y0 + (l.y1-l.y0)*(0.35+0.3*float64(i)),
			r: r, temp: 0.8 - 0.3*float64(i),
		})
	}
	l.next = 0
}

// poolBlobs is the pool, as three low blobs sized by the wax in it,
// swelling gently.
func (l *lavalamp) poolBlobs(t float64) [3]blob {
	rp := math.Sqrt(l.pool/2.2) + 1
	var out [3]blob
	for i := range out {
		x := (float64(i) - 1) * l.rmax * 0.45
		out[i] = blob{x: x, y: l.y0 + rp*0.15 + 0.6*math.Sin(t*0.7+float64(i)*2), r: rp * (0.9 + 0.1*math.Sin(t*0.5+float64(i)))}
	}
	return out
}

func (l *lavalamp) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	if v.W != l.w || v.H != l.h {
		l.layout(v.W, v.H)
		l.blobs = nil
	}
	if l.blobs == nil {
		l.fillWax()
	}
	dt = math.Min(dt, 0.1)
	// Up, in the screen's plane; lying flat, the screen's own up.
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 3 {
		a := toScreen(ss, r.V)
		ux, uy := a[0], a[1]
		k := clamp01((math.Hypot(ux, uy) - 1.5) / 3)
		if m := math.Hypot(ux, uy); m > 0 {
			ux, uy = ux/m*k, uy/m*k+(1-k)
		}
		m := math.Hypot(ux, uy)
		l.up[0] += (ux/m - l.up[0]) * math.Min(1, dt*2)
		l.up[1] += (uy/m - l.up[1]) * math.Min(1, dt*2)
	}
	var stir [2]float64
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 2 {
			la := toScreen(ss, r.V)
			stir = [2]float64{-la[0] * 0.8, -la[1] * 0.8}
		}
	}
	if l.kick > 0 {
		l.kick -= dt
		stir = [2]float64{20 * math.Sin(t*31), 14 * math.Cos(t*23)}
	}
	for s := 0; s < 3; s++ {
		l.step(dt/3, stir, t)
	}
	l.render(v, t)
}

// step pinches blobs off the pool, lifts and cools them, and melts the
// ones that sink back into it.
func (l *lavalamp) step(h float64, stir [2]float64, t float64) {
	pool := l.poolBlobs(t)
	top := l.y0 + pool[1].r*0.9
	if t >= l.next && l.pool > l.rmax*l.rmax*0.1 {
		r := l.rmax * (0.18 + 0.12*l.rng.Float64())
		if r*r < l.pool*0.6 {
			l.pool -= r * r
			l.blobs = append(l.blobs, blob{x: (l.rng.Float64()*2 - 1) * l.rmax * 0.3, y: top - r*0.3, r: r, temp: 1})
		}
		l.next = t + 3 + 5*l.rng.Float64()
	}
	for i := 0; i < len(l.blobs); i++ {
		b := &l.blobs[i]
		b.temp -= b.temp * h * 0.035 // the liquid cools it
		// Buoyancy along up: warm wax is lighter than the liquid.
		lift := (b.temp - 0.45) * 14
		b.vx += (l.up[0]*lift + stir[0]) * h
		b.vy += (l.up[1]*lift + stir[1]) * h
		// A slow wander, so no two rises are alike.
		b.vx += math.Sin(t*0.3+float64(i)*1.7) * 0.4 * h
		drag := math.Exp(-1.6 * h)
		b.vx, b.vy = b.vx*drag, b.vy*drag
		b.x += b.vx * h
		b.y += b.vy * h
		// Sunk back to the pool: it melts in.
		if b.vy < 0 && b.temp < 0.45 && b.y < top+b.r*0.2 {
			l.pool += b.r * b.r
			l.blobs = append(l.blobs[:i], l.blobs[i+1:]...)
			i--
		}
	}
	// Blobs make room for each other, softly: they may overlap and merge
	// on screen, but not collapse into one.
	for i := range l.blobs {
		for j := i + 1; j < len(l.blobs); j++ {
			a, b := &l.blobs[i], &l.blobs[j]
			dx, dy := b.x-a.x, b.y-a.y
			d := math.Hypot(dx, dy)
			min := 0.7 * (a.r + b.r)
			if d < min && d > 1e-6 {
				f := (min - d) / min * 6 * h
				a.vx, a.vy = a.vx-dx/d*f, a.vy-dy/d*f
				b.vx, b.vy = b.vx+dx/d*f, b.vy+dy/d*f
			}
		}
	}
	// The glass: blobs squeeze against it, and pool on the bottom.
	for i := range l.blobs {
		b := &l.blobs[i]
		lo, hi := l.y0+b.r*0.45, l.y1-b.r*0.6
		if b.y < lo {
			b.y, b.vy = lo, math.Max(b.vy, 0)
		}
		if b.y > hi {
			b.y, b.vy = hi, math.Min(b.vy, 0)
		}
		r, _ := l.profile(b.y)
		if lim := r - b.r*0.55; math.Abs(b.x) > lim {
			b.x = math.Copysign(math.Max(lim, 0), b.x)
			b.vx *= -0.3
		}
	}
}

func (l *lavalamp) render(v *View, t float64) {
	w := l.w
	for i, a := range l.art {
		if a.ch != 0 && a.ch != ' ' {
			v.Set(i%w, i/w, a.ch, a.col)
		}
	}
	pool := l.poolBlobs(t)
	bulb := Vec3{0, -0.8, 0.6}.Norm() // light from below, y up
	front := Vec3{-0.5, 0.4, 0.75}.Norm()
	parallelRows(l.h, func(cy int) {
		y := float64(l.h-1-cy)*2 + 1
		r, _ := l.profile(y)
		glow := 1 - clamp01((y-l.y0)/(l.y1-l.y0)) // brightest near the bulb
		for cx := 0; cx < w; cx++ {
			i := cy*w + cx
			if !l.inner[i] {
				continue
			}
			x := float64(cx) + 0.5 - l.cx
			// The wax: a metaball field that falls to nothing at twice each
			// blob's radius (so blobs keep their size and only neighbors
			// merge); the surface is where it reaches lavaT, a lone blob's
			// radius. Its slope gives a normal.
			f, gx, gy, heat := 0.0, 0.0, 0.0, 0.0
			for bi, b := range append(l.blobs, pool[:]...) {
				if bi >= len(l.blobs) {
					b.temp = 1 // the pool, on the bulb
				}
				dx, dy := x-b.x, y-b.y
				q := (dx*dx + dy*dy) / (4 * b.r * b.r)
				if q >= 1 {
					continue
				}
				k := 1 - q
				f += k * k * k
				heat += k * k * k * b.temp
				dk := -6 * k * k / (4 * b.r * b.r) // d(k^3)/d(r^2) * 2
				gx += dk * dx
				gy += dk * dy
			}
			f /= lavaT
			edge := math.Abs(x) / r // toward the glass's edge, the round bottle darkens
			if f > 1 {
				n := Vec3{-gx * l.rmax * 2, -gy * l.rmax * 2, 1.4}.Norm()
				lit := 0.2 + 0.55*math.Max(0, n.Dot(bulb))*(0.5+0.5*glow) + 0.35*math.Max(0, n.Dot(front))
				spec := math.Pow(math.Max(0, n.Dot(front.Add(Vec3{0, 0, 1}).Norm())), 24)
				warm := heat / (f * lavaT) // the wax's temperature here
				c := (lit + 0.6*spec + 0.25*warm - 0.1) * (1 - 0.35*edge*edge)
				ch := "=+#%@"[int(clamp01(c)*4+0.5)]
				v.Set(cx, cy, ch, mzPick(lavaWax, c))
				continue
			}
			// The liquid: glowing near the bulb and near the wax.
			c := (0.2 + 0.55*glow + 0.25*clamp01(f)) * (1 - 0.5*edge*edge)
			ch := byte(" .:"[int(clamp01(c)*2.99)])
			if ch == ' ' {
				ch = '.'
			}
			v.Set(cx, cy, ch, mzPick(lavaLiquid, c))
		}
	})
}
