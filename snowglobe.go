package main

import (
	"math"
	"math/rand"
	"strconv"
	"time"
)

func init() {
	register(entry{
		name: "snowglobe",
		desc: "a snow globe: shake the phone and the snow swirls through the glass, then settles on a little village",
		uses: []string{"linear_acceleration"},
		new:  func(specs []string) Demo { return &snowglobe{} },
	})
}

// snowglobe is a glass globe on a turned wooden base with a small village
// inside: a cabin with lit windows, pines, a lamppost and a snowman, ray
// marched once per screen size and cached.
//
// The snow is a few thousand flakes in 3D. Shaking the phone stirs the
// liquid: every shake spawns swirls (point vortices, mirrored in the glass
// so the flow never leaves it), twisting the phone spins the liquid, and
// the flakes, a little heavier than the liquid, also lag the glass. They
// sink slowly and settle into a height map over the scene, so snow builds
// up on the ground, the roof, the branches, and slides where it gets too
// steep or when the globe is tilted. A strong current or a hard shake picks
// settled snow back up.
//
// Globe coordinates: radius 1, x right, y up, z toward the viewer.
type snowglobe struct {
	w, h   int // view the cache was made for
	S      float64
	cx, cy float64 // screen position of the globe center
	px     []pixel
	plaque [2]int // where the plaque's text goes, -1 if hidden

	// Height map of the scene tops and the settled snow on them.
	top   []float64 // scene height per cell, NaN where the floor is outside the glass
	snow  []float64 // settled snow depth per cell
	onGnd []bool    // the top is the ground, not an object
	flk   []flake3
	vort  []vortex
	spin  float64 // solid rotation of the liquid, rad/s
	down  [2]float64
	gyroZ float64
	kick  float64 // time left of a keyboard shake
	shake Vec3
	agit  float64 // small-scale turbulence: follows the shake, fades after
	rng   *rand.Rand
	wbuf  []float64 // flake weight per pixel, per frame
	zbuf  []float64 // nearest flake per pixel
}

type flake3 struct {
	p, v Vec3
	size float64 // 0.6 .. 1.4: sink speed and weight on screen
	m    float64 // snow depth it adds where it settles
	ph   float64 // flutter phase
}

type vortex struct{ x, y, g, a float64 }

// pixel is what the cache knows about one screen cell.
type pixel struct {
	ch    byte
	col   uint8
	mat   uint8
	depth float64 // along the view ray to the scene, for hiding flakes
	glass bool    // inside the globe's outline, in front of the base
	cell  int32   // height map cell under the surface, -1 if no snow can lie here
	snowI float64 // light on snow lying here
	warm  float64 // how much of it is the lamp's
	ov    byte    // glass reflection drawn over everything
	ovCol uint8
}

const (
	sgN      = 48    // height map cells per side, over [-1, 1]
	sgFlakes = 2200  // all the snow, as flakes
	sgMass   = 0.028 // snow depth one flake adds to a cell
	sgElev   = 0.32  // camera elevation, radians
	sgGlass  = 0.975 // inner radius flakes can reach
	sgRepose = 1.1   // steepest slope snow holds
	sgLand   = 0.28  // flow speed above which snow won't settle
)

// Materials.
const (
	mNone = iota
	mGround
	mWall
	mRoof
	mWindow
	mDoor
	mStone
	mPine
	mTrunk
	mPost
	mBulb
	mSnowman
	mNose
	mCoal
	mBase
	mPlaque
)

var (
	sgCam    = Vec3{0, -math.Sin(sgElev), -math.Cos(sgElev)} // view direction
	sgUp     = Vec3{0, math.Cos(sgElev), -math.Sin(sgElev)}  // screen up
	sgMoon   = Vec3{-0.55, 0.7, 0.45}.Norm()
	sgCabin  = Vec3{-0.3, 0, -0.15}
	sgLampAt = Vec3{0.12, 0, 0.3}
	sgPines  = [][3]float64{{0.36, -0.28, 1}, {0.5, 0.12, 0.72}, {0.05, -0.5, 0.62}, {-0.55, -0.2, 0.55}}
	sgMan    = Vec3{-0.4, 0, 0.38}
)

// The village is modeled in its own units and shown sgK times bigger,
// raised by sgOff, so it fills more of the globe.
const sgK, sgOff = 1.3, 0.15

func sgLocal(p Vec3) Vec3 { return Vec3{p[0] / sgK, (p[1] - sgOff) / sgK, p[2] / sgK} }

// groundY is the ground's height in globe coordinates.
func groundY(x, z float64) float64 { return sgGround(x/sgK, z/sgK)*sgK + sgOff }

func sgGround(x, z float64) float64 {
	return -0.5 + 0.1*(1-(x*x+z*z)) + 0.012*math.Sin(5*x+1)*math.Cos(4*z)
}

func (g *snowglobe) Setup(ss *Streams) ([]*Gauge, error) {
	g.rng = rand.New(rand.NewSource(7))
	if _, err := ss.Subscribe("gravity", 30); err != nil {
		if _, err := ss.Subscribe("accelerometer", 30); err != nil {
			return nil, err
		}
	}
	ss.Subscribe("gyroscope", 30) // optional: twisting spins the liquid
	lin, err := ss.Subscribe("linear_acceleration", 50)
	if err != nil {
		return nil, err
	}
	g.down = [2]float64{0, -1}
	return gaugesFor(lin, 3), nil
}

func (g *snowglobe) Help() []string {
	return []string{
		"Shake the phone: the snow flies up and swirls through the whole globe, then drifts down and piles up on the ground, the roof, the pines and the snowman. The harder you shake, the more it picks up.",
		"Twist the phone to spin the liquid. Tilt it and the snow slides and falls that way.",
		"space  shake    r  start over",
	}
}

func (g *snowglobe) Key(k byte) {
	switch k {
	case ' ':
		g.kick = 0.5
	case 'r':
		g.w = 0
	}
}

// scene is the village's distance field inside the glass.
func (g *snowglobe) scene(p Vec3) (float64, uint8) {
	d, m := sceneLocal(sgLocal(p))
	return d * sgK, m
}

func sceneLocal(p Vec3) (float64, uint8) {
	d, m := (p[1]-sgGround(p[0], p[2]))*0.8, uint8(mGround)
	add := func(dd float64, mm uint8) {
		if dd < d {
			d, m = dd, mm
		}
	}

	// Cabin: log walls, a gabled roof, a stone chimney.
	c := sgCabin
	yb := sgGround(c[0], c[2]) - 0.02
	q := Vec3{p[0] - c[0], p[1] - yb, p[2] - c[2]}
	add(sdBox(q.Sub(Vec3{0, 0.12, 0}), Vec3{0.2, 0.12, 0.14}), mWall)
	r := q.Sub(Vec3{0, 0.24, 0})
	const rw, rh = 0.2, 0.16
	roof := math.Max(math.Max(-r[1], (math.Abs(r[2])*rh+r[1]*rw-rw*rh)/math.Hypot(rw, rh)), math.Abs(r[0])-0.24)
	add(roof, mRoof)
	add(sdBox(q.Sub(Vec3{0.1, 0.33, -0.05}), Vec3{0.03, 0.09, 0.03}), mStone)

	// Pines: three cones each on a trunk.
	for _, pn := range sgPines {
		s := pn[2]
		qb := Vec3{p[0] - pn[0], p[1] - sgGround(pn[0], pn[1]), p[2] - pn[1]}
		rr := math.Hypot(qb[0], qb[2])
		add(math.Max(rr-0.025*s, math.Max(-qb[1], qb[1]-0.12*s)), mTrunk)
		for _, tier := range [][3]float64{{0.08, 0.17, 0.22}, {0.2, 0.13, 0.19}, {0.31, 0.09, 0.16}} {
			y0, rad, hgt := tier[0]*s, tier[1]*s, tier[2]*s
			ty := qb[1] - y0
			cone := math.Max(-ty, (rr*hgt+ty*rad-rad*hgt)/math.Hypot(hgt, rad))
			add(cone, mPine)
		}
	}

	// Lamppost with a glowing head.
	l := sgLampAt
	ql := Vec3{p[0] - l[0], p[1] - sgGround(l[0], l[2]), p[2] - l[2]}
	add(math.Max(math.Hypot(ql[0], ql[2])-0.013, math.Max(-ql[1], ql[1]-0.4)), mPost)
	add(sdSphere(ql.Sub(Vec3{0, 0.43, 0}), 0, 0.04), mBulb)

	// Snowman.
	sm := sgMan
	qs := Vec3{p[0] - sm[0], p[1] - sgGround(sm[0], sm[2]), p[2] - sm[2]}
	body := smin(smin(sdSphere(qs.Sub(Vec3{0, 0.07, 0}), 0, 0.085), sdSphere(qs.Sub(Vec3{0, 0.2, 0}), 0, 0.062), 0.02),
		sdSphere(qs.Sub(Vec3{0, 0.3, 0}), 0, 0.045), 0.015)
	add(body, mSnowman)
	add(sdSphere(qs.Sub(Vec3{0, 0.3, 0.05}), 0, 0.016), mNose)
	return d, m
}

// base is the turned wooden stand outside the glass.
func sgBase(p Vec3) (float64, uint8) {
	const top, bot = -0.8, -1.3
	rho := math.Hypot(p[0], p[2])
	rad := 0.72 + (top-p[1])*0.42 - 0.03*math.Exp(-math.Pow((p[1]+1.02)/0.05, 2)) // a groove
	d := math.Max((rho-rad)*0.9, math.Max(p[1]-top, bot-p[1]))
	// The plaque, on the front face.
	pz := 0.72 + (top+1.1)*0.42
	pl := sdBox(Vec3{p[0], p[1] + 1.1, p[2] - pz}, Vec3{0.3, 0.075, 0.03})
	if pl < d {
		return pl, mPlaque
	}
	return d, mBase
}

func (g *snowglobe) all(p Vec3) (float64, uint8) {
	b, bm := sgBase(p)
	s, sm := g.scene(p)
	s = math.Max(s, p.Len()-0.99) // the village stays inside the glass
	if s < b {
		return s, sm
	}
	return b, bm
}

func (g *snowglobe) normal(p Vec3) Vec3 {
	const e = 0.002
	f := func(q Vec3) float64 { d, _ := g.all(q); return d }
	return Vec3{
		f(p.Add(Vec3{e, 0, 0})) - f(p.Sub(Vec3{e, 0, 0})),
		f(p.Add(Vec3{0, e, 0})) - f(p.Sub(Vec3{0, e, 0})),
		f(p.Add(Vec3{0, 0, e})) - f(p.Sub(Vec3{0, 0, e})),
	}.Norm()
}

func (g *snowglobe) shadow(p, dir Vec3) float64 {
	res, t := 1.0, 0.02
	for i := 0; i < 40 && t < 2; i++ {
		d, _ := g.scene(p.Add(dir.Scale(t)))
		if p.Add(dir.Scale(t)).Len() > 0.99 {
			break
		}
		if d < 0.001 {
			return 0.15
		}
		res = math.Min(res, 8*d/t)
		t += math.Max(d, 0.01)
	}
	return 0.15 + 0.85*clamp01(res)
}

func (g *snowglobe) lampPos() Vec3 {
	l := sgLampAt.Add(Vec3{0, sgGround(sgLampAt[0], sgLampAt[2]) + 0.43, 0})
	return Vec3{l[0] * sgK, l[1]*sgK + sgOff, l[2] * sgK}
}

// layout renders the static picture into the cache and builds the height
// map. It runs when the view size changes.
func (g *snowglobe) layout(w, h int) {
	g.w, g.h = w, h
	g.S = math.Min(float64(w)/2.2, 2*float64(h)/2.75)
	g.cx = float64(w) / 2
	g.cy = float64(h)/2 - 0.3*g.S/2
	g.px = make([]pixel, w*h)
	g.wbuf, g.zbuf = make([]float64, w*h), make([]float64, w*h)

	g.top = make([]float64, sgN*sgN)
	g.onGnd = make([]bool, sgN*sgN)
	for i := range g.top {
		x, z := g.cellXZ(i)
		g.top[i] = math.NaN()
		if x*x+z*z > 0.92 {
			continue
		}
		for y := 0.95; y > -0.99; {
			d, m := g.scene(Vec3{x, y, z})
			if d < 0.002 {
				if x*x+y*y+z*z < 0.99*0.99 {
					g.top[i], g.onGnd[i] = y, m == mGround
				}
				break
			}
			y -= math.Max(d, 0.003)
		}
	}
	lamp := g.lampPos()
	parallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			g.px[y*w+x] = g.render(x, y, lamp)
		}
	})
	g.plaque = [2]int{-1, -1}
	if u, v, ok := g.project(Vec3{0, -1.1, 0.72 + (-0.8+1.1)*0.42 + 0.03}); ok {
		if i := v*w + u; g.px[i].mat == mPlaque {
			g.plaque = [2]int{u, v}
		}
	}
	g.reset()
}

func (g *snowglobe) cellXZ(i int) (float64, float64) {
	return (float64(i%sgN)+0.5)/sgN*2 - 1, (float64(i/sgN)+0.5)/sgN*2 - 1
}

func (g *snowglobe) cellAt(x, z float64) int {
	cx, cz := int((x+1)/2*sgN), int((z+1)/2*sgN)
	if cx < 0 || cz < 0 || cx >= sgN || cz >= sgN {
		return -1
	}
	return cz*sgN + cx
}

// project maps a globe point to a screen cell.
func (g *snowglobe) project(p Vec3) (int, int, bool) {
	x := int(math.Floor(g.cx + p[0]*g.S))
	y := int(math.Floor(g.cy - p.Dot(sgUp)*g.S/2))
	return x, y, x >= 0 && y >= 0 && x < g.w && y < g.h
}

var (
	sgSoil    = []uint8{233, 234, 235, 58, 94}
	sgLog     = []uint8{52, 94, 94, 130, 136, 173}
	sgShingle = []uint8{52, 52, 88, 124, 131, 167}
	sgRock    = []uint8{237, 239, 242, 245, 248}
	sgGreen   = []uint8{22, 22, 23, 29, 29, 36, 72}
	sgBark    = []uint8{52, 58, 94}
	sgIron    = []uint8{235, 237, 240, 243}
	sgWhite   = []uint8{243, 245, 248, 251, 253, 255, 231}
	sgWood    = []uint8{52, 88, 94, 130, 136, 173, 180, 223}
	sgGold    = []uint8{94, 136, 178, 220, 221, 228}
	sgSnowC   = []uint8{245, 248, 250, 252, 254, 255, 231}
	sgSnowW   = []uint8{180, 223, 224, 230, 231}
	sgFlakeC  = []uint8{244, 248, 252, 255, 231}
)

func sgShade(ramp []uint8, i float64, chars string) (byte, uint8) {
	i = clamp01(i)
	return chars[int(i*float64(len(chars)-1)+0.5)], ramp[int(i*float64(len(ramp)-1)+0.5)]
}

const sgChars = ".:-=+*#%@"

func (g *snowglobe) render(x, y int, lamp Vec3) pixel {
	px := pixel{ch: ' ', cell: -1, depth: math.Inf(1)}
	u := (float64(x) + 0.5 - g.cx) / g.S
	v := (g.cy - float64(y) - 0.5) * 2 / g.S
	o := Vec3{u, 0, 0}.Add(sgUp.Scale(v)).Sub(sgCam.Scale(3))
	r2 := u*u + v*v
	inGlobe := r2 < 1

	t := 0.0
	hit := false
	var m uint8
	for i := 0; i < 160 && t < 6; i++ {
		d, mm := g.all(o.Add(sgCam.Scale(t)))
		if d < 0.0015 {
			hit, m = true, mm
			break
		}
		t += math.Max(d, 0.002)
	}
	// Where the ray enters and leaves the glass.
	b := o.Dot(sgCam)
	disc := b*b - (o.Dot(o) - 1)
	tIn, tOut := math.Inf(1), math.Inf(1)
	if disc > 0 {
		tIn, tOut = -b-math.Sqrt(disc), -b+math.Sqrt(disc)
	}
	px.glass = inGlobe && (!hit || m != mBase && m != mPlaque || t > tIn)
	if !hit {
		if inGlobe {
			// The painted night on the back of the glass: a few stars.
			e := o.Add(sgCam.Scale(tOut))
			px.depth = tOut
			if e[1] > -0.2 {
				hsh := hash2(int(e[0]*40), int(e[1]*40), 3)
				if hsh%23 == 0 {
					px.ch, px.col = '.', []uint8{60, 67, 103}[hsh>>8%3]
				}
			}
		}
		g.glassOver(&px, u, v, r2, inGlobe && px.glass)
		return px
	}
	p := o.Add(sgCam.Scale(t))
	n := g.normal(p)
	px.mat, px.depth = m, t
	if !px.glass {
		px.depth = math.Inf(-1) // the base hides whatever is behind it
	}

	var sh float64 = 1
	inside := p.Len() < 0.995
	if inside {
		sh = g.shadow(p, sgMoon)
	}
	diff := math.Max(0, n.Dot(sgMoon)) * sh
	lamp3 := 0.0
	if inside {
		ld := lamp.Sub(p)
		dist := ld.Len()
		lamp3 = math.Max(0, n.Dot(ld.Scale(1/dist))) * 0.9 / (1 + 30*dist*dist)
	}
	light := 0.12 + 0.75*diff + lamp3
	view := sgCam.Scale(-1)
	spec := math.Pow(math.Max(0, n.Dot(sgMoon.Add(view).Norm())), 30) * sh

	switch m {
	case mGround:
		wv := hash2(int(p[0]*60), int(p[2]*60), 1)
		px.ch, px.col = sgShade(sgSoil, light*0.8, ".,:;")
		if wv%7 == 0 {
			px.ch = '\''
		}
	case mWall:
		px.ch, px.col = sgShade(sgLog, light, "-=#")
		// Log courses, and the windows and the door on the front and side.
		pl := sgLocal(p)
		ly := pl[1] - (sgGround(sgCabin[0], sgCabin[2]) - 0.02)
		if math.Mod(ly*40, 1) < 0.3 {
			px.ch = '_'
		}
		lx, lz := pl[0]-sgCabin[0], pl[2]-sgCabin[2]
		switch {
		case lz > 0.13 && lx > -0.14 && lx < -0.05 && ly < 0.16:
			px.mat, px.ch, px.col = mDoor, '#', 52
		case lz > 0.13 && lx > 0.02 && lx < 0.14 && ly > 0.06 && ly < 0.17,
			lx > 0.19 && math.Abs(lz) < 0.06 && ly > 0.06 && ly < 0.17:
			px.mat, px.ch, px.col = mWindow, '#', 220
		}
	case mRoof:
		px.ch, px.col = sgShade(sgShingle, light+spec*0.3, "~=%#")
	case mStone:
		px.ch, px.col = sgShade(sgRock, light, ":o8#")
	case mPine:
		px.ch, px.col = sgShade(sgGreen, light*1.1, "^*A#")
	case mTrunk:
		px.ch, px.col = sgShade(sgBark, light, "|#")
	case mPost:
		px.ch, px.col = sgShade(sgIron, light+spec, "|")
	case mBulb:
		px.ch, px.col = '@', 230
	case mSnowman:
		px.ch, px.col = sgShade(sgWhite, light*1.1+spec*0.4, sgChars)
	case mNose:
		px.ch, px.col = '>', 208
	case mBase:
		px.ch, px.col = sgShade(sgWood, 0.1+0.65*diff+0.7*spec, "=+#%@")
	case mPlaque:
		px.ch, px.col = sgShade(sgGold, 0.3+0.5*diff+0.6*spec, "=")
	}

	// Snow can lie on surfaces that face up and are the top of their
	// height-map column (not under the roof's eaves).
	if inside && n[1] > 0.4 && m != mBulb && m != mPost {
		if c := g.cellAt(p[0], p[2]); c >= 0 && !math.IsNaN(g.top[c]) && math.Abs(g.top[c]-p[1]) < 0.04 {
			px.cell = int32(c)
			px.snowI = 0.3 + 0.6*math.Max(0, n.Dot(sgMoon))*sh + 0.8*lamp3 + 0.3*spec
			px.warm = clamp01(lamp3 * 3)
		}
	}
	g.glassOver(&px, u, v, r2, px.glass)
	return px
}

// glassOver adds the glass itself: a thin rim and a curved reflection of a
// window on the upper left.
func (g *snowglobe) glassOver(px *pixel, u, v, r2 float64, glass bool) {
	if !glass {
		return
	}
	r := math.Sqrt(r2)
	a := math.Atan2(v, u)
	switch {
	case r > 0.965:
		px.ov, px.ovCol = lineChar(-v, u*2), 110
	case r > 0.85 && r < 0.885 && a > 2.0 && a < 2.6:
		px.ov, px.ovCol = '\'', 152
	case math.Hypot(u+0.43, v-0.52) < 0.035:
		px.ov, px.ovCol = '*', 231
	}
}

// reset throws all the snow into the liquid, as if just shaken.
func (g *snowglobe) reset() {
	g.snow = make([]float64, sgN*sgN)
	g.flk = g.flk[:0]
	for len(g.flk) < sgFlakes {
		p := Vec3{g.rng.Float64()*2 - 1, g.rng.Float64()*2 - 1, g.rng.Float64()*2 - 1}
		if p.Len() > sgGlass || p[1] < groundY(p[0], p[2])+0.05 {
			continue
		}
		g.flk = append(g.flk, g.newFlake(p, Vec3{}))
	}
	g.vort = g.vort[:0]
	for i := 0; i < 8; i++ {
		g.spawn(12)
	}
}

func (g *snowglobe) newFlake(p, v Vec3) flake3 {
	return flake3{p: p, v: v, size: 0.6 + 0.8*g.rng.Float64(), ph: g.rng.Float64() * 6.3, m: sgMass}
}

// spawn adds a swirl whose strength follows the shake.
func (g *snowglobe) spawn(shake float64) {
	a, r := g.rng.Float64()*2*math.Pi, 0.75*math.Sqrt(g.rng.Float64())
	s := math.Min(0.3+0.05*shake, 1.4)
	if g.rng.Intn(2) == 0 {
		s = -s
	}
	vx := vortex{x: r * math.Cos(a), y: r * math.Sin(a), g: s * (0.5 + g.rng.Float64()), a: 0.12 + 0.18*g.rng.Float64()}
	if len(g.vort) < 28 {
		g.vort = append(g.vort, vx)
		return
	}
	weak := 0
	for i := range g.vort {
		if math.Abs(g.vort[i].g) < math.Abs(g.vort[weak].g) {
			weak = i
		}
	}
	g.vort[weak] = vx
}

// driftVortices moves each swirl along with the flow of the others, which
// makes the mixing chaotic instead of a few fixed whirlpools.
func (g *snowglobe) driftVortices(dt float64) {
	moved := make([][2]float64, len(g.vort))
	for i, vx := range g.vort {
		self := g.vort[i].g
		g.vort[i].g = 0 // not pushed by itself (its image still counts)
		ux, uy := g.flow(vx.x, vx.y)
		g.vort[i].g = self
		moved[i] = [2]float64{vx.x + ux*dt, vx.y + uy*dt}
	}
	for i, m := range moved {
		if r := math.Hypot(m[0], m[1]); r > 0.85 {
			m[0], m[1] = m[0]*0.85/r, m[1]*0.85/r
		}
		g.vort[i].x, g.vort[i].y = m[0], m[1]
	}
}

// flowAt is the flow at a point in the globe: each slice across the view
// is a smaller circle, so the flow is scaled to it and stays along its
// glass instead of pushing flakes into it.
func (g *snowglobe) flowAt(p Vec3) (float64, float64) {
	rho := math.Sqrt(math.Max(1-p[2]*p[2], 0.05))
	x, y := p[0]/rho, p[1]/rho
	ux, uy := g.flow(x, y)
	// The swirls' soft cores make the mirroring inexact; near the glass,
	// drop whatever still flows into it.
	if r := math.Hypot(x, y); r > 0.75 {
		nx, ny := x/r, y/r
		if un := ux*nx + uy*ny; un > 0 {
			k := smoothstep(0.75, 0.97, r)
			ux, uy = ux-un*nx*k, uy-un*ny*k
		}
	}
	return ux, uy
}

// flow is the liquid's velocity at (x, y): the swirls, each with its image
// in the glass so nothing flows out, and the spin.
func (g *snowglobe) flow(x, y float64) (float64, float64) {
	ux, uy := -g.spin*y, g.spin*x
	for _, vx := range g.vort {
		for k := 0; k < 2; k++ {
			cx, cy, s := vx.x, vx.y, vx.g
			if k == 1 { // image vortex: the circle theorem
				r2 := cx*cx + cy*cy
				if r2 < 0.01 {
					continue
				}
				cx, cy, s = cx/r2, cy/r2, -s
			}
			dx, dy := x-cx, y-cy
			core := vx.a * vx.a
			if k == 1 {
				core = 0 // the image is outside the glass: exact
			}
			f := s / (2 * math.Pi * (dx*dx + dy*dy + core))
			ux -= dy * f
			uy += dx * f
		}
	}
	return ux, uy
}

func (g *snowglobe) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	if v.W != g.w || v.H != g.h {
		g.layout(v.W, v.H)
	}
	dt = math.Min(dt, 0.1)
	g.sense(ss, dt)
	g.simulate(dt, t)
	g.compose(v, t)
}

// sense reads gravity (which way is down on screen), the shake, and the
// twist.
func (g *snowglobe) sense(ss *Streams, dt float64) {
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 3 {
		a := toScreen(ss, r.V)
		dx, dy := -a[0], -a[1]
		// Lying flat, "down" on screen is undefined: fall back to the
		// bottom of the screen.
		k := clamp01((math.Hypot(dx, dy) - 1.5) / 3)
		if l := math.Hypot(dx, dy); l > 0 {
			dx, dy = dx/l*k, dy/l*k-(1-k)
		}
		l := math.Hypot(dx, dy)
		g.down[0] += (dx/l - g.down[0]) * math.Min(1, dt*6)
		g.down[1] += (dy/l - g.down[1]) * math.Min(1, dt*6)
	}
	shake := Vec3{}
	if r := ss.Get("linear_acceleration").Read(); r.OK && len(r.V) >= 3 {
		shake = toScreen(ss, r.V)
	}
	if g.kick > 0 {
		g.kick -= dt
		shake = Vec3{25 * math.Sin(g.kick*40), 10 * math.Cos(g.kick*33), 0}
	}
	g.shake = shake
	g.agitate(dt)
	g.spawnFor(dt)
	if s := ss.Get("gyroscope"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			wz := toScreen(ss, r.V)[2]
			g.spin -= (wz - g.gyroZ) * 0.8 // the liquid lags the glass's turn
			g.gyroZ = wz
		}
	}
	g.spin *= math.Exp(-dt / 3)
	g.driftVortices(dt)
	for i := 0; i < len(g.vort); i++ {
		g.vort[i].g *= math.Exp(-dt / 3)
		if math.Abs(g.vort[i].g) < 0.05 {
			g.vort = append(g.vort[:i], g.vort[i+1:]...)
			i--
		}
	}
}

// agitate tracks the small-scale churning a shake leaves in the liquid.
func (g *snowglobe) agitate(dt float64) {
	g.agit = math.Max(g.agit*math.Exp(-dt/1.2), g.shake.Len())
}

// spawnFor adds swirls for dt of the current shake: more and stronger the
// harder it is.
func (g *snowglobe) spawnFor(dt float64) {
	mag := g.shake.Len()
	for n := g.rng.Float64(); n < math.Max(0, mag-2)*4*dt; n++ {
		g.spawn(mag)
	}
}

func (g *snowglobe) simulate(dt, t float64) {
	down := Vec3{g.down[0], g.down[1], 0}
	mag := g.shake.Len()

	// Flakes: carried by the liquid, sinking, lagging the glass's shakes.
	const steps = 2
	h := dt / steps
	for s := 0; s < steps; s++ {
		for i := 0; i < len(g.flk); i++ {
			f := &g.flk[i]
			ux, uy := g.flowAt(f.p)
			sink := 0.1 * f.size
			flutter := 0.04 * math.Sin(t*2.3+f.ph)
			target := Vec3{ux + down[0]*sink + flutter*down[1], uy + down[1]*sink - flutter*down[0], 0.03 * math.Sin(t*1.7+f.ph*2)}
			f.v = f.v.Add(target.Sub(f.v).Scale(math.Min(1, 10*h))).Sub(g.shake.Scale(0.05 * h))
			if g.agit > 2 { // jostled by eddies too small for the swirls
				k := 0.03 * g.agit
				f.v = f.v.Add(Vec3{(g.rng.Float64()*2 - 1) * k, (g.rng.Float64()*2 - 1) * k, (g.rng.Float64()*2 - 1) * k})
			}
			f.p = f.p.Add(f.v.Scale(h))
			if l := f.p.Len(); l > sgGlass {
				nrm := f.p.Scale(1 / l)
				f.p = nrm.Scale(sgGlass)
				if vn := f.v.Dot(nrm); vn > 0 {
					f.v = f.v.Sub(nrm.Scale(vn * 1.3))
				}
			}
			c := g.cellAt(f.p[0], f.p[2])
			for k := 0; k < 4 && (c < 0 || math.IsNaN(g.top[c])); k++ { // past the floor's edge: the nearest floor inward
				c = g.cellAt(f.p[0]*0.9, f.p[2]*0.9)
				f.p[0], f.p[2] = f.p[0]*0.97, f.p[2]*0.97
			}
			if c < 0 || math.IsNaN(g.top[c]) {
				continue
			}
			if surf := g.top[c] + g.snow[c]; f.p[1] < surf {
				if math.Hypot(ux, uy)+0.05*g.agit < sgLand { // calm enough to settle
					g.snow[c] += f.m
					g.flk[i] = g.flk[len(g.flk)-1]
					g.flk = g.flk[:len(g.flk)-1]
					i--
					continue
				}
				// Too stirred to settle. Over the ground it bounces; objects
				// it passes through, hidden by them, since the flow here
				// doesn't know about them and would pile flakes up on
				// their sides.
				if g.onGnd[c] {
					f.p[1] = surf + 0.005
					f.v[1] = math.Abs(f.v[1])*0.5 + math.Hypot(ux, uy)*0.3
				}
			}
		}
	}

	// Settled snow: currents and hard shakes pick it up.
	lift := math.Max(0, mag-5) * 0.25
	for c, d := range g.snow {
		if d < sgMass/4 {
			continue
		}
		x, z := g.cellXZ(c)
		y := g.top[c] + d
		rate := lift
		ux, uy := 0.0, 0.0
		if len(g.vort) > 0 || g.spin != 0 {
			ux, uy = g.flowAt(Vec3{x, y, z})
			rate += math.Max(0, math.Hypot(ux, uy)-sgLand) * 1.5
		}
		if rate == 0 {
			continue
		}
		for n := g.rng.Float64(); n < rate*dt*d/sgMass && g.snow[c] > 0; n++ {
			m := math.Min(g.snow[c], sgMass) // a thin layer lifts as lighter flakes
			g.snow[c] -= m
			p := Vec3{x + (g.rng.Float64()-0.5)*0.04, y + 0.01, z + (g.rng.Float64()-0.5)*0.04}
			fling := 0.2 + 0.03*mag // a hard shake throws it every way
			vel := Vec3{ux + (g.rng.Float64()*2-1)*fling, uy + 0.3 + 0.6*g.rng.Float64() + g.rng.Float64()*fling, (g.rng.Float64()*2 - 1) * fling}
			fl := g.newFlake(p, vel.Sub(down.Scale(0.4)))
			fl.m = m
			g.flk = append(g.flk, fl)
		}
	}
	g.slide(down)
}

// slide lets snow run down slopes steeper than it can hold, measured along
// the current down direction, so it slumps off steep branches and flows
// when the globe is tilted.
func (g *snowglobe) slide(down Vec3) {
	const cs = 2.0 / sgN
	upx, upy := -down[0], math.Max(-down[1], 0.3)
	for pass := 0; pass < 2; pass++ {
		for c := range g.snow {
			if g.snow[c] <= 0 || math.IsNaN(g.top[c]) {
				continue
			}
			cx, cz := c%sgN, c/sgN
			x, _ := g.cellXZ(c)
			pc := upx*x + upy*(g.top[c]+g.snow[c])
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, nz := cx+d[0], cz+d[1]
				if nx < 0 || nz < 0 || nx >= sgN || nz >= sgN {
					continue
				}
				n := nz*sgN + nx
				if math.IsNaN(g.top[n]) {
					continue
				}
				pn := upx*(x+float64(d[0])*cs) + upy*(g.top[n]+g.snow[n])
				if drop := pc - pn - sgRepose*cs; drop > 0 {
					m := math.Min(g.snow[c], drop/(2*upy)*0.5)
					g.snow[c] -= m
					g.snow[n] += m
					pc -= upy * m
				} else if g.snow[c] > g.snow[n] && math.Abs(g.top[c]-g.top[n]) < cs {
					// On level ground it evens out, reaching the rim
					// under the glass where flakes can't get.
					m := 0.02 * (g.snow[c] - g.snow[n])
					g.snow[c] -= m
					g.snow[n] += m
				}
			}
		}
	}
}

// compose draws the cached picture with the snow cover, the flakes and the
// glass on top.
func (g *snowglobe) compose(v *View, t float64) {
	w, h := g.w, g.h
	for i := range g.wbuf {
		g.wbuf[i], g.zbuf[i] = 0, math.Inf(1)
	}
	for _, f := range g.flk {
		x, y, ok := g.project(f.p)
		if !ok {
			continue
		}
		i := y*w + x
		depth := f.p.Dot(sgCam) + 3
		if depth > g.px[i].depth-0.01 {
			continue // behind the scene
		}
		g.wbuf[i] += f.size
		g.zbuf[i] = math.Min(g.zbuf[i], depth)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			p := &g.px[i]
			ch, col := p.ch, p.col
			if p.mat == mWindow { // firelight flickers
				col = []uint8{214, 220, 221, 220, 215}[int(t*9+float64(x*7+y*3))%5]
			}
			if p.cell >= 0 {
				if d := g.snow[p.cell]; d > 0 {
					cover := smoothstep(0.002, 0.015, d)
					if cover >= 1 || frac(hash2(x, y, 5), 0) < cover {
						ramp := sgSnowC
						if p.warm > 0.3 {
							ramp = sgSnowW
						}
						ch, col = sgShade(ramp, p.snowI+math.Min(d, 0.1), ":-=+*#%@")
						if cover < 1 {
							ch = ':'
						}
					}
				}
			}
			if wt := g.wbuf[i]; wt > 0 {
				near := clamp01((3.5 - g.zbuf[i]) / 1.2)
				ch = ".*%@"[min(int(wt/1.5), 3)]
				if wt < 1 && near < 0.4 {
					ch = '.'
				}
				col = sgFlakeC[int(near*float64(len(sgFlakeC)-1)+0.5)]
			}
			if p.ov != 0 {
				ch, col = p.ov, p.ovCol
			}
			v.Set(x, y, ch, col)
		}
	}
	if g.plaque[0] >= 0 {
		year := strconv.Itoa(time.Now().Year())
		text := "WINTER " + year
		if g.S < 30 {
			text = year
		}
		v.Text(g.plaque[0]-len(text)/2, g.plaque[1], text, 230)
	}
}
