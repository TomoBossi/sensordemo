package main

import (
	"fmt"
	"math"
	"strings"
)

// Each die's colors: its body, dark to bright, and its numbers.
var diceColors = []struct {
	body []uint8
	ink  uint8
}{
	{[]uint8{243, 246, 249, 251, 253, 255, 231}, 233}, // ivory
	{[]uint8{52, 88, 124, 160, 196, 203, 210}, 231},   // red
	{[]uint8{17, 18, 19, 20, 26, 33, 75}, 231},        // blue
	{[]uint8{22, 28, 34, 35, 41, 77, 114}, 231},       // green
	{[]uint8{53, 54, 55, 91, 92, 98, 141}, 231},       // purple
	{[]uint8{94, 130, 136, 172, 178, 214, 221}, 233},  // amber
	{[]uint8{23, 24, 30, 31, 37, 44, 80}, 231},        // teal
	{[]uint8{233, 235, 237, 239, 242, 245, 248}, 220}, // black, gold numbers
}

var (
	diceFelt  = []uint8{232, 233, 22, 22, 28, 29, 34, 35, 71} // shadows reach near black
	diceLight = Vec3{-0.5, 1, -0.55}.Norm()                   // above, from the top left corner
)

// digitFont is a 3x5 font for the numbers on the faces.
var digitFont = [10][5]string{
	{"###", "#.#", "#.#", "#.#", "###"},
	{".#.", "##.", ".#.", ".#.", "###"},
	{"###", "..#", "###", "#..", "###"},
	{"###", "..#", "###", "..#", "###"},
	{"#.#", "#.#", "###", "..#", "..#"},
	{"###", "#..", "###", "..#", "###"},
	{"###", "#..", "###", "#.#", "###"},
	{"###", "..#", "..#", "..#", "..#"},
	{"###", "#.#", "###", "#.#", "###"},
	{"###", "#.#", "###", "..#", "###"},
}

// inNumeral reports whether (u, w), in a box of height gh centered on the
// origin, falls on a stroke of the number n (6 and 9 get a bar beneath).
func inNumeral(n int, u, w, gh float64) bool {
	s := fmt.Sprint(n)
	cell := gh / 5
	width := float64(len(s)*4-1) * cell
	x := (u + width/2) / cell
	y := (gh/2 - w) / cell
	if n == 6 || n == 9 {
		y += 0.6 // room for the bar
		if y >= 5.2 && y < 5.9 && x >= 0 && x < 3 {
			return true
		}
	}
	if x < 0 || y < 0 || y >= 5 || x >= float64(len(s)*4-1) {
		return false
	}
	ci, col := int(x)/4, int(x)%4
	if col == 3 {
		return false
	}
	return digitFont[s[ci]-'0'][int(y)][col] == '#'
}

var pipLayout = map[int][][2]float64{
	1: {{0, 0}},
	2: {{-1, -1}, {1, 1}},
	3: {{-1, -1}, {0, 0}, {1, 1}},
	4: {{-1, -1}, {1, -1}, {-1, 1}, {1, 1}},
	5: {{-1, -1}, {1, -1}, {0, 0}, {-1, 1}, {1, 1}},
	6: {{-1, -1}, {-1, 0}, {-1, 1}, {1, -1}, {1, 0}, {1, 1}},
}

// hit intersects a ray (local to the die) with its polyhedron: the entry
// distance and face.
func (k *dieKind) hit(o, dir Vec3) (float64, int, bool) {
	tin, tout, face := math.Inf(-1), math.Inf(1), -1
	for i, n := range k.n {
		den := n.Dot(dir)
		num := k.d[i] - n.Dot(o)
		switch {
		case den < -1e-12:
			if t := num / den; t > tin {
				tin, face = t, i
			}
		case den > 1e-12:
			tout = math.Min(tout, num/den)
		case num < 0:
			return 0, 0, false
		}
	}
	return tin, face, face >= 0 && tin <= tout && tin > 0
}

// cast finds the nearest die along a world ray.
func (d *dice) cast(o, dir Vec3) (float64, int, int) {
	best, bi, bf := math.Inf(1), -1, -1
	for i := range d.dice {
		dd := &d.dice[i]
		rel := dd.p.Sub(o)
		along := rel.Dot(dir)
		if rel.Dot(rel)-along*along > dd.k.radius*dd.k.radius || along < -dd.k.radius {
			continue
		}
		Rt := dd.R.T()
		if t, f, ok := dd.k.hit(Rt.Apply(rel.Scale(-1)), Rt.Apply(dir)); ok && t < best {
			best, bi, bf = t, i, f
		}
	}
	return best, bi, bf
}

func (d *dice) render(v *View, area int) {
	fw, fd := d.fw, d.fd
	eye := Vec3{0, d.eyeY, 0}
	parallelRows(area, func(y int) {
		for x := 0; x < v.W; x++ {
			u := (float64(x)+0.5)/float64(v.W)*2 - 1
			w := (float64(y)+0.5)/float64(area)*2 - 1
			target := Vec3{u * fw, 0, w * fd}
			dir := target.Sub(eye).Norm()
			c, col := d.shadePixel(eye, dir, x, y)
			v.Set(x, y, c, col)
		}
	})
}

func (d *dice) shadePixel(eye, dir Vec3, px, py int) (byte, uint8) {
	L := d.light
	if t, i, f := d.cast(eye, dir); i >= 0 {
		return d.shadeDie(&d.dice[i], f, eye.Add(dir.Scale(t)), dir)
	}
	// The tray: floor inside the walls, else the walls' inner faces or the rim.
	tf := -eye[1] / dir[1]
	fp := eye.Add(dir.Scale(tf))
	if math.Abs(fp[0]) <= d.W && math.Abs(fp[2]) <= d.D {
		shade := 0.35 + 0.55*L[1]
		shadow := false
		if _, i, _ := d.cast(fp.Add(Vec3{0, 0.01, 0}), L); i >= 0 {
			shadow = true // a die's
		}
		// The walls between the felt and the lamp shade it: how far a
		// wall's shadow reaches is its height times the light's slant.
		reachX, reachZ := diceWall*math.Abs(L[0])/L[1], diceWall*math.Abs(L[2])/L[1]
		if L[0] < 0 && fp[0]+d.W < reachX || L[0] > 0 && d.W-fp[0] < reachX ||
			L[2] < 0 && fp[2]+d.D < reachZ || L[2] > 0 && d.D-fp[2] < reachZ {
			shadow = true
		}
		n := noise3(fp[0]*1.7, fp[2]*1.7, 0)
		ch := byte(':')
		if n < 0 {
			ch = '.'
		}
		if shadow {
			shade *= 0.3
			ch = '.'
		}
		return ch, mzPick(diceFelt, shade+0.06*n)
	}
	// Which wall face or rim top the ray meets first.
	best, bn := math.Inf(1), Vec3{0, 1, 0}
	try := func(t float64, n Vec3, ok func(p Vec3) bool) {
		if t > 0 && t < best {
			if p := eye.Add(dir.Scale(t)); ok(p) {
				best, bn = t, n
			}
		}
	}
	for _, sx := range []float64{-1, 1} {
		try((sx*d.W-eye[0])/dir[0], Vec3{-sx, 0, 0}, func(p Vec3) bool {
			return p[1] >= 0 && p[1] <= diceWall && math.Abs(p[2]) <= d.D
		})
		try((sx*d.D-eye[2])/dir[2], Vec3{0, 0, -sx}, func(p Vec3) bool {
			return p[1] >= 0 && p[1] <= diceWall && math.Abs(p[0]) <= d.W
		})
	}
	try((diceWall-eye[1])/dir[1], Vec3{0, 1, 0}, func(p Vec3) bool {
		return math.Abs(p[0]) > d.W || math.Abs(p[2]) > d.D
	})
	// A fill from the lower right tells the two walls away from the lamp
	// apart.
	fill := Vec3{0.8, 0.35, 0.25}.Norm()
	i := 0.1 + 0.75*math.Max(0, bn.Dot(L)) + 0.25*math.Max(0, bn.Dot(fill))
	if bn[1] > 0.5 {
		i += 0.1 // the rim's top catches the light
	}
	const solid = "=+#%@"
	return solid[int(clamp01(i)*4+0.5)], mzPick(artWoodRamp, i)
}

// shadeDie colors a die's face at world point p: light, darkened edges
// (which read as rounded), and its number or pips.
func (d *dice) shadeDie(dd *die, f int, p, dir Vec3) (byte, uint8) {
	k := dd.k
	local := dd.R.T().Apply(p.Sub(dd.p))
	n := dd.R.Apply(k.n[f])
	L := d.light
	eyeDir := dir.Scale(-1)
	diff := math.Max(0, n.Dot(L))
	spec := math.Pow(math.Max(0, n.Dot(L.Add(eyeDir).Norm())), 30)
	// How far to the nearest edge: small there, like a rounded corner.
	edge := math.Inf(1)
	for j := range k.n {
		if j != f {
			edge = math.Min(edge, k.d[j]-k.n[j].Dot(local))
		}
	}
	i := (0.18 + 0.72*diff + 0.4*spec) * (0.55 + 0.45*smoothstep(0.02, 0.14, edge))
	pal := diceColors[dd.color]

	// The face's own frame, to draw on it.
	fn := k.n[f]
	rel := local.Sub(fn.Scale(k.d[f]))
	up := k.up[f]
	right := up.Cross(fn)
	u, w := rel.Dot(right), rel.Dot(up)
	reach := k.glyph[f]
	ink := false
	switch {
	case k.pips:
		for _, pp := range pipLayout[k.label[f]] {
			if math.Hypot(u-pp[0]*0.56*reach, w-pp[1]*0.56*reach) < 0.25*reach {
				ink = true
			}
		}
	case k.sides == 4:
		// A d4's numbers sit at its corners, each upright toward its corner.
		for vi, c := range k.verts {
			if math.Abs(c.Dot(fn)-k.d[f]) > 1e-6 {
				continue
			}
			cr := c.Sub(fn.Scale(k.d[f]))
			vu := cr.Norm()
			vr := vu.Cross(fn)
			pos := cr.Scale(0.5)
			q := rel.Sub(pos)
			if inNumeral(k.label[vi], q.Dot(vr), q.Dot(vu), 0.5*reach) {
				ink = true
			}
		}
	default:
		ink = inNumeral(k.label[f], u, w, 0.95*reach)
	}
	if ink {
		return '#', pal.ink
	}
	const solid = "=+*#%@"
	return solid[int(clamp01(i)*5+0.5)], mzPick(pal.body, i)
}

// caption shows the dice and, once they rest, the result.
func (d *dice) caption(v *View, area int, t float64) {
	var names []string
	count := map[int]int{}
	var order []int
	for _, k := range d.kinds {
		if count[k.sides] == 0 {
			order = append(order, k.sides)
		}
		count[k.sides]++
	}
	for _, s := range order {
		names = append(names, fmt.Sprintf("%dd%d", count[s], s))
	}
	spec := strings.Join(names, "+")
	line := spec + ": shake to roll"
	col := uint8(244)
	if d.result != nil {
		sum := 0
		var parts []string
		for _, r := range d.result {
			sum += r
			parts = append(parts, fmt.Sprint(r))
		}
		line = fmt.Sprintf("%s: %d", spec, sum)
		if len(d.result) > 1 {
			line = fmt.Sprintf("%s: %s = %d", spec, strings.Join(parts, " + "), sum)
		}
		col = 220
	} else if d.calm == 0 {
		line = spec + ": rolling..."
	}
	v.Text(max(0, (v.W-len(line))/2), area, line, col)
}
