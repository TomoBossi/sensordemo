package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

func init() {
	register(entry{
		name: "dice",
		desc: "shake the phone to roll 3D dice in a felt tray (optional: 3d20, 2d6+1d12; default 2d6)",
		uses: []string{"linear_acceleration", "gravity"},
		new:  func(specs []string) Demo { return &dice{} },
	})
}

// dice rolls real 3D dice: each die is a convex polyhedron (d4, d6, d8,
// d10, d12, d20), simulated as a rigid body whose corners bounce off the
// tray's floor, walls and glass lid and whose bodies bump each other, and
// ray cast per character with its numbers (pips on a d6) on its faces, a
// shadow on the felt and darkened edges that read as rounded.
//
// The tray lies in the screen. Shaking the phone throws the dice (the tray
// accelerates, the dice lag behind); tilting it from how you hold it lets
// them slide. When they come to rest, the result shows.
//
// World units are about a centimeter: x right, y up out of the screen, z
// toward the bottom of the screen.
type dice struct {
	kinds  []*dieKind
	dice   []die
	w, h   int     // view the tray was laid out for
	W, D   float64 // tray half width and half depth
	rng    *rand.Rand
	rest   [2]float64 // resting tilt (gravity on screen), which slowly follows the phone
	calm   float64    // seconds all dice have been still
	result []int
	rolled bool
	throw  bool
	since  float64 // time of the last result, for its entrance
	err    string
	light  Vec3    // toward the lamp
	fw, fd float64 // floor half extents the screen shows
	eyeY   float64
}

type die struct {
	k      *dieKind
	p, v   Vec3
	R      Mat3 // local to world
	w      Vec3 // angular velocity, world
	color  int
	sleep  float64
	asleep bool
}

// dieKind is a polyhedron: its faces as planes n.x <= d, its corners, and
// what each face shows.
type dieKind struct {
	sides  int
	n      []Vec3
	d      []float64
	verts  []Vec3
	label  []int     // number on each face (d4: on each corner)
	glyph  []float64 // numeral height on each face
	up     []Vec3    // numeral's up direction on each face
	radius float64   // circumradius
	body   float64   // radius used against other dice
	pips   bool
}

const (
	diceG      = 320.0 // gravity, units/s^2: a third of real, for dice that seem their size
	diceLid    = 7.0   // the glass lid's height
	diceWall   = 2.2   // the tray walls' height
	diceShake  = 40.0  // units/s^2 per m/s^2 of the phone's shake
	diceTilt   = 32.0  // units/s^2 per m/s^2 of tilt
	diceSteps  = 10    // physics substeps per frame
	diceSlow   = 0.65  // the dice's time runs at this pace: livelier to watch
	diceMaxDie = 12
)

var phi = (1 + math.Sqrt(5)) / 2

func (d *dice) Setup(ss *Streams) ([]*Gauge, error) {
	d.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	spec := demoArg
	if spec == "" {
		spec = "2d6"
	}
	kinds, err := parseDice(spec)
	if err != nil {
		return nil, err
	}
	d.kinds = kinds
	if _, err := ss.Subscribe("gravity", 30); err != nil {
		if _, err := ss.Subscribe("accelerometer", 30); err != nil {
			return nil, err
		}
	}
	d.light = diceLight
	lin, err := ss.Subscribe("linear_acceleration", 60)
	if err != nil {
		return nil, err
	}
	return gaugesFor(lin, 3), nil
}

// parseDice reads dice notation: NdF terms joined by +, F one of 4, 6, 8,
// 10, 12, 20.
func parseDice(spec string) ([]*dieKind, error) {
	var out []*dieKind
	for _, term := range strings.Split(strings.ToLower(spec), "+") {
		term = strings.TrimSpace(term)
		i := strings.IndexByte(term, 'd')
		if i < 0 {
			return nil, fmt.Errorf("dice: %q is not dice notation (try 2d6, 3d20, 1d8+1d12)", term)
		}
		n := 1
		if i > 0 {
			var err error
			if n, err = strconv.Atoi(term[:i]); err != nil || n < 1 {
				return nil, fmt.Errorf("dice: bad count in %q", term)
			}
		}
		f, err := strconv.Atoi(term[i+1:])
		if err != nil {
			return nil, fmt.Errorf("dice: bad sides in %q", term)
		}
		k := newDieKind(f)
		if k == nil {
			return nil, fmt.Errorf("dice: no d%d; there are d4, d6, d8, d10, d12, d20", f)
		}
		for j := 0; j < n; j++ {
			out = append(out, k)
		}
	}
	if len(out) > diceMaxDie {
		return nil, fmt.Errorf("dice: at most %d dice fit the tray", diceMaxDie)
	}
	return out, nil
}

// newDieKind builds a die's polyhedron from its corners and face normals,
// scaled to a circumradius of 1.2.
func newDieKind(sides int) *dieKind {
	var verts []Vec3
	cyc := func(a, b, c float64) []Vec3 { // (a,b,c) and its cyclic shifts
		return []Vec3{{a, b, c}, {c, a, b}, {b, c, a}}
	}
	signs := func(v Vec3) []Vec3 { // every sign combination of the nonzero parts
		out := []Vec3{v}
		for i := 0; i < 3; i++ {
			if v[i] == 0 {
				continue
			}
			var more []Vec3
			for _, u := range out {
				u[i] = -u[i]
				more = append(more, u)
			}
			out = append(out, more...)
		}
		return out
	}
	var icosa, dodeca []Vec3
	for _, c := range cyc(0, 1, phi) {
		icosa = append(icosa, signs(c)...)
	}
	dodeca = signs(Vec3{1, 1, 1})
	for _, c := range cyc(0, 1/phi, phi) {
		dodeca = append(dodeca, signs(c)...)
	}
	k := &dieKind{sides: sides}
	switch sides {
	case 4:
		verts = []Vec3{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}
	case 6:
		verts = signs(Vec3{1, 1, 1})
		k.pips = true
	case 8:
		verts = []Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	case 10:
		// A pentagonal trapezohedron: poles, and two rings of five offset by
		// 36 degrees. The kites are flat when H = d(1+cos36)/(1-cos36).
		const dz = 0.1
		c36 := math.Cos(math.Pi / 5)
		H := dz * (1 + c36) / (1 - c36)
		verts = []Vec3{{0, H, 0}, {0, -H, 0}}
		var up, lo []Vec3
		for i := 0; i < 5; i++ {
			a := float64(i) * 2 * math.Pi / 5
			up = append(up, Vec3{math.Cos(a), dz, math.Sin(a)})
			lo = append(lo, Vec3{math.Cos(a + math.Pi/5), -dz, math.Sin(a + math.Pi/5)})
		}
		verts = append(append(verts, up...), lo...)
	case 12:
		verts = dodeca
	case 20:
		verts = icosa
	default:
		return nil
	}
	// Scale to the circumradius, then find the faces from the corners:
	// every plane through three corners with all the others behind it.
	r := 0.0
	for _, v := range verts {
		r = math.Max(r, v.Len())
	}
	for i := range verts {
		verts[i] = verts[i].Scale(1.2 / r)
	}
	k.verts, k.radius = verts, 1.2
	for i := range verts {
		for j := i + 1; j < len(verts); j++ {
			for l := j + 1; l < len(verts); l++ {
				n := verts[j].Sub(verts[i]).Cross(verts[l].Sub(verts[i]))
				if n.Len() < 1e-9 {
					continue
				}
				n = n.Norm()
				d := n.Dot(verts[i])
				if d < 0 {
					n, d = n.Scale(-1), -d
				}
				outside := false
				for _, v := range verts {
					if v.Dot(n) > d+1e-6 {
						outside = true
						break
					}
				}
				dup := false
				for _, m := range k.n {
					if m.Dot(n) > 1-1e-6 {
						dup = true
					}
				}
				if !outside && !dup {
					k.n = append(k.n, n)
					k.d = append(k.d, d)
				}
			}
		}
	}
	inr := k.d[0]
	for _, x := range k.d {
		inr = math.Min(inr, x)
	}
	k.body = (inr + k.radius) / 2

	// Labels: opposite faces add up to sides+1 (d10: 0-9, adding to 9);
	// a d4's numbers are on its corners.
	nf := len(k.n)
	k.label = make([]int, nf)
	if sides == 4 {
		k.label = []int{1, 2, 3, 4}
	} else {
		used := make([]bool, nf)
		next := 1
		lo, hi := 1, sides
		if sides == 10 {
			lo, hi = 0, 9
		}
		_ = next
		for i := 0; i < nf; i++ {
			if used[i] {
				continue
			}
			opp := -1
			for j := 0; j < nf; j++ {
				if j != i && !used[j] && k.n[i].Dot(k.n[j]) < -0.999 {
					opp = j
				}
			}
			k.label[i], used[i] = lo, true
			if opp >= 0 {
				k.label[opp], used[opp] = hi, true
			}
			lo++
			hi--
		}
	}
	// Each face's numeral: upright toward a fixed direction in the die,
	// sized to the face.
	for i, n := range k.n {
		ref := Vec3{0, 1, 0}
		if math.Abs(n[1]) > 0.9 {
			ref = Vec3{0, 0, -1}
		}
		up := ref.Sub(n.Scale(ref.Dot(n))).Norm()
		k.up = append(k.up, up)
		// The distance from the face's center to its nearest edge.
		c := n.Scale(k.d[i])
		reach := math.Inf(1)
		for j := range k.n {
			if j == i {
				continue
			}
			// Along the face, the edge with plane j is where n_j.x = d_j.
			m := k.n[j].Sub(n.Scale(k.n[j].Dot(n)))
			if l := m.Len(); l > 1e-6 {
				reach = math.Min(reach, (k.d[j]-k.n[j].Dot(c))/l)
			}
		}
		k.glyph = append(k.glyph, reach)
	}
	return k
}

func (d *dice) Help() []string {
	return []string{
		"Shake the phone to throw the dice; tilt it to slide them around. When they stop, the total shows.",
		"Choose the dice with an argument: sensordemo dice 3d20, sensordemo dice 1d6+1d10+1d20 (d4, d6, d8, d10, d12, d20; up to 12 dice). A d10 shows 0-9 and counts 0 as 10; a d4 reads the number at its top corner.",
		"space  throw",
	}
}

func (d *dice) Key(k byte) {
	if k == ' ' {
		d.throw = true
	}
}

// layout sizes the tray to the view: its shape follows the screen, its
// size the number of dice.
func (d *dice) layout(w, h int) {
	d.w, d.h = w, h
	area := 4.2 * (float64(len(d.kinds)) + 1.5) // two dice: each about a third of the width
	aspect := float64(h) * 2 / float64(w)       // depth per width
	d.W = math.Sqrt(area / aspect)
	d.D = d.W * aspect
	if d.dice == nil {
		for i, k := range d.kinds {
			d.dice = append(d.dice, die{k: k, R: Identity(), color: i % len(diceColors)})
		}
		d.throwAll()
	}
	// What the screen shows: the rim's top in a band of the same visual
	// thickness on every side (a row is two columns tall), at least two
	// rows. Seen from above, the rim is magnified by k.
	tc := math.Max(4, math.Round(0.06*float64(w))) // rim, in columns
	mx, mz := tc/(float64(w)/2), (tc/2)/(float64(h)/2)
	d.eyeY = 2.3 * math.Max(d.W, d.D) * 1.2
	for it := 0; it < 3; it++ {
		k := d.eyeY / (d.eyeY - diceWall)
		d.fw, d.fd = d.W*k/(1-mx), d.D*k/(1-mz)
		d.eyeY = 2.3 * math.Max(d.fw, d.fd)
	}
	for i := range d.dice { // keep them inside a resized tray
		p := &d.dice[i].p
		p[0] = math.Max(-d.W+1.3, math.Min(d.W-1.3, p[0]))
		p[2] = math.Max(-d.D+1.3, math.Min(d.D-1.3, p[2]))
	}
}

// throwAll tosses every die from above with a random spin.
func (d *dice) throwAll() {
	for i := range d.dice {
		dd := &d.dice[i]
		dd.p = Vec3{(d.rng.Float64()*2 - 1) * (d.W - 1.5), 3 + 2.5*d.rng.Float64(), (d.rng.Float64()*2 - 1) * (d.D - 1.5)}
		dd.v = Vec3{(d.rng.Float64()*2 - 1) * 20, 4 + 8*d.rng.Float64(), (d.rng.Float64()*2 - 1) * 20}
		dd.w = Vec3{(d.rng.Float64()*2 - 1) * 22, (d.rng.Float64()*2 - 1) * 22, (d.rng.Float64()*2 - 1) * 22}
		dd.R = rotAxis(Vec3{d.rng.NormFloat64(), d.rng.NormFloat64(), d.rng.NormFloat64()}.Norm(), d.rng.Float64()*6.3)
		dd.asleep, dd.sleep = false, 0
	}
	d.result, d.calm, d.rolled = nil, 0, true
}

func (d *dice) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 10 {
		return
	}
	area := v.H - 1
	if v.W != d.w || area != d.h {
		d.layout(v.W, area)
	}
	dt = math.Min(dt, 0.05)

	// Forces from the phone: the shake throws, the tilt slides.
	var acc Vec3
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			la := toScreen(ss, r.V)
			// Screen (x right, y up, z out) to tray (x right, y out, z down).
			acc = Vec3{la[0], la[2], -la[1]}.Scale(-diceShake)
		}
	}
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 3 {
		a := toScreen(ss, r.V)
		k := math.Min(1, dt/3) // the resting tilt follows the phone over seconds
		d.rest[0] += (a[0] - d.rest[0]) * k
		d.rest[1] += (a[1] - d.rest[1]) * k
		acc = acc.Add(Vec3{-(a[0] - d.rest[0]), 0, a[1] - d.rest[1]}.Scale(diceTilt))
	}
	if d.throw {
		d.throw = false
		d.throwAll()
		d.since = t
	}
	if acc.Len() > 400 { // a good shake wakes everything
		for i := range d.dice {
			d.dice[i].asleep = false
		}
		d.result = nil
	}
	h := dt * diceSlow / diceSteps
	for s := 0; s < diceSteps; s++ {
		d.physics(acc, h)
	}

	// Settled?
	still := true
	for _, dd := range d.dice {
		if !dd.asleep && (dd.v.Len() > 1.5 || dd.w.Len() > 1.5) {
			still = false
		}
	}
	if still {
		d.calm += dt
	} else {
		d.calm, d.result = 0, nil
	}
	if d.calm > 0.3 && d.result == nil && d.rolled {
		d.result = nil
		for _, dd := range d.dice {
			d.result = append(d.result, dd.value())
		}
		d.since = t
	}

	d.render(v, area)
	d.caption(v, area, t)
}

// flat reports whether the die lies on one of its faces.
func (dd *die) flat() bool {
	for _, n := range dd.k.n {
		if dd.R.Apply(n)[1] < -0.985 {
			return true
		}
	}
	return false
}

// value reads the die: the face pointing up (a d4: the corner).
func (dd *die) value() int {
	k := dd.k
	best, bi := math.Inf(-1), 0
	if k.sides == 4 {
		for i, c := range k.verts {
			if y := dd.R.Apply(c)[1]; y > best {
				best, bi = y, i
			}
		}
		return k.label[bi]
	}
	for i, n := range k.n {
		if y := dd.R.Apply(n)[1]; y > best {
			best, bi = y, i
		}
	}
	if k.sides == 10 && k.label[bi] == 0 {
		return 10
	}
	return k.label[bi]
}

// physics advances every die by h: gravity and the phone's pushes, then
// contacts with the tray and each other.
func (d *dice) physics(acc Vec3, h float64) {
	g := Vec3{0, -diceG, 0}.Add(acc)
	for i := range d.dice {
		dd := &d.dice[i]
		if dd.asleep {
			continue
		}
		dd.v = dd.v.Add(g.Scale(h))
		dd.p = dd.p.Add(dd.v.Scale(h))
		if wl := dd.w.Len(); wl > 1e-9 {
			dd.R = rotAxis(dd.w.Scale(1/wl), wl*h).Mul(dd.R)
		}
		dd.v = dd.v.Scale(1 - 0.05*h)
		dd.w = dd.w.Scale(1 - 0.1*h)
	}
	// The tray: floor, lid and walls, as planes n.x >= off.
	planes := []struct {
		n   Vec3
		off float64
		e   float64
	}{
		{Vec3{0, 1, 0}, 0, 0.4},
		{Vec3{0, -1, 0}, -diceLid, 0.3},
		{Vec3{1, 0, 0}, -d.W, 0.5},
		{Vec3{-1, 0, 0}, -d.W, 0.5},
		{Vec3{0, 0, 1}, -d.D, 0.5},
		{Vec3{0, 0, -1}, -d.D, 0.5},
	}
	for i := range d.dice {
		dd := &d.dice[i]
		if dd.asleep {
			continue
		}
		// Contacts: corners below a plane. Their impulses are applied in a
		// few passes so that several at once (a face lying on the felt)
		// settle together instead of knocking each other about.
		type contact struct {
			r, n Vec3
			e    float64
		}
		var cs []contact
		var push [6]float64
		for pi, pl := range planes {
			if dd.p.Dot(pl.n)-pl.off > dd.k.radius { // nowhere near
				continue
			}
			for _, c := range dd.k.verts {
				r := dd.R.Apply(c)
				if depth := pl.off - dd.p.Add(r).Dot(pl.n); depth > 0 {
					cs = append(cs, contact{r, pl.n, pl.e})
					push[pi] = math.Max(push[pi], depth)
				}
			}
		}
		touching := len(cs) > 0
		for pass := 0; pass < 4; pass++ {
			for _, c := range cs {
				dd.impulse(c.r, c.n, c.e, 0.25)
			}
		}
		for pi, pl := range planes {
			dd.p = dd.p.Add(pl.n.Scale(push[pi] * 0.8))
		}
		if touching { // rolling resistance and felt drag
			dd.w = dd.w.Scale(math.Max(0, 1-1.0*h))
			dd.v = dd.v.Scale(math.Max(0, 1-0.3*h))
			// Lying flat on a face and slow, felt holds it (static
			// friction): it stops instead of creeping.
			if dd.v.Len() < 2 && dd.w.Len() < 2.5 && dd.flat() {
				k := math.Exp(-10 * h)
				dd.v, dd.w = dd.v.Scale(k), dd.w.Scale(k)
			}
		}
		// Rest: slow, touching and lying on a face for a moment, it sleeps
		// until disturbed. (Not on an edge: it would tip over.)
		if touching && dd.v.Len() < 1.2 && dd.w.Len() < 1.5 && dd.flat() {
			dd.sleep += h
			if dd.sleep > 0.25 {
				dd.asleep, dd.v, dd.w = true, Vec3{}, Vec3{}
			}
		} else {
			dd.sleep = 0
		}
	}
	// Dice against dice, as spheres.
	for i := range d.dice {
		for j := i + 1; j < len(d.dice); j++ {
			a, b := &d.dice[i], &d.dice[j]
			delta := b.p.Sub(a.p)
			dist := delta.Len()
			min := a.k.body + b.k.body
			if dist >= min || dist < 1e-9 {
				continue
			}
			n := delta.Scale(1 / dist)
			push := (min - dist) / 2
			if a.asleep && b.asleep {
				continue
			}
			a.asleep, b.asleep = false, false
			a.p, b.p = a.p.Sub(n.Scale(push)), b.p.Add(n.Scale(push))
			if vn := b.v.Sub(a.v).Dot(n); vn < 0 {
				j := -(1 + 0.4) * vn / 2
				a.v, b.v = a.v.Sub(n.Scale(j)), b.v.Add(n.Scale(j))
				// A glancing knock sets them spinning.
				tang := b.v.Sub(a.v).Sub(n.Scale(b.v.Sub(a.v).Dot(n)))
				spin := n.Cross(tang).Scale(0.15)
				a.w, b.w = a.w.Add(spin), b.w.Sub(spin)
			}
		}
	}
}

// impulse bounces a die's corner at r (from its center) off a surface with
// normal n: restitution e, friction mu. The die is treated as a solid ball
// for its inertia.
func (dd *die) impulse(r, n Vec3, e, mu float64) {
	const m = 1.0
	invI := 1 / (0.4 * m * dd.k.body * dd.k.body)
	vc := dd.v.Add(dd.w.Cross(r))
	vn := vc.Dot(n)
	if vn >= 0 {
		return
	}
	if vn > -3 {
		e = 0 // a gentle touch doesn't bounce: resting contacts stay put
	}
	rn := r.Cross(n)
	jn := -(1 + e) * vn / (1/m + invI*rn.Dot(rn))
	dd.v = dd.v.Add(n.Scale(jn / m))
	dd.w = dd.w.Add(rn.Scale(jn * invI))
	// Friction, up to mu times the normal impulse.
	vc = dd.v.Add(dd.w.Cross(r))
	vt := vc.Sub(n.Scale(vc.Dot(n)))
	if l := vt.Len(); l > 1e-9 {
		t := vt.Scale(1 / l)
		rt := r.Cross(t)
		jt := math.Min(l/(1/m+invI*rt.Dot(rt)), mu*jn)
		dd.v = dd.v.Sub(t.Scale(jt / m))
		dd.w = dd.w.Sub(rt.Scale(jt * invI))
	}
}
