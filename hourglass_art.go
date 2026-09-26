package main

import "math"

// The hourglass's frame and glass, ray marched once per screen size and
// drawn under the sand. The view is straight on and orthographic, so the
// glass's outline is exactly the profile the sand is simulated in.
//
// Coordinates are sand cells: x from the axis, y down the screen (as the
// sand grid), z toward the viewer. The frame is turned wood: two ornate
// bases (a brass collar gripping the glass, a rounded plate, a brass bead,
// a cove, the wide main plate, a foot band) joined by two turned spindles,
// and a brass ring at the neck.

type artCell struct {
	ch    byte
	col   uint8
	solid bool
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

var (
	artWoodRamp  = []uint8{52, 52, 88, 94, 130, 136, 173, 179, 180}
	artEndRamp   = []uint8{234, 52, 52, 88, 94, 130, 136}
	artBrassRamp = []uint8{58, 94, 136, 178, 220, 221, 228, 230, 231}
	artGlassRamp = []uint8{60, 67, 110, 152, 195}
	artLight     = Vec3{-0.55, -0.6, 0.6}.Norm() // upper left, in front (y down)
)

// frame holds the frame's measures, in cells.
type frame struct {
	hb     float64 // each base's height
	rb     float64 // the main plate's radius
	rg     float64 // the collar's radius
	rp     float64 // where the posts stand
	top    float64 // the glass's top and bottom rows
	bot    float64
	neckY  float64
	neckR  float64
	postA  float64 // the posts' ends
	postB  float64
	shells []float64 // glass inner radius per cell row
}

func sdBox2(px, py, hx, hy float64) float64 {
	dx, dy := math.Abs(px)-hx, math.Abs(py)-hy
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0)
}

// ring is a turned disc: radius r from the axis over heights u0..u1, with
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
	// Spaced so that on a small screen, where a base gets seven rows, each
	// row lands on its own part.
	add(ring(r, u, f.rg, -0.5, 0.14*h, 0.6), artCollar)
	add(ring(r, u, 0.86*R, 0.14*h, 0.29*h, 1.2), artPlate)
	add(math.Hypot(r-(0.86*R-0.6), u-0.36*h)-0.065*h, artBead)
	add(ring(r, u, 0.74*R, 0.36*h, 0.45*h, 0.6), artCove)
	add(ring(r, u, R, 0.43*h, 0.72*h, 1.6), artMain)
	add(ring(r, u, 0.93*R, 0.71*h, 0.86*h, 0.8), artFoot)
	return d, m
}

func bump(t, c, w float64) float64 { return math.Exp(-math.Pow((t-c)/w, 2)) }

// spindle is a post's radius along it, t from 0 to 1: balls at the ends,
// a bead in the middle, rings at the quarters, a slight swell.
func spindle(t float64) float64 {
	return 2.0 + 0.3*math.Sin(math.Pi*t) +
		1.6*(bump(t, 0.05, 0.03)+bump(t, 0.95, 0.03)) +
		1.3*bump(t, 0.5, 0.025) +
		0.9*(bump(t, 0.27, 0.014)+bump(t, 0.73, 0.014))
}

func (f *frame) sdf(p Vec3) (float64, int) {
	r := math.Hypot(p[0], p[2])
	var d float64
	var m int
	u := p[1] - f.bot
	if p[1] < f.neckY {
		u = f.top - p[1]
	}
	d, m = f.base(r, u)
	// Three bun feet at the outer end, one in front.
	for k := 0; k < 3; k++ {
		a := math.Pi/2 + float64(k)*2*math.Pi/3
		fx, fz := 0.66*f.rb*math.Cos(a), 0.66*f.rb*math.Sin(a)
		rx, ry := 0.14*f.rb, 0.08*f.hb
		q := Vec3{(p[0] - fx) / rx, (u - 0.92*f.hb) / ry, (p[2] - fz) / rx}
		if db := (q.Len() - 1) * math.Min(rx, ry); db < d {
			d, m = db, artBun
		}
	}
	// The posts.
	if p[1] > f.postA-2 && p[1] < f.postB+2 {
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
	if dn := math.Hypot(r-(f.neckR+2), p[1]-f.neckY) - 1.7; dn < d {
		d, m = dn, artRing
	}
	return d, m
}

// turnedChar picks a character by which way the surface faces: a rounded
// edge facing up is = and one facing down _, and the front is shaded. (The
// sides, where the form curves away, get ( and ) after rendering.)
func turnedChar(n Vec3, i float64) byte {
	switch {
	case n[1] < -0.55:
		return '='
	case n[1] > 0.55:
		return '_'
	}
	return shadeChar(0.15 + 0.8*i)
}

func (f *frame) normal(p Vec3) Vec3 {
	const e = 0.25
	s := func(q Vec3) float64 { d, _ := f.sdf(q); return d }
	return Vec3{
		s(p.Add(Vec3{e, 0, 0})) - s(p.Sub(Vec3{e, 0, 0})),
		s(p.Add(Vec3{0, e, 0})) - s(p.Sub(Vec3{0, e, 0})),
		s(p.Add(Vec3{0, 0, e})) - s(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

// renderArt ray marches the frame and the glass into g.art.
func (g *hourglass) renderArt() {
	f := &frame{
		hb:    float64(g.baseRows * hgSY),
		rb:    g.baseR(),
		top:   float64(g.top),
		bot:   float64(g.bot + 1),
		neckY: float64(g.nr),
		neckR: g.neck,
	}
	f.rg = g.profile(float64(g.top)) + 2.2
	f.rp = 0.76 * f.rb
	f.postA, f.postB = f.top-0.25*f.hb, f.bot+0.25*f.hb
	g.art = make([]artCell, g.w*g.h)
	zs := f.rb + 6
	parallelRows(g.h, func(cy int) {
		row := g.art[cy*g.w : (cy+1)*g.w]
		for cx := range row {
			x := (float64(cx)+0.5)*hgSX - g.cxF
			y := (float64(cy) + 0.5) * hgSY
			row[cx] = g.shadeArt(f, x, y, zs)
		}
		// Each turned form's run in a row starts with ( and ends with ).
		for cx := 0; cx < len(row); cx++ {
			if !row[cx].solid || cx > 0 && row[cx-1].solid {
				continue
			}
			end := cx
			for end+1 < len(row) && row[end+1].solid {
				end++
			}
			if end > cx {
				row[cx].ch, row[end].ch = '(', ')'
			} else {
				row[cx].ch = '|'
			}
		}
	})
}

func (g *hourglass) shadeArt(f *frame, x, y, zs float64) artCell {
	// Solids first: they're in front of or beside the glass.
	z := zs
	u := y - f.bot // height into the base, for the inlay
	if y < f.neckY {
		u = f.top - y
	}
	for i := 0; i < 80 && z > -zs; i++ {
		d, m := f.sdf(Vec3{x, y, z})
		if d < 0.05 {
			n := f.normal(Vec3{x, y, z})
			diff := math.Max(0, n.Dot(artLight))
			h := artLight.Add(Vec3{0, 0, 1}).Norm()
			spec := math.Pow(math.Max(0, n.Dot(h)), 24)
			// Turned wood shows rings of grain along the axis.
			grain := 0.07 * math.Sin(y*1.7+2*math.Sin(y*0.37))
			var i float64
			ramp := artWoodRamp
			// Around the axis, for the carving's rhythm.
			phi := math.Atan2(x, math.Max(z, 0.01))
			var ch byte
			switch m {
			case artCollar, artRing:
				i, ramp = 0.12+0.55*diff+0.8*spec, artBrassRamp
				if m == artCollar && math.Cos(phi*40) > 0.4 {
					ch = '#' // knurling
				}
			case artBead:
				i, ramp = 0.15+0.55*diff+0.9*spec, artBrassRamp
				ch = "oO"[boolIdx(math.Cos(phi*30) > 0)]
			case artCove:
				i = 0.1 + 0.62*diff + 0.3*spec + grain
				if math.Cos(phi*26) > 0.5 { // flutes, in shadow
					ch, i = '|', i*0.6
				}
			case artMain:
				i = 0.12 + 0.62*diff + 0.35*spec + grain
				if uu := u / f.hb; uu > 0.49 && uu < 0.62 { // the inlay
					ramp, i, ch = artBrassRamp, 0.2+0.55*diff+0.7*spec, '~'
				}
			case artFoot:
				i, ramp = 0.1+0.6*diff+0.2*spec+grain, artEndRamp
				ch = "[]"[boolIdx(math.Sin(phi*22) > 0)]
			case artBun:
				i, ramp = 0.1+0.6*diff+0.3*spec, artEndRamp
			default:
				i = 0.12 + 0.62*diff + 0.35*spec + grain
			}
			if ch == 0 {
				ch = turnedChar(n, i)
			}
			return artCell{ch, mzPick(ramp, i), true}
		}
		z -= math.Max(d, 0.05)
	}
	// The glass: a thin shell of revolution just outside the sand.
	if y < f.top || y >= f.bot {
		return artCell{' ', 0, false}
	}
	r0 := g.profile(y)
	const th = 0.9
	R := r0 + th
	ax := math.Abs(x)
	if ax > R+0.5 {
		return artCell{' ', 0, false}
	}
	if ax >= r0-0.5 { // the wall seen edge on
		slope := (g.profile(y+1) - g.profile(y-1)) / 2 // dr/dy
		dx := slope * hgSY / hgSX                      // in characters per row
		if x < 0 {
			dx = -dx
		}
		ch := lineChar(dx, 1)
		return artCell{ch, artGlassRamp[3], false}
	}
	// Across the face: a streak where the curved glass mirrors the light.
	zf := math.Sqrt(R*R - ax*ax)
	n := Vec3{x / R, 0, zf / R}
	hv := artLight.Add(Vec3{0, 0, 1}).Norm()
	switch s := n.Dot(hv); {
	case s > 0.997:
		return artCell{'|', artGlassRamp[4], false}
	case s > 0.985:
		return artCell{':', artGlassRamp[2], false}
	}
	return artCell{' ', 0, false}
}
