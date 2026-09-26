package main

import "math"

// The hourglass's frame and glass, ray marched once per screen size and
// drawn around the sand. The camera looks from a little above (artElev),
// so the tops of the bases, collars and rings show as lit ellipses. The
// glass is modeled so its outline on screen is the profile the sand is
// simulated in: its radius at height y is the sand's half width on the
// screen row where that height lands.
//
// World coordinates are sand cells: x right, y down, z toward the viewer,
// the origin at the neck. The frame is turned walnut and brass: two bases
// (a knurled brass collar gripping the glass, a rounded plate, a brass
// bead, a fluted cove, the wide main plate with an inlaid brass band, a
// dentil foot, three bun feet) joined by two turned spindles, and a brass
// ring at the neck.

type artCell struct {
	ch    byte
	col   uint8
	front bool // a solid in front of the glass: drawn over the sand
}

// Parts of the frame, each with its material and carving.
const (
	artNone   = iota
	artCollar // brass, knurled
	artPlate  // wood
	artBead   // brass beading
	artCove   // wood, fluted
	artMain   // wood, with an inlaid brass band
	artFoot   // end grain, dentils
	artBun    // the bun feet
	artPost   // wood
	artRing   // brass ring at the neck
)

const artElev = 14 * math.Pi / 180

var (
	// Warm wood, as the snow globe's base: dark red through brown to peach.
	artWoodRamp  = []uint8{232, 52, 88, 94, 130, 137, 173, 180, 223} // one hue throughout
	artEndRamp   = []uint8{232, 233, 52, 88, 94, 130, 137}
	artBrassRamp = []uint8{94, 130, 136, 172, 178, 214, 220, 221, 222, 229, 230, 231}
	artLight     = Vec3{-0.62, -0.55, 0.55}.Norm() // strong, from the left and above (y down)
	artFill      = Vec3{0.7, -0.2, 0.7}.Norm()     // a soft fill from the right
	// Lights in the room the glass mirrors: a window upper left, a lamp right.
	artRoom = []Vec3{Vec3{-0.95, -0.2, 0.55}.Norm(), Vec3{0.9, -0.1, 0.35}.Norm()}
)

// frame holds the frame's measures, in cells.
type frame struct {
	hb, rb, rg, rp float64 // base height, base radius, collar radius, post radius
	yTop, yBot     float64 // the glass's ends
	neckR          float64
	postA, postB   float64
	lamp           Vec3 // the light: a lamp above, left, in front
	view, down     Vec3 // camera direction, and screen down, in world
	ync            float64
	g              *hourglass
}

func sdBox2(px, py, hx, hy float64) float64 {
	dx, dy := math.Abs(px)-hx, math.Abs(py)-hy
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0)
}

// ring is a turned disc: radius R from the axis over heights u0..u1, with
// rounded edges.
func ring(r, u, R, u0, u1, rnd float64) float64 {
	return sdBox2(r, u-(u0+u1)/2, R-rnd, (u1-u0)/2-rnd) - rnd
}

// base is one base's profile, turned: r from the axis, u the height away
// from the glass's end.
func (f *frame) base(r, u float64) (float64, int) {
	h, R := f.hb, f.rb
	d, m := math.Inf(1), artNone
	add := func(dd float64, mm int) {
		if dd < d {
			d, m = dd, mm
		}
	}
	// Few, generous forms: at a terminal's resolution small ones blur
	// into stripes.
	// The collar is a ring gripping the glass's end; inside it, seen
	// through the glass, the base's wooden face.
	collar := artCollar
	if r < f.rg-3 {
		collar = artPlate
	}
	add(ring(r, u, f.rg, -0.8, 0.13*h, 0.6), collar)
	add(ring(r, u, 0.7*R, 0.11*h, 0.26*h, 0.06*h), artCove)
	add(ring(r, u, R, 0.22*h, 0.62*h, 0.13*h), artMain)
	add(math.Hypot(r-0.78*R, u-0.22*h)-0.055*h, artBead)
	add(ring(r, u, 0.9*R, 0.6*h, 0.8*h, 0.05*h), artFoot)
	return d, m
}

func bump(t, c, w float64) float64 { return math.Exp(-math.Pow((t-c)/w, 2)) }

// spindle is a post's radius along it, t from 0 to 1: balls at the ends,
// a bead in the middle, rings at the quarters, a slight swell.
func spindle(t float64) float64 {
	return 2.0 + 0.4*math.Sin(math.Pi*t) +
		2.2*(bump(t, 0.05, 0.035)+bump(t, 0.95, 0.035)) +
		2.0*bump(t, 0.5, 0.03) +
		1.3*(bump(t, 0.27, 0.018)+bump(t, 0.73, 0.018))
}

// height is how far into a base a point is, from the glass's end outward.
func (f *frame) height(y float64) float64 {
	if y < 0 {
		return f.yTop - y
	}
	return y - f.yBot
}

func (f *frame) sdf(p Vec3) (float64, int) {
	r := math.Hypot(p[0], p[2])
	u := f.height(p[1])
	d, m := f.base(r, u)
	// Three bun feet at the outer end, one in front.
	for k := 0; k < 3; k++ {
		a := math.Pi/2 + float64(k)*2*math.Pi/3
		fx, fz := 0.66*f.rb*math.Cos(a), 0.66*f.rb*math.Sin(a)
		rx, ry := 0.15*f.rb, 0.11*f.hb
		q := Vec3{(p[0] - fx) / rx, (u - 0.88*f.hb) / ry, (p[2] - fz) / rx}
		if db := (q.Len() - 1) * math.Min(rx, ry); db < d {
			d, m = db, artBun
		}
	}
	// The posts.
	if p[1] > f.postA-3 && p[1] < f.postB+3 {
		t := (p[1] - f.postA) / (f.postB - f.postA)
		for _, px := range []float64{-f.rp, f.rp} {
			dp := (math.Hypot(p[0]-px, p[2]) - spindle(clamp01(t))) * 0.7
			dp = math.Max(dp, math.Max(f.postA-p[1], p[1]-f.postB))
			if dp < d {
				d, m = dp, artPost
			}
		}
	}
	// The ring at the neck.
	if dn := math.Hypot(r-(f.neckR+2.2), p[1]) - 1.8; dn < d {
		d, m = dn, artRing
	}
	return d, m
}

func (f *frame) normal(p Vec3) Vec3 {
	const e = 0.2
	s := func(q Vec3) float64 { d, _ := f.sdf(q); return d }
	return Vec3{
		s(p.Add(Vec3{e, 0, 0})) - s(p.Sub(Vec3{e, 0, 0})),
		s(p.Add(Vec3{0, e, 0})) - s(p.Sub(Vec3{0, e, 0})),
		s(p.Add(Vec3{0, 0, e})) - s(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

// glassR is the glass's inner radius at world height y: the sand's half
// width on the screen row that height lands on.
func (f *frame) glassR(y float64) float64 {
	return f.g.profile(f.ync + y*math.Cos(artElev))
}

// renderArt ray marches the frame and the glass into g.art.
func (g *hourglass) renderArt() {
	ce, se := math.Cos(artElev), math.Sin(artElev)
	f := &frame{
		rb:    g.baseR(),
		neckR: g.neck,
		view:  Vec3{0, se, -ce},
		down:  Vec3{0, ce, se},
		ync:   float64(g.nr) + 0.5,
		g:     g,
	}
	f.yTop = (float64(g.top) - f.ync) / ce
	f.yBot = (float64(g.bot+1) - f.ync) / ce
	// The bases fill the rows beyond the glass, less the depth of their
	// ellipses (the top base's back, the bottom base's front).
	f.hb = math.Max(8, (float64(g.baseRows*hgSY)-f.rb*se-1)/ce)
	f.rg = g.profile(float64(g.top)) + 2.4
	f.rp = 0.76 * f.rb
	f.postA, f.postB = f.yTop-0.25*f.hb, f.yBot+0.25*f.hb
	f.lamp = Vec3{-1.3 * f.rb, f.yTop - f.hb - 2.2*f.rb, 2.2 * f.rb}
	g.art = make([]artCell, g.w*g.h)
	parallelRows(g.h, func(cy int) {
		for cx := 0; cx < g.w; cx++ {
			x := (float64(cx)+0.5)*hgSX - g.cxF
			s := (float64(cy)+0.5)*hgSY - f.ync
			g.art[cy*g.w+cx] = f.shade(x, s)
		}
	})
	g.drawGlassOutline()
}

// shade renders the frame, or the glass's reflections, at screen position
// (x, s): x in cells from the axis, s in cells below the neck.
func (f *frame) shade(x, s float64) artCell {
	const far = 400
	o := Vec3{x, 0, 0}.Add(f.down.Scale(s)).Sub(f.view.Scale(far))
	t := far - f.rb - 10
	hitT := math.Inf(1)
	var hit Vec3
	m := artNone
	for i := 0; i < 120 && t < far+f.rb+10; i++ {
		p := o.Add(f.view.Scale(t))
		d, mm := f.sdf(p)
		if d < 0.04 {
			hitT, hit, m = t, p, mm
			break
		}
		t += math.Max(d, 0.04)
	}

	// Where the ray meets the front of the glass, if it does.
	glassT := math.Inf(1)
	var gn Vec3
	if y := s / math.Cos(artElev); y > f.yTop && y < f.yBot {
		R := f.glassR(y) + 0.9
		if math.Abs(x) < R {
			zf := math.Sqrt(R*R - x*x)
			glassT = far + y*math.Sin(artElev) - zf*math.Cos(artElev)
			gn = Vec3{x / R, 0, zf / R}
		}
	}

	if hitT < math.Inf(1) {
		c := f.shadeSolid(hit, m)
		c.front = hitT < glassT
		return c
	}
	if glassT < math.Inf(1) {
		// The room's lights mirrored in the curved glass: streaks that
		// follow its curve.
		// The glass's normal is taken level, so only the lights'
		// direction around it matters.
		best := 0.0
		for _, l := range artRoom {
			hv := l.Sub(f.view)
			hv[1] = 0
			best = math.Max(best, gn.Dot(hv.Norm()))
		}
		switch {
		case best > 0.9993:
			return artCell{'|', 231, false}
		case best > 0.998:
			return artCell{':', 110, false}
		}
		// Seen at a grazing angle near its edges, the glass shows its
		// thickness: a faint rim of light.
		if gn[2] < 0.45 {
			return artCell{'.', 60, false}
		}
	}
	return artCell{' ', 0, false}
}

func (f *frame) shadeSolid(p Vec3, m int) artCell {
	n := f.normal(p)
	eye := f.view.Scale(-1)
	// A lamp at a finite distance: light falls off across flat faces, and
	// the varnish shows its reflection as a glossy spot.
	toLamp := f.lamp.Sub(p)
	dist := toLamp.Len()
	L := toLamp.Scale(1 / dist)
	mid := f.lamp.Len() // the lamp's distance to the middle
	fall := math.Min(1.25, 0.35+0.75*mid/dist)
	diff := math.Max(0, n.Dot(L)) * fall
	fill := 0.25 * math.Max(0, n.Dot(artFill))
	spec := math.Pow(math.Max(0, n.Dot(L.Add(eye).Norm())), 40) * fall
	u := f.height(p[1])
	// Turned wood: rings of grain along the axis, and on faces that face
	// up or down, concentric rings.
	grain := 0.0 // the characters show the grain
	wood := 0.08 + 0.7*diff + fill*0.5 + 0.6*spec + grain
	metal := 0.3 + 0.5*diff + fill*0.5 + 0.9*spec // polished: never dull
	i, ramp := wood, artWoodRamp
	switch m {
	case artCollar, artRing, artBead:
		i, ramp = metal, artBrassRamp
	case artMain:
		if uu := u / f.hb; uu > 0.38 && uu < 0.46 && n[1] > -0.6 && n[1] < 0.6 {
			i, ramp = metal, artBrassRamp // the inlaid band
		}
	case artBun:
		i = wood * 0.92
	}
	// Dense characters, as the snow globe's base, so the forms read
	// solid; a strong light sweeps each from bright to dark.
	const solid = "=+#%@"
	ch := solid[int(clamp01(i)*float64(len(solid)-1)+0.5)]
	return artCell{ch, mzPick(ramp, i), false}
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// drawGlassOutline draws the glass's walls as unbroken lines: in each row,
// every character from where the wall was at the end of the row above to
// where it is at the end of this one, in the columns just outside the
// sand. A fainter line outside shows the glass's thickness.
func (g *hourglass) drawGlassOutline() {
	for side := -1; side <= 1; side += 2 {
		prev := math.NaN()
		for cy := g.top / hgSY; cy <= g.bot/hgSY; cy++ {
			lo, hi := math.Inf(1), math.Inf(-1)
			first, last := math.NaN(), math.NaN()
			for sy := 0; sy < hgSY; sy++ {
				y := cy*hgSY + sy
				if y < g.top || y > g.bot {
					continue
				}
				c := float64(g.wallCol(y, side))
				if math.IsNaN(first) {
					first = c
				}
				last = c
				lo, hi = math.Min(lo, c), math.Max(hi, c)
			}
			if math.IsNaN(first) {
				continue
			}
			if !math.IsNaN(prev) {
				lo, hi = math.Min(lo, prev), math.Max(hi, prev)
				first = prev
			}
			prev = last
			ch := byte('|')
			switch mv := last - first; {
			case mv > 0:
				ch = '\\'
			case mv < 0:
				ch = '/'
			}
			for c := int(lo); c <= int(hi); c++ {
				if o := c + side; o >= 0 && o < g.w && !g.art[cy*g.w+o].front {
					g.art[cy*g.w+o] = artCell{ch, 60, false}
				}
			}
			for c := int(lo); c <= int(hi); c++ {
				if c >= 0 && c < g.w && !g.art[cy*g.w+c].front {
					g.art[cy*g.w+c] = artCell{ch, 152, false}
				}
			}
		}
	}
}

// wallCol is the character column just outside the sand on cell row y:
// the first one on that side holding no sand cell.
func (g *hourglass) wallCol(y, side int) int {
	hw := g.halfW[y]
	if side > 0 {
		xr := int(math.Ceil(g.cxF+hw-0.5)) - 1 // rightmost sand cell
		return xr/hgSX + 1
	}
	xl := int(math.Floor(g.cxF-hw-0.5)) + 1 // leftmost sand cell
	return xl/hgSX - 1
}
