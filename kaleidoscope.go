package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "kaleidoscope",
		desc: "turn the phone like a kaleidoscope: glass in its chamber, twelve-fold between two mirrors",
		uses: []string{"gyroscope"},
		new:  func(specs []string) Demo { return &kaleidoscope{} },
	})
}

// kaleidoscope is a two-mirror kaleidoscope: mirrors at 30 degrees make
// twelve images of a wedge of the object chamber, a mandala centered on
// the screen. Every character is folded into that wedge (mirror lines lie
// along the axes, so it is exactly symmetric left to right and top to
// bottom).
//
// The chamber holds two layers of glass: shards fixed in it, and loose
// beads that lag behind when it turns and settle after. Turning the phone
// about its screen (the gyroscope) turns the chamber against the mirrors,
// as in a real kaleidoscope; held still, nothing moves, and turning back
// brings the pattern back.
//
// The light is computed in color: the glass filters a light behind it, so
// where pieces overlap their colors multiply into deeper hues, and where
// there is no glass the view is dark. Pieces have dark leading at their
// edges, a bright beveled rim inside it, and a faint texture; the light
// is brightest at the center. The color becomes the nearest of the
// terminal's 256, and the character's density follows the brightness.
type kaleidoscope struct {
	shards, beads []glass
	seed          int64
	turn          float64 // the chamber's angle, from the phone's turning
	shown         float64 // the angle drawn, easing after turn
	loose         float64 // the beads' angle, lagging
	auto          bool
}

type glass struct {
	x, y  float64      // center, in the chamber (radius 1)
	r     float64      // extent
	shape [][2]float64 // corners around the center; nil = round
	rgb   [3]float64   // transmitted color
}

var jewels = [][3]float64{
	{0.95, 0.12, 0.18}, // ruby
	{0.15, 0.35, 1.0},  // sapphire
	{0.1, 0.85, 0.4},   // emerald
	{1.0, 0.62, 0.1},   // amber
	{0.65, 0.25, 0.95}, // amethyst
	{0.1, 0.85, 0.85},  // turquoise
	{1.0, 0.92, 0.25},  // citrine
	{1.0, 0.4, 0.65},   // rose
}

const kalSector = math.Pi / 6 // the angle between the mirrors

func (k *kaleidoscope) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("gyroscope", 60)
	if err != nil {
		return nil, err
	}
	k.seed = int64(rand.Uint32())
	k.fill()
	return gaugesFor(st, 3), nil
}

func (k *kaleidoscope) Help() []string {
	return []string{
		"Turn the phone slowly about its screen, as you would turn a kaleidoscope: the glass turns against the mirrors and the pattern unfolds. Turn back and it comes back; hold still and it rests.",
		"a  turn by itself    r  new glass",
	}
}

func (k *kaleidoscope) Key(c byte) {
	switch c {
	case 'a':
		k.auto = !k.auto
	case 'r':
		k.seed++
		k.fill()
	}
}

// fill makes the chamber's glass: larger shards, and smaller beads and
// rods in the loose layer.
func (k *kaleidoscope) fill() {
	rng := rand.New(rand.NewSource(k.seed))
	piece := func(rmin, rmax float64, rods bool) glass {
		// Even in radius, not area: near the middle the wedge is small, and
		// would otherwise go bare. Out to the screen's corners.
		a, d := rng.Float64()*2*math.Pi, rng.Float64()*1.35
		g := glass{x: d * math.Cos(a), y: d * math.Sin(a), r: rmin + (rmax-rmin)*rng.Float64(), rgb: jewels[rng.Intn(len(jewels))]}
		turn := rng.Float64() * 2 * math.Pi
		switch kind := rng.Intn(5); {
		case kind == 0: // round
		case rods && kind <= 2: // a rod
			g.shape = [][2]float64{{-g.r, -g.r * 0.22}, {g.r, -g.r * 0.22}, {g.r, g.r * 0.22}, {-g.r, g.r * 0.22}}
		default: // a shard: 3 to 6 corners
			n := 3 + rng.Intn(4)
			for i := 0; i < n; i++ {
				b := float64(i)*2*math.Pi/float64(n) + (rng.Float64()-0.5)*0.5
				rr := g.r * (0.7 + 0.3*rng.Float64())
				g.shape = append(g.shape, [2]float64{rr * math.Cos(b), rr * math.Sin(b)})
			}
		}
		for i, p := range g.shape { // at a random angle
			c, s := math.Cos(turn), math.Sin(turn)
			g.shape[i] = [2]float64{p[0]*c - p[1]*s, p[0]*s + p[1]*c}
		}
		return g
	}
	k.shards, k.beads = nil, nil
	// Packed, as a real object cell: the wedge the mirrors show is a
	// twelfth of the chamber, and every part of it should hold glass.
	for i := 0; i < 150; i++ {
		k.shards = append(k.shards, piece(0.06, 0.17, false))
	}
	for i := 0; i < 110; i++ {
		k.beads = append(k.beads, piece(0.03, 0.08, true))
	}
}

func (k *kaleidoscope) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 6 {
		return
	}
	dt = math.Min(dt, 0.1)
	// Turning about the screen, with a dead band so the sensor's noise
	// doesn't stir it.
	if r := ss.Get("gyroscope").Read(); r.OK && len(r.V) >= 3 {
		if wz := toScreen(ss, r.V)[2]; math.Abs(wz) > 0.04 {
			k.turn += (wz - math.Copysign(0.04, wz)) * dt
		}
	}
	if k.auto {
		k.turn += 0.12 * dt
	}
	// Ease toward the targets, and land on them: resting means still.
	ease := func(x *float64, to, rate float64) {
		*x += (to - *x) * math.Min(1, dt*rate)
		if math.Abs(to-*x) < 1e-3 {
			*x = to
		}
	}
	ease(&k.shown, k.turn, 6)
	ease(&k.loose, k.shown*0.7, 2) // the beads lag, then settle
	k.render(v)
}

// transmit is the color the glass of one layer passes at chamber point
// (x, y) (white where there's none), and whether any glass is there. At a
// piece's edge the leading is dark; just inside, the bevel is bright.
func transmit(layer []glass, x, y float64) ([3]float64, bool) {
	out := [3]float64{1, 1, 1}
	hit := false
	for i := range layer {
		g := &layer[i]
		dx, dy := x-g.x, y-g.y
		if dx*dx+dy*dy > g.r*g.r*1.1 {
			continue
		}
		var edge float64 // distance inside the edge; < 0 outside
		if g.shape == nil {
			edge = g.r*0.85 - math.Hypot(dx, dy)
		} else {
			edge = polyEdge(g.shape, dx, dy)
		}
		if edge < 0 {
			continue
		}
		hit = true
		k := 1.0
		switch {
		case edge < 0.012:
			k = 0.08 // leading
		case edge < 0.04:
			k = 1.35 // the bevel catches the light
		default:
			k = 0.85 + 0.15*math.Sin(dx*40+dy*23) // a faint texture
		}
		for c := 0; c < 3; c++ {
			out[c] *= g.rgb[c] * k
		}
	}
	return out, hit
}

// polyEdge is how far (x, y) lies inside a convex polygon (< 0 outside).
func polyEdge(pts [][2]float64, x, y float64) float64 {
	sign, inside := 0.0, math.Inf(1)
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		ex, ey := b[0]-a[0], b[1]-a[1]
		d := (ex*(y-a[1]) - ey*(x-a[0])) / math.Hypot(ex, ey)
		if sign == 0 {
			sign = math.Copysign(1, d)
		}
		inside = math.Min(inside, d*sign)
	}
	return inside
}

func (k *kaleidoscope) render(v *View) {
	w, h := float64(v.W), float64(v.H)
	cx, cy := w/2, h/2
	R := math.Hypot(cx, cy*2) * 0.72 // the chamber's radius on screen, columns
	ca, sa := math.Cos(-k.shown), math.Sin(-k.shown)
	cb, sb := math.Cos(-k.loose), math.Sin(-k.loose)
	parallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			px, py := (float64(x)+0.5-cx)/R, (cy-float64(y)-0.5)*2/R
			r := math.Hypot(px, py)
			// Fold into the wedge between the mirrors.
			phi := math.Mod(math.Atan2(py, px)+4*math.Pi, 2*kalSector)
			if phi > kalSector {
				phi = 2*kalSector - phi
			}
			fx, fy := r*math.Cos(phi), r*math.Sin(phi)
			// The two layers, each turned by its own angle.
			a, ha := transmit(k.shards, fx*ca-fy*sa, fx*sa+fy*ca)
			b, hb := transmit(k.beads, fx*cb-fy*sb, fx*sb+fy*cb)
			light := 1.15 * (1 - 0.55*smoothstep(0.1, 1.25, r)) // brightest in the middle
			var rgb [3]float64
			if ha || hb {
				for c := 0; c < 3; c++ {
					rgb[c] = a[c] * b[c] * light
				}
			} else {
				// No glass: frosted, faintly violet, with a grain.
				fr := 0.1 + 0.05*math.Sin(fx*57+fy*31)*math.Sin(fx*23-fy*41)
				rgb = [3]float64{fr * 0.8 * light, fr * 0.55 * light, fr * 1.3 * light}
			}
			lum := 0.3*rgb[0] + 0.55*rgb[1] + 0.15*rgb[2]
			const ramp = " .:-=+*#%@"
			ch := ramp[min(len(ramp)-1, int(math.Sqrt(clamp01(lum))*float64(len(ramp)-1)+0.5))]
			if ch == ' ' {
				continue
			}
			v.Set(x, y, ch, rgb256(rgb))
		}
	})
}

// rgb256 is the terminal color nearest an RGB color (0..1): from the 6x6x6
// cube, or the gray ramp for grays.
func rgb256(c [3]float64) uint8 {
	levels := [6]float64{0, 95, 135, 175, 215, 255}
	var idx [3]int
	var cube [3]float64
	for i := 0; i < 3; i++ {
		v := clamp01(c[i]) * 255
		best := 0
		for j := 1; j < 6; j++ {
			if math.Abs(levels[j]-v) < math.Abs(levels[best]-v) {
				best = j
			}
		}
		idx[i], cube[i] = best, levels[best]
	}
	col := uint8(16 + 36*idx[0] + 6*idx[1] + idx[2])
	// A gray may be nearer.
	avg := (clamp01(c[0]) + clamp01(c[1]) + clamp01(c[2])) / 3 * 255
	g := int(math.Round((avg - 8) / 10))
	g = max(0, min(23, g))
	gv := float64(8 + 10*g)
	dist := func(a [3]float64) float64 {
		s := 0.0
		for i := 0; i < 3; i++ {
			d := a[i] - clamp01(c[i])*255
			s += d * d
		}
		return s
	}
	if dist([3]float64{gv, gv, gv}) < dist(cube) {
		return uint8(232 + g)
	}
	return col
}
