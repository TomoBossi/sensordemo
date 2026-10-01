package flicker

import (
	"math"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// The bulb, floating free, upright, in scene units: the classic shape, a
// round globe of clear glass whose neck curves in (concave) down to a
// brass screw base, and under the base the black glass insulator and the
// contact.
const (
	globeY  = 0.45 // the globe's center; its radius is 1, its apex at 1.45
	neckBot = -1.05
	neckR   = 0.42 // the neck's radius where it meets the base
	baseR   = 0.44
	baseBot = -1.72
	insBot  = -1.88

	viewD  = 7.0
	viewEl = 6 * math.Pi / 180
	frameY = -0.25 // where the view centers: the bulb, base and all
	bob    = 0.06  // how far it rises and sinks as it floats
)

// The squirrel cage, upright: strands zigzag between hooks on a wide ring
// at the top and a small one at the bottom, round the glass stem that
// rises from the neck.
const (
	cageN    = 5
	cageTop  = 0.8
	cageBot  = -0.3
	cageRt   = 0.5
	cageRb   = 0.14
	strandR  = 0.014
	stemFoot = -1.0
	stemTop  = 0.86
)

// filCenter is where the filament's light seems to come from.
var filCenter = Vec3{0, 0.3, 0}

// Two lights in the room, fixed in it, for the glints on the glass and the
// metal: a window up left, a lamp to the right.
var (
	roomLights = []Vec3{Vec3{-0.45, 0.75, 0.5}.Norm(), Vec3{0.65, 0.45, 0.6}.Norm()}
	keyLight   = Vec3{-0.5, 0.65, 0.55}.Norm()
)

var (
	// The filament, from a dull red glow to white-hot.
	heatRamp = []uint8{52, 88, 124, 160, 196, 202, 208, 214, 220, 221, 228, 229, 230, 231}
	// The wall behind it, lit, as the candle's.
	wallRamp = []uint8{233, 52, 52, 88, 94, 130, 166, 172, 178}
	// Its haze in the glass.
	hazeRamp = []uint8{52, 52, 88, 94, 130, 136, 172, 178, 214}
	// The glass's edge: cool when dark, warmed by the filament when lit.
	glassCool = []uint8{236, 238, 60, 67, 110, 152, 195}
	glassWarm = []uint8{94, 136, 179, 180, 223, 230}
	brassRamp = []uint8{58, 94, 100, 136, 142, 178, 179, 186, 222, 229}
	blackRamp = []uint8{233, 234, 235, 236, 238, 240, 243, 247}
	wireRamp  = []uint8{238, 240, 243, 246, 249}
)

// bayer4 orders a 4x4 cell's thresholds, for dithering.
var bayer4 = [16]float64{0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5}

func dither(x, y int) float64 { return (bayer4[(y%4)*4+x%4] + 0.5) / 16 }

// Parts.
const (
	pNone = iota
	pGlass
	pBrass
	pBlack // the insulator under the base
	pTip   // the contact
	pStrand
	pStem
	pWire // the hooks' arms
)

// roundCone is the distance to a cone between radius r1 at y = 0 and r2 at
// y = h, its ends rounded.
func roundCone(p Vec3, r1, r2, h float64) float64 {
	qx, qy := math.Hypot(p[0], p[2]), p[1]
	b := (r1 - r2) / h
	a := math.Sqrt(1 - b*b)
	k := -b*qx + a*qy
	switch {
	case k < 0:
		return math.Hypot(qx, qy) - r1
	case k > a*h:
		return math.Hypot(qx, qy-h) - r2
	}
	return qx*a + qy*b - r1
}

// glassSDF is the outside of the glass: the globe, and the neck, whose
// radius eases from the base's out to the globe's in a concave curve, as a
// classic bulb's does; the two blended smooth.
func glassSDF(p Vec3) float64 {
	globe := Vec3{p[0], p[1] - globeY, p[2]}.Len() - 1
	// The neck ends inside the globe, narrower than it there, so where the
	// two meet there is no edge, only the blend.
	const neckTop, topR = -0.2, 0.68 // the globe is 0.76 across here
	u := Clamp01((p[1] - neckBot) / (neckTop - neckBot))
	r := neckR + (topR-neckR)*math.Pow(u, 1.7)
	neck := math.Max((math.Hypot(p[0], p[2])-r)*0.8, math.Max(neckBot-0.02-p[1], p[1]-neckTop))
	return Smin(globe, neck, 0.3)
}

// outer is the bulb from outside: the glass, the base, the insulator and
// the contact.
func outer(p Vec3) (float64, int) {
	d, m := glassSDF(p), pGlass
	if p[1] > neckBot+0.15 {
		return d, m
	}
	r := math.Hypot(p[0], p[2])
	th := math.Atan2(p[2], p[0])
	// The screw: a helix of thread round the shell.
	thread := 0.03 * math.Sin((p[1]-neckBot)*2*math.Pi/0.12+th)
	base := math.Max(r-baseR-thread, math.Max(p[1]-neckBot-0.04, baseBot-p[1]))
	if base < d {
		d, m = base, pBrass
	}
	// The insulator: black glass narrowing under the base.
	ir := 0.36 - 0.12*Clamp01((baseBot-p[1])/(baseBot-insBot))
	if ins := math.Max(r-ir, math.Max(p[1]-baseBot-0.01, insBot-p[1])); ins < d {
		d, m = ins, pBlack
	}
	if tip := (Vec3{p[0], p[1] - insBot, p[2]}).Len() - 0.12; tip < d {
		d, m = tip, pTip
	}
	return d, m
}

func outerNormal(p Vec3) Vec3 {
	const e = 0.002
	var n Vec3
	for _, k := range [4]Vec3{{1, -1, -1}, {-1, -1, 1}, {-1, 1, -1}, {1, 1, 1}} {
		d, _ := outer(p.Add(k.Scale(e)))
		n = n.Add(k.Scale(d))
	}
	return n.Norm()
}

// seg is the distance from p to the segment a-b, and how far along it p's
// nearest point is.
func seg(p, a, b Vec3) (float64, float64) {
	ab, ap := b.Sub(a), p.Sub(a)
	h := Clamp01(ap.Dot(ab) / ab.Dot(ab))
	return ap.Sub(ab.Scale(h)).Len(), h
}

// cage is the inside: the strands (and how far from them, for their glow),
// the stem and the hooks' arms, turned by twist. It returns the distance
// to the nearest, which it is, the distance to the nearest strand, and the
// direction of what it found, for drawing it as a line.
type cagePart struct {
	d, strand float64
	part      int
	along     Vec3
	mid       float64 // for a strand: 1 at its middle, 0 at its hooks
}

func (s *flicker) cage(p Vec3) cagePart {
	c, sn := math.Cos(s.twist), math.Sin(s.twist)
	q := Vec3{c*p[0] - sn*p[2], p[1], sn*p[0] + c*p[2]}
	out := cagePart{d: math.Inf(1), strand: math.Inf(1)}
	try := func(d float64, part int, along Vec3, mid float64) {
		if part == pStrand && d < out.strand {
			out.strand = d
		}
		if d < out.d {
			out.d, out.part, out.along, out.mid = d, part, along, mid
		}
	}
	// The stem, and its flare where it meets the neck.
	r := math.Hypot(q[0], q[2])
	flare := 0.03 + 0.05*Smoothstep(-0.55, stemFoot, q[1])
	try(math.Max(r-flare, math.Max(stemFoot-q[1], q[1]-stemTop)), pStem, Vec3{0, 1, 0}, 0)
	// Only the strands near p's angle need looking at.
	ang := math.Atan2(q[2], q[0])
	k0 := int(math.Floor(ang/(2*math.Pi)*cageN+cageN)) % cageN
	for dk := -1; dk <= 1; dk++ {
		k := (k0 + dk + cageN) % cageN
		a0 := (float64(k) + 0.5) * 2 * math.Pi / cageN // a top hook
		a1 := a0 + math.Pi/cageN                       // the bottom hook after it
		a2 := a0 + 2*math.Pi/cageN                     // the next top hook
		top0 := Vec3{cageRt * math.Cos(a0), cageTop, cageRt * math.Sin(a0)}
		bot := Vec3{cageRb * math.Cos(a1), cageBot, cageRb * math.Sin(a1)}
		top1 := Vec3{cageRt * math.Cos(a2), cageTop, cageRt * math.Sin(a2)}
		// Each strand bows out a little, as a hot filament sags: two pieces
		// meeting at a point pushed outward.
		for _, pr := range [2][2]Vec3{{top0, bot}, {bot, top1}} {
			mid := pr[0].Add(pr[1]).Scale(0.5)
			out2 := Vec3{mid[0], 0, mid[2]}.Norm().Scale(0.07)
			mid = mid.Add(out2)
			for half, sg := range [2][2]Vec3{{pr[0], mid}, {mid, pr[1]}} {
				d, h := seg(q, sg[0], sg[1])
				along := h // 0 at the hook, 1 at the middle
				if half == 1 {
					along = 1 - h
				}
				try(d-strandR, pStrand, sg[1].Sub(sg[0]), along)
			}
		}
		// The hooks' arms, from the stem out to each hook.
		d, _ := seg(q, Vec3{0, cageTop + 0.02, 0}, top0)
		try(d-0.008, pWire, top0, 0)
		d, _ = seg(q, Vec3{0, cageBot - 0.04, 0}, bot)
		try(d-0.008, pWire, Vec3{bot[0], 0, bot[2]}, 0)
	}
	// Directions back to the scene's frame, for the screen.
	out.along = Vec3{c*out.along[0] + sn*out.along[2], out.along[1], -sn*out.along[0] + c*out.along[2]}
	return out
}

// view is the camera this frame.
type view struct {
	o, F, R, U Vec3
	tanX, tanY float64
	w, h       int
}

func (s *flicker) camera(w, h int) view {
	E := s.eye
	F := E.Scale(-1)
	R := F.Cross(Vec3{0, 1, 0}).Norm()
	U := R.Cross(F)
	// Fit the bulb, base and socket, top to bottom, and its width across.
	tanY := 1.95 / viewD
	tanX := tanY * float64(w) / (2 * float64(h))
	if need := 1.25 / viewD; tanX < need {
		tanY *= need / tanX
		tanX = need
	}
	// Floating: the bulb rises and sinks slowly (the view, the other way).
	y := frameY - bob*math.Sin(s.t*2*math.Pi/5)
	return view{o: Vec3{0, y, 0}.Add(E.Scale(viewD)), F: F, R: R, U: U, tanX: tanX, tanY: tanY, w: w, h: h}
}

// screen projects a direction (from anywhere) to a screen-true 2D one, for
// LineChar.
func (c view) screen(d Vec3) (float64, float64) { return d.Dot(c.R), -d.Dot(c.U) * 0.5 }

func (s *flicker) render(v *View) {
	w, h := v.W, v.H
	if w < 8 || h < 6 {
		return
	}
	cam := s.camera(w, h)
	g := s.glow
	// Where the filament sits on screen, for the halo round it.
	fc := filCenter.Sub(cam.o)
	fx := (fc.Dot(cam.R)/fc.Dot(cam.F)/cam.tanX + 1) / 2 * float64(w)
	fy := (1 - fc.Dot(cam.U)/fc.Dot(cam.F)/cam.tanY) / 2 * float64(h)
	ParallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/float64(w)*2 - 1
			vv := 1 - (float64(y)+0.5)/float64(h)*2
			dir := cam.F.Add(cam.R.Scale(u * cam.tanX)).Add(cam.U.Scale(vv * cam.tanY)).Norm()
			ch, col := s.shade(cam, dir, x, y)
			if ch == 0 && g > 0.02 {
				// The wall behind, lit by the bulb as the candle lights
				// its own: sparse warm dots, brightest close round it,
				// spreading as far as its light reaches, pulsing with it.
				dx, dy := (float64(x)-fx)/float64(w)*2*cam.tanX*viewD, (float64(y)-fy)/float64(h)*2*cam.tanY*viewD
				lit := g * 0.95 / (1 + (dx*dx+dy*dy)*0.55)
				if (x+y)%2 == 0 && lit > 0.06 {
					ch, col = ".:"[BoolIdx(lit > 0.3)], Pick(wallRamp, math.Round(Clamp01(lit*1.15)*8)/8)
				}
			}
			if ch != 0 {
				v.Set(x, y, ch, col)
			}
		}
	})
}

// shade draws one cell: what the ray from the eye meets.
func (s *flicker) shade(cam view, dir Vec3, x, y int) (byte, uint8) {
	o := cam.o
	tt, part := viewD-2.6, pNone
	var p Vec3
	for i := 0; i < 120; i++ {
		p = o.Add(dir.Scale(tt))
		d, m := outer(p)
		if d < 0.0015 {
			part = m
			break
		}
		tt += math.Max(d, 0.002)
		if tt > viewD+2.5 {
			break
		}
	}
	if part == pNone {
		return 0, 0
	}
	n := outerNormal(p)
	if part == pGlass {
		return s.glass(cam, p, dir, n, x, y)
	}
	return s.solid(cam, p, dir, n, part)
}

// solid shades the base, ring, socket and cord.
func (s *flicker) solid(cam view, p, dir, n Vec3, part int) (byte, uint8) {
	g := s.glow
	refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
	key := math.Max(0, n.Dot(keyLight))
	spec := 0.0
	for _, l := range roomLights {
		spec = math.Max(spec, math.Pow(math.Max(0, refl.Dot(l)), 40))
	}
	// The filament's light from above, down through the neck: on the
	// base's top threads.
	below := g * Smoothstep(neckBot-0.35, neckBot, p[1]) * (0.5 + 0.5*math.Max(0, n[1]+0.3))
	switch part {
	case pBrass:
		// Thread crests catch the light, the grooves between them don't.
		th := math.Atan2(p[2], p[0])
		ph := math.Sin((p[1]-neckBot)*2*math.Pi/0.12 + th)
		i := 0.12 + 0.5*key + 0.35*below + 0.15*ph
		if spec > 0.5 {
			return '@', 230
		}
		i += spec * 0.5
		if ph > 0.2 {
			return "=#%"[min(2, int(Clamp01(i)*3))], Pick(brassRamp, i)
		}
		return "-=+"[min(2, int(Clamp01(i)*3))], Pick(brassRamp, i*0.75)
	case pBlack:
		i := 0.1 + 0.25*key + 0.6*spec + 0.25*below
		if spec > 0.6 {
			return '*', 250
		}
		return ":=#"[min(2, int(Clamp01(i)*3))], Pick(blackRamp, i)
	}
	// The contact: a dab of solder, silvery.
	i := 0.2 + 0.5*key + 0.5*spec
	return "o@"[BoolIdx(spec > 0.5)], Pick(wireRamp, i)
}

// glass shades a ray meeting the glass at p. The glass itself shows only
// where it should: its edge, an unbroken line along it; the room's lights,
// glinting; and, lit, a warm sheen. Through it: the cage, its glowing
// strands, the haze round them.
func (s *flicker) glass(cam view, p, dir, n Vec3, x, y int) (byte, uint8) {
	g := s.glow
	cos := -dir.Dot(n)
	refl := dir.Sub(n.Scale(2 * dir.Dot(n)))
	// The edge: where the glass turns away, a line following it.
	if cos < 0.3 {
		dx, dy := cam.screen(n)
		ch := LineChar(-dy, dx)
		i := 0.45 + 0.55*Smoothstep(0.3, 0.05, cos)
		if g > 0.05 {
			return ch, Pick(glassWarm, i*0.6+0.4*g)
		}
		return ch, Pick(glassCool, i)
	}
	// The room's lights, glinting on the glass, sliding as the view turns.
	best := 0.0
	for _, l := range roomLights {
		best = math.Max(best, refl.Dot(l))
	}
	switch {
	case best > 0.997:
		return '@', 231
	case best > 0.992:
		return '*', 253
	}
	// Inside: march the cage, and gather the strands' glow on the way.
	ch, col, hit := s.inside(cam, p, dir, x, y)
	if hit {
		return ch, col
	}
	if best > 0.96 && (x+y)%2 == 0 {
		return '.', 247 // a soft edge round the glints
	}
	return ch, col
}

// inside follows a ray through the glass's inside: to a strand, the stem or
// a wire, or out the far side. It returns what to draw, and whether it met
// something solid.
func (s *flicker) inside(cam view, p, dir Vec3, x, y int) (byte, uint8, bool) {
	g := s.glow
	// Only the cage's own space needs marching closely: a cylinder round it.
	t := 0.02
	haze := 0.0
	for i := 0; i < 90 && t < 2.6; i++ {
		q := p.Add(dir.Scale(t))
		if glassSDF(q) > 0 {
			break // out the far side
		}
		r := math.Hypot(q[0], q[2])
		if r > cageRt+0.15 || q[1] < stemFoot-0.1 || q[1] > cageTop+0.1 {
			// Far from the cage: step to its space, gathering a faint haze.
			step := math.Max(0.04, math.Min(r-cageRt-0.1, 0.25))
			haze += step * 0.08 / (1 + q.Sub(filCenter).Dot(q.Sub(filCenter)))
			t += step
			continue
		}
		c := s.cage(q)
		haze += math.Min(c.d, 0.05) * (math.Exp(-c.strand*14)*1.4 + 0.25)
		if c.d < 0.004 {
			dx, dy := cam.screen(c.along)
			line := LineChar(-dy, dx)
			switch c.part {
			case pStrand:
				if g < 0.03 {
					return line, Pick(wireRamp, 0.35), true // unlit: a dull gray wire
				}
				// Hottest midway, cooler where the hooks draw its heat.
				heat := (0.2 + 0.8*g) * (0.7 + 0.3*math.Sqrt(c.mid))
				if heat > 0.85 {
					return "#@"[BoolIdx(heat > 0.93)], Pick(heatRamp, heat), true
				}
				return line, Pick(heatRamp, heat), true
			case pStem:
				i := 0.3 + 0.5*g
				if g > 0.05 {
					return '|', Pick(glassWarm, i), true
				}
				return '|', Pick(glassCool, 0.5), true
			default:
				if g > 0.05 {
					return line, Pick(hazeRamp, 0.4+0.5*g), true
				}
				return line, Pick(wireRamp, 0.2), true
			}
		}
		t += math.Max(c.d*0.9, 0.004)
	}
	// Nothing solid: the haze of the strands' light in the glass, as dots,
	// thickest round them.
	hz := Clamp01(haze * g * 2.2)
	if hz > 0.04 && hz > dither(x, y)*0.8 {
		return ".:;+"[min(3, int(hz*4))], Pick(hazeRamp, 0.25+0.75*hz), false
	}
	return 0, 0, false
}
