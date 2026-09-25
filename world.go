package main

import (
	"math"
	"math/rand"
)

// The world of the space demo: an endless floor with one object per grid
// cell (chosen and sized by a hash of the cell, so it is the same every run),
// lit by a sun that is also drawn in the sky, with soft shadows, a ringed
// planet, a moon and a fixed star field.

const (
	matFloor = iota
	matPillar
	matOrb
	matTrunk
	matCrown
	matBlock
	matRing
	matCrystal
	matMonolith
)

var matColors = [][]uint8{
	matFloor:    {236, 237, 238, 240, 242, 244, 246, 248, 250, 252},
	matPillar:   {17, 18, 24, 25, 31, 32, 38, 39, 45, 117},
	matOrb:      {52, 88, 124, 160, 196, 202, 208, 214, 220, 229},
	matTrunk:    {52, 52, 94, 94, 130, 130, 136, 137, 173, 180},
	matCrown:    {22, 22, 28, 28, 34, 70, 76, 112, 148, 150},
	matBlock:    {52, 88, 130, 166, 172, 208, 209, 215, 216, 223},
	matRing:     {53, 89, 90, 126, 127, 163, 164, 170, 206, 213},
	matCrystal:  {23, 30, 37, 44, 51, 87, 123, 159, 195, 231},
	matMonolith: {232, 232, 233, 233, 234, 235, 236, 237, 238, 239},
}

// Sky objects, as directions (x east, y north, z up).
var (
	sunDir    = skyDir(40, 30)
	planetDir = skyDir(205, 30)
	moonDir   = skyDir(300, 48)
	ringAxis  = Vec3{0.35, -0.2, 1}.Norm() // planet's ring plane normal
	stars     = makeStars(1400)
)

const (
	sunRadius    = 3.2 * math.Pi / 180
	planetRadius = 4.5 * math.Pi / 180
	moonRadius   = 2.6 * math.Pi / 180
)

func skyDir(azDeg, elDeg float64) Vec3 {
	az, el := azDeg*math.Pi/180, elDeg*math.Pi/180
	return Vec3{math.Cos(el) * math.Sin(az), math.Cos(el) * math.Cos(az), math.Sin(el)}
}

type star struct {
	dir Vec3
	c   byte
	fg  uint8
}

// makeStars scatters a fixed catalog over the sky, mostly faint.
func makeStars(n int) []star {
	rng := rand.New(rand.NewSource(42))
	out := make([]star, 0, n)
	for len(out) < n {
		d := Vec3{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}.Norm()
		if d[2] < 0.02 {
			continue
		}
		m := rng.Float64()
		s := star{dir: d, c: '.', fg: 244}
		switch {
		case m > 0.985:
			s.c, s.fg = '*', 231
		case m > 0.93:
			s.c, s.fg = '+', 253
		case m > 0.75:
			s.c, s.fg = '.', 250
		}
		if rng.Intn(9) == 0 { // a few colored ones
			s.fg = []uint8{153, 217, 229, 159}[rng.Intn(4)]
		}
		out = append(out, s)
	}
	return out
}

// cellHash mixes a cell's coordinates into 32 random-looking bits.
func cellHash(ix, iy int) uint32 {
	h := uint32(ix)*0x8da6b343 ^ uint32(iy)*0xd8163841
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return h
}

func frac(h uint32, shift uint) float64 { return float64((h>>shift)&0xff) / 255 }

func sdBox(q, b Vec3) float64 {
	dx, dy, dz := math.Abs(q[0])-b[0], math.Abs(q[1])-b[1], math.Abs(q[2])-b[2]
	out := math.Hypot(math.Hypot(math.Max(dx, 0), math.Max(dy, 0)), math.Max(dz, 0))
	return out + math.Min(math.Max(dx, math.Max(dy, dz)), 0)
}

func sdSphere(q Vec3, z, r float64) float64 {
	return math.Sqrt(q[0]*q[0]+q[1]*q[1]+(q[2]-z)*(q[2]-z)) - r
}

// cellObject is the distance from q (relative to the cell center, on the
// floor) to the cell's object, and its material. Objects stay within 1.6 m
// of the center, so the cell boundary bounds neighbors (see scene).
func cellObject(q Vec3, h uint32) (float64, int) {
	a, b, c := frac(h, 8), frac(h, 16), frac(h, 24)
	switch k := h % 100; {
	case k < 18: // empty
		return math.Inf(1), matFloor
	case k < 40: // pillar, sometimes with an orb on top
		r, ht := 0.3+0.3*a, 2+3*b
		d := math.Max(math.Hypot(q[0], q[1])-r, q[2]-ht)
		if c > 0.45 {
			if o := sdSphere(q, ht+r*1.5, r*1.5); o < d {
				return o, matOrb
			}
		}
		return d, matPillar
	case k < 62: // tree: trunk and one or two round crowns
		trunk := 1.1 + 0.7*a
		cr := 0.8 + 0.45*b
		d := math.Max(math.Hypot(q[0], q[1])-0.14, q[2]-trunk-cr*0.5)
		crown := sdSphere(q, trunk+cr*0.6, cr)
		if c > 0.5 {
			q2 := Vec3{q[0] - cr*0.45, q[1] + cr*0.2, q[2]}
			crown = math.Min(crown, sdSphere(q2, trunk+cr*1.35, cr*0.6))
		}
		if crown < d {
			return crown, matCrown
		}
		return d, matTrunk
	case k < 78: // block
		b3 := Vec3{0.35 + 0.6*a, 0.35 + 0.6*b, 0.3 + 0.9*c}
		return sdBox(Vec3{q[0], q[1], q[2] - b3[2]}, b3), matBlock
	case k < 88: // standing ring, like a portal
		R := 0.9 + 0.45*a
		u, w := q[0], q[1] // ring plane contains z and one horizontal axis
		if b > 0.5 {
			u, w = q[1], q[0]
		}
		t := math.Hypot(u, q[2]-R-0.2) - R
		return math.Hypot(t, w) - 0.13, matRing
	case k < 98: // floating crystal (octahedron)
		s, z := 0.45+0.35*a, 1.5+0.9*b
		return (math.Abs(q[0]) + math.Abs(q[1]) + math.Abs(q[2]-z) - s) * 0.577, matCrystal
	default: // the rare monolith
		return sdBox(Vec3{q[0], q[1], q[2] - 2.2}, Vec3{0.12, 0.5, 2.2}), matMonolith
	}
}

// scene is the distance to the nearest surface and its material.
func scene(p Vec3) (float64, int) {
	d, m := p[2], matFloor
	ix, iy := math.Round(p[0]/pillarGap), math.Round(p[1]/pillarGap)
	q := Vec3{p[0] - ix*pillarGap, p[1] - iy*pillarGap, p[2]}
	if od, om := cellObject(q, cellHash(int(ix), int(iy))); od < d {
		d, m = od, om
	}
	// Neighboring objects are at least (gap/2 - 1.6) past this cell's
	// boundary, so never step further than that past the boundary.
	border := pillarGap/2 - math.Max(math.Abs(q[0]), math.Abs(q[1]))
	return math.Min(d, border+pillarGap/2-1.6), m
}

func sceneNormal(p Vec3) Vec3 {
	const e = 0.01
	d := func(q Vec3) float64 { v, _ := scene(q); return v }
	return Vec3{
		d(p.Add(Vec3{e, 0, 0})) - d(p.Sub(Vec3{e, 0, 0})),
		d(p.Add(Vec3{0, e, 0})) - d(p.Sub(Vec3{0, e, 0})),
		d(p.Add(Vec3{0, 0, e})) - d(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

// inShadow marches from p toward the sun, treated as a point: p is shadowed
// if the ray hits anything. A yes/no answer keeps shadow edges crisp and
// doesn't depend on distance estimates, which scene deliberately shortens
// near cell borders.
func inShadow(p Vec3) bool {
	t := 0.05
	for i := 0; i < 64 && t < 20; i++ {
		q := p.Add(sunDir.Scale(t))
		if q[2] > 6.5 { // above everything in the world
			return false
		}
		d, _ := scene(q)
		if d < 0.003 {
			return true
		}
		t += math.Max(d, 0.02)
	}
	return false
}

// shadeSky returns what a ray that hits nothing sees, besides stars: the
// sun, the planet and its rings, the moon. ok is false for empty sky.
func shadeSky(dir Vec3) (byte, uint8, bool) {
	// Sun: disc and a halo of short rays.
	if a := math.Acos(math.Min(1, dir.Dot(sunDir))); a < sunRadius*2.3 {
		switch {
		case a < sunRadius:
			return '@', 226, true
		case a < sunRadius*1.5:
			return '*', 220, true
		default:
			e1 := sunDir.Cross(Vec3{0, 0, 1}).Norm()
			ang := math.Atan2(dir.Dot(sunDir.Cross(e1)), dir.Dot(e1))
			if math.Mod(ang*6/math.Pi+12, 1) < 0.3 {
				return '+', 214, true
			}
		}
	}
	// Planet and rings: the near half of the rings covers the planet, the
	// planet covers the far half.
	rc, rfg, rt, ring := ringAt(dir)
	if ring && rt < planetDist {
		return rc, rfg, true
	}
	if c, fg, ok := shadeBody(dir, planetDir, planetRadius, matColors[matBlock]); ok {
		return c, fg, true
	}
	if ring {
		return rc, rfg, true
	}
	return shadeBody(dir, moonDir, moonRadius, matColors[matFloor])
}

// shadeBody draws a sphere in the sky at direction center with angular
// radius r, lit by the sun (so it shows a phase). The unlit side is left
// dark, which makes the moon a crescent.
func shadeBody(dir, center Vec3, r float64, cols []uint8) (byte, uint8, bool) {
	cosA := dir.Dot(center)
	if cosA < math.Cos(r) {
		return 0, 0, false
	}
	e1 := center.Cross(Vec3{0, 0, 1}).Norm()
	e2 := e1.Cross(center)
	s := math.Sin(r)
	x, y := dir.Dot(e1)/s, dir.Dot(e2)/s
	z := math.Sqrt(math.Max(0, 1-x*x-y*y))
	n := e1.Scale(x).Add(e2.Scale(y)).Sub(center.Scale(z)) // faces the viewer
	lum := n.Dot(sunDir)
	if lum <= 0.02 {
		return ' ', 0, true // night side: blocks the stars behind
	}
	k := min(int(lum*float64(len(shade))), len(shade)-1)
	return shade[k], cols[min(k, len(cols)-1)], true
}

const planetDist = 100.0 // how far the planet is placed, for the ring geometry

// ringAt finds where the ray crosses the planet's ring plane between 1.4 and
// 2.3 planet radii from its center; t is the distance along the ray.
func ringAt(dir Vec3) (c byte, fg uint8, t float64, ok bool) {
	center := planetDir.Scale(planetDist)
	den := dir.Dot(ringAxis)
	if math.Abs(den) < 1e-6 {
		return
	}
	t = center.Dot(ringAxis) / den
	if t <= 0 {
		return
	}
	rad := dir.Scale(t).Sub(center).Len() / (planetDist * math.Sin(planetRadius))
	switch {
	case rad < 1.4 || rad > 2.3:
		return
	case rad < 1.7:
		return '=', 180, t, true
	case rad < 1.85:
		return // the gap between the rings
	default:
		return '-', 144, t, true
	}
}
