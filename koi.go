package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "koi",
		desc: "a koi pond seen from above: ripples, caustics, lily pads; tap the phone to feed the fish, cover it to scare them",
		uses: []string{"proximity", "light", "linear_acceleration", "gravity"},
		new:  func(specs []string) Demo { return &pond{} },
	})
}

// pond is a koi pond seen from above. The water's surface is a height
// field that carries ripples (the wave equation, on the screen's grid):
// its slopes bend the view of the bottom, focus the sun into caustics on
// it and glint. Koi swim beneath, each a spine that follows its head, with
// a beating tail, fins and its variety's colors; they wander, keep off the
// edge and each other, sink and rise, and cast shadows on the bottom that
// fall further the higher they swim. Lily pads float on top, one or two in
// flower, drifting down the phone's tilt and on the ripples.
//
// A tap on the phone scatters food; the koi come up for it, each bite a
// ripple. Covering the proximity sensor casts a shadow on the pond and the
// fish bolt for the depths. In the dark (the light sensor), dusk falls on
// the pond and the moon shows in it.
//
// Units: x in columns, y in rows times two, so both are square.
type pond struct {
	rng       *rand.Rand
	W, H      int       // the grid, a point per cell
	cur, prev []float64 // the surface's ripples
	edge      []float64 // distance to the pond's edge (negative in the water)
	seed      float64
	fish      []koiFish
	pads      []lily
	food      []pellet
	fear      float64
	night     float64
	mode      int // 0 follow the light sensor, 1 day, 2 night
	lastAcc   float64
	tilt      [2]float64

	// The bank, drawn once: its characters, and how bright (stones above
	// 1, grass below).
	bankC []byte
	bankB []float64

	// This frame's fish and shadows over the bottom.
	fc     []byte
	fcol   []uint8
	shadow []float64
}

const koiSeg = 10

type koiFish struct {
	pts        [koiSeg][2]float64 // the spine, head first
	dir        float64            // heading
	speed      float64
	depth      float64 // 0 at the surface .. 1 on the bottom
	wantDepth  float64
	length     float64
	kind       int
	seed       float64
	phase      float64 // the tail's beat
	turn       float64 // its wandering
	nextChange float64
}

type lily struct {
	x, y, vx, vy, r, rot float64
	flower               bool
}

type pellet struct {
	x, y, age float64
}

// A variety's colors: its base, its patches, and spots.
type koiKind struct {
	base, patch, spot []uint8
	patchAt, spotAt   float64 // noise thresholds: patches above, spots above
}

var koiKinds = []koiKind{
	{[]uint8{244, 247, 250, 253, 255, 231}, []uint8{88, 124, 160, 196, 203, 210}, nil, 0.05, 2},                       // kohaku
	{[]uint8{94, 130, 172, 178, 214, 220, 227}, nil, nil, 2, 2},                                                       // ogon
	{[]uint8{232, 233, 234, 236, 238, 240}, []uint8{88, 124, 160, 196, 203, 210}, []uint8{250, 253, 255}, 0.15, 0.45}, // showa
	{[]uint8{130, 166, 202, 208, 209, 215}, nil, nil, 2, 2},                                                           // orange
	{[]uint8{237, 60, 67, 68, 110, 146}, []uint8{130, 166, 202, 208, 209}, nil, 0.35, 2},                              // asagi
	{[]uint8{244, 247, 250, 253, 255, 231}, []uint8{88, 124, 160, 196, 203, 210}, []uint8{232, 234, 236}, 0.1, 0.5},   // sanke
	{[]uint8{136, 178, 184, 220, 226, 229}, nil, nil, 2, 2},                                                           // yamabuki
}

var (
	pondFloor     = []uint8{232, 233, 233, 23, 23, 30}
	pondCaustic   = []uint8{23, 23, 30, 30, 37, 44}
	pondNight     = []uint8{232, 233, 17, 17, 18, 24}
	pondMoon      = []uint8{67, 110, 153, 189, 195, 231}
	pondStone     = []uint8{235, 237, 239, 241, 243, 245, 247, 249}
	pondGrass     = []uint8{232, 22, 22, 28, 34, 70}
	pondPad       = []uint8{22, 28, 34, 70, 76, 112, 148}
	pondLotus     = []uint8{125, 162, 168, 175, 218, 225, 231}
	pondSun       = [2]float64{-0.55, -0.83} // toward the sun, on the pond's plane (up-left)
	pondGlintRamp = []uint8{116, 152, 195, 231}
)

func (p *pond) Setup(ss *Streams) ([]*Gauge, error) {
	p.rng = rand.New(rand.NewSource(int64(rand.Uint32())))
	p.seed = p.rng.Float64() * 100
	st, err := ss.Subscribe("linear_acceleration", 60)
	if err != nil {
		return nil, err
	}
	ss.Subscribe("proximity", 0)
	ss.Subscribe("light", 0)
	ss.Subscribe("gravity", 30)
	return gaugesFor(st, 3), nil
}

func (p *pond) Help() []string {
	return []string{
		"Tap the phone to scatter food: the koi come up for it. Cover the top of the phone (the proximity sensor) to cast a shadow and scare them. Tilt it and the lily pads drift.",
		"In the dark, dusk falls on the pond (the light sensor).",
		"space  feed    n  day, night or follow the light    r  a new pond",
	}
}

func (p *pond) Key(k byte) {
	switch k {
	case ' ':
		p.feed()
	case 'n':
		p.mode = (p.mode + 1) % 3
	case 'r':
		p.seed = p.rng.Float64() * 100
		p.W = 0 // rebuilt on the next frame
	}
}

// build lays out the pond for a w x h grid: its edge, the koi and pads.
func (p *pond) build(w, h int) {
	p.W, p.H = w, h
	n := w * h
	p.cur, p.prev = make([]float64, n), make([]float64, n)
	p.edge = make([]float64, n)
	p.fc, p.fcol, p.shadow = make([]byte, n), make([]uint8, n), make([]float64, n)
	p.bankC, p.bankB = make([]byte, n), make([]float64, n)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			k := y*w + x
			fx, fy := float64(x)+0.5, 2*float64(y)+1
			p.edge[k] = p.edgeAt(fx, fy)
			if p.edge[k] > 0 {
				p.bankC[k], p.bankB[k] = p.bank(fx, fy, p.edge[k])
			}
		}
	}
	p.food = nil
	p.fish = p.fish[:0]
	L := math.Max(12, math.Min(40, float64(w)*0.36))
	for i := 0; i < 6; i++ {
		f := koiFish{kind: i % len(koiKinds), seed: p.rng.Float64() * 50, speed: 5}
		f.length = L * (0.8 + 0.35*p.rng.Float64())
		f.dir = p.rng.Float64() * 2 * math.Pi
		f.depth = 0.2 + 0.6*p.rng.Float64()
		f.wantDepth = f.depth
		x, y := p.randomSpot(f.length * 0.5)
		for k := range f.pts {
			s := float64(k) * f.length / (koiSeg - 1)
			f.pts[k] = [2]float64{x - math.Cos(f.dir)*s, y - math.Sin(f.dir)*s}
		}
		p.fish = append(p.fish, f)
	}
	p.pads = p.pads[:0]
	for i := 0; i < 4; i++ {
		r := L * (0.22 + 0.13*p.rng.Float64())
		var x, y float64
		for tries := 0; tries < 50; tries++ { // apart from the others
			x, y = p.randomSpot(r * 1.5)
			ok := true
			for _, o := range p.pads {
				if math.Hypot(x-o.x, y-o.y) < (r+o.r)*1.6 {
					ok = false
				}
			}
			if ok {
				break
			}
		}
		p.pads = append(p.pads, lily{x: x, y: y, r: r, rot: p.rng.Float64() * 2 * math.Pi, flower: i < 2})
	}
}

// edgeAt is the distance from (x, y) to the pond's edge, roughly: a
// rounded rectangle, its bank wavering.
func (p *pond) edgeAt(x, y float64) float64 {
	hw, hh := float64(p.W)/2, float64(p.H)
	nx, ny := (x-hw)/hw, (y-hh)/hh
	r := math.Pow(math.Pow(math.Abs(nx), 3.5)+math.Pow(math.Abs(ny), 3.5), 1/3.5)
	th := math.Atan2(ny, nx)
	rr := 0.84 + 0.05*snoise(math.Cos(th)*1.5, math.Sin(th)*1.5, p.seed) + 0.025*snoise(math.Cos(th)*4, math.Sin(th)*4, p.seed+3)
	return (r - rr) * math.Min(hw, hh)
}

func (p *pond) edgeGrid(x, y float64) float64 {
	i, j := int(x), int(y/2)
	if i < 0 || j < 0 || i >= p.W || j >= p.H {
		return 10
	}
	return p.edge[j*p.W+i]
}

// randomSpot picks a spot in the water at least margin from the bank, or
// if the pond has none that far in, the deepest it came across.
func (p *pond) randomSpot(margin float64) (float64, float64) {
	bx, by, be := float64(p.W)/2, float64(p.H), math.Inf(1)
	for tries := 0; tries < 300; tries++ {
		x := p.rng.Float64() * float64(p.W)
		y := p.rng.Float64() * float64(p.H) * 2
		e := p.edgeAt(x, y)
		if e < -margin {
			return x, y
		}
		if e < be {
			bx, by, be = x, y, e
		}
	}
	return bx, by
}

// feed scatters a handful of pellets over a spot, each landing with a
// ripple.
func (p *pond) feed() {
	if p.W == 0 {
		return
	}
	x, y := p.randomSpot(10)
	for i := 0; i < 7 && len(p.food) < 40; i++ {
		fx := x + (p.rng.Float64()-0.5)*10
		fy := y + (p.rng.Float64()-0.5)*10
		if p.edgeAt(fx, fy) < -2 {
			p.food = append(p.food, pellet{x: fx, y: fy})
			p.splash(fx, fy, 1.5, 0.5)
		}
	}
}

// splash pushes the surface down around (x, y).
func (p *pond) splash(x, y, r, amp float64) {
	for j := int((y - r*2) / 2); j <= int((y+r*2)/2)+1; j++ {
		for i := int(x - r - 1); i <= int(x+r+1); i++ {
			if i < 1 || j < 1 || i >= p.W-1 || j >= p.H-1 {
				continue
			}
			dx, dy := float64(i)+0.5-x, 2*float64(j)+1-y
			if d2 := (dx*dx + dy*dy) / (r * r); d2 < 1 {
				p.cur[j*p.W+i] -= amp * (1 - d2)
			}
		}
	}
}

// ripple advances the surface: the wave equation, a row being twice as
// tall as a column is wide; the bank holds still.
func (p *pond) ripple(dt float64) {
	w, h := p.W, p.H
	c2 := 14.0 * 14.0 * dt * dt
	for j := 1; j < h-1; j++ {
		for i := 1; i < w-1; i++ {
			k := j*w + i
			if p.edge[k] > 0 {
				p.prev[k] = 0
				continue
			}
			lap := p.cur[k-1] + p.cur[k+1] - 2*p.cur[k] + (p.cur[k-w]+p.cur[k+w]-2*p.cur[k])/4
			p.prev[k] = (2*p.cur[k] - p.prev[k] + c2*lap) * 0.992
		}
	}
	p.cur, p.prev = p.prev, p.cur
}

func (p *pond) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	dt = math.Min(dt, 0.1)
	if v.W != p.W || v.H != p.H {
		p.build(v.W, v.H)
	}
	p.sense(ss, dt)
	for i := 0; i < 2; i++ {
		p.ripple(dt / 2)
	}
	p.swim(dt, t)
	p.drift(dt)
	p.render(v, t)
}

func (p *pond) sense(ss *Streams, dt float64) {
	if s := ss.Get("linear_acceleration"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			m := math.Sqrt(r.V[0]*r.V[0] + r.V[1]*r.V[1] + r.V[2]*r.V[2])
			if m > 4 && p.lastAcc < 2 { // a tap: sharp, from stillness
				p.feed()
			}
			p.lastAcc = p.lastAcc*0.8 + m*0.2
		}
	}
	scared := false
	if s := ss.Get("proximity"); s != nil {
		if r := s.Read(); r.OK && len(r.V) > 0 && r.V[0] < 3 {
			scared = true
		}
	}
	if scared {
		p.fear = math.Min(1, p.fear+dt*4)
	} else {
		p.fear = math.Max(0, p.fear-dt*0.25)
	}
	want := 0.0
	switch p.mode {
	case 0:
		if s := ss.Get("light"); s != nil {
			if r := s.Read(); r.OK && len(r.V) > 0 {
				want = 1 - smoothstep(3, 40, r.V[0])
			}
		}
	case 2:
		want = 1
	}
	p.night += (want - p.night) * math.Min(1, dt*0.7)
	p.tilt = [2]float64{}
	if s := ss.Get("gravity"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 3 {
			g := toScreen(ss, r.V)
			p.tilt = [2]float64{-g[0] / 9.8, g[1] / 9.8} // downhill, on screen
		}
	}
}

// swim steers each koi: wandering, off the bank and away from the others,
// toward food, or away from a fright; then its body follows its head.
func (p *pond) swim(dt, t float64) {
	cx, cy := float64(p.W)/2, float64(p.H)
	for fi := range p.fish {
		f := &p.fish[fi]
		hx, hy := f.pts[0][0], f.pts[0][1]
		f.turn += (p.rng.Float64() - 0.5) * dt * 4
		f.turn *= math.Pow(0.6, dt)
		steer := f.turn * 0.9
		cruise := 4.5 + 1.5*math.Sin(t*0.1+f.seed)
		want := cruise
		if t > f.nextChange {
			f.wantDepth = 0.15 + 0.7*p.rng.Float64()
			f.nextChange = t + 6 + 10*p.rng.Float64()
		}
		wd := f.wantDepth
		// Food: the nearest pellet it can see draws it up.
		best, bd := -1, 45.0*45.0
		for i, pe := range p.food {
			dx, dy := pe.x-hx, pe.y-hy
			if d := dx*dx + dy*dy; d < bd {
				best, bd = i, d
			}
		}
		if best >= 0 && p.fear < 0.3 {
			pe := p.food[best]
			steer += angleTo(f.dir, math.Atan2(pe.y-hy, pe.x-hx)) * 2.5
			want = cruise * 1.6
			wd = 0
			if math.Sqrt(bd) < 2+f.length*0.06 && f.depth < 0.2 {
				p.food = append(p.food[:best], p.food[best+1:]...)
				p.splash(pe.x, pe.y, 2, 0.8)
			}
		}
		// A fright: away from the shadow over the middle, and down.
		if p.fear > 0.05 {
			away := math.Atan2(hy-cy, hx-cx)
			steer += angleTo(f.dir, away) * 3 * p.fear
			want = cruise * (1 + 2.2*p.fear)
			wd = 0.95
		}
		// Others: turn away from a head close ahead.
		for oi := range p.fish {
			if oi == fi {
				continue
			}
			o := &p.fish[oi]
			dx, dy := o.pts[koiSeg/3][0]-hx, o.pts[koiSeg/3][1]-hy
			if d := math.Hypot(dx, dy); d < f.length*0.7 && math.Abs(o.depth-f.depth) < 0.35 {
				side := math.Cos(f.dir)*dy - math.Sin(f.dir)*dx
				steer -= math.Copysign(1.2*(1-d/(f.length*0.7)), side)
			}
		}
		// The bank: look ahead, and turn in.
		look := f.length*0.6 + f.speed*0.8
		ax, ay := hx+math.Cos(f.dir)*look, hy+math.Sin(f.dir)*look
		if e := p.edgeGrid(ax, ay); e > -f.length*0.35 {
			gx := p.edgeGrid(ax+2, ay) - p.edgeGrid(ax-2, ay)
			gy := p.edgeGrid(ax, ay+2) - p.edgeGrid(ax, ay-2)
			in := math.Atan2(-gy, -gx)
			steer += angleTo(f.dir, in) * 3 * smoothstep(-f.length*0.35, 2, e)
		}
		maxTurn := 1.4 + 0.25*f.speed
		steer = math.Max(-maxTurn, math.Min(maxTurn, steer))
		f.dir += steer * dt
		f.speed += (want - f.speed) * math.Min(1, dt*1.5)
		f.depth += (wd - f.depth) * math.Min(1, dt*0.6)
		f.phase += dt * (2 + f.speed*0.9)
		f.pts[0][0] += math.Cos(f.dir) * f.speed * dt
		f.pts[0][1] += math.Sin(f.dir) * f.speed * dt
		// Never onto the bank.
		if e := p.edgeGrid(f.pts[0][0], f.pts[0][1]); e > -1 {
			f.pts[0][0] += (cx - f.pts[0][0]) * 0.02
			f.pts[0][1] += (cy - f.pts[0][1]) * 0.02
		}
		seg := f.length / (koiSeg - 1)
		for k := 1; k < koiSeg; k++ {
			dx, dy := f.pts[k][0]-f.pts[k-1][0], f.pts[k][1]-f.pts[k-1][1]
			d := math.Max(1e-9, math.Hypot(dx, dy))
			f.pts[k] = [2]float64{f.pts[k-1][0] + dx/d*seg, f.pts[k-1][1] + dy/d*seg}
		}
		// Near the surface, a swimming koi wrinkles it.
		if f.depth < 0.12 && p.rng.Float64() < dt*3 {
			p.splash(f.pts[koiSeg/2][0], f.pts[koiSeg/2][1], 2.5, 0.25)
		}
	}
	for i := range p.food {
		p.food[i].age += dt
	}
}

// snoise is noise3 centered: -1..1.
func snoise(x, y, z float64) float64 { return 2*noise3(x, y, z) - 1 }

// angleTo is the turn from a to b, -pi..pi.
func angleTo(a, b float64) float64 {
	return math.Remainder(b-a, 2*math.Pi)
}

// drift moves the lily pads: down the tilt, on the ripples' slopes, apart
// from each other, off the bank.
func (p *pond) drift(dt float64) {
	for i := range p.pads {
		pd := &p.pads[i]
		gx, gy := p.slope(pd.x, pd.y)
		pd.vx += (p.tilt[0]*3 - gx*40 - pd.vx*0.6) * dt
		pd.vy += (p.tilt[1]*3 - gy*40 - pd.vy*0.6) * dt
		for j := range p.pads {
			if j == i {
				continue
			}
			o := &p.pads[j]
			dx, dy := pd.x-o.x, pd.y-o.y
			if d := math.Hypot(dx, dy); d < pd.r+o.r && d > 1e-6 {
				push := (pd.r + o.r - d) * 2
				pd.vx += dx / d * push * dt
				pd.vy += dy / d * push * dt
			}
		}
		if e := p.edgeGrid(pd.x, pd.y); e > -pd.r {
			gx := p.edgeGrid(pd.x+2, pd.y) - p.edgeGrid(pd.x-2, pd.y)
			gy := p.edgeGrid(pd.x, pd.y+2) - p.edgeGrid(pd.x, pd.y-2)
			n := math.Max(1e-9, math.Hypot(gx, gy))
			k := (e + pd.r) * 3
			pd.vx -= gx / n * k * dt
			pd.vy -= gy / n * k * dt
		}
		pd.x += pd.vx * dt
		pd.y += pd.vy * dt
		pd.rot += (pd.vx*0.02 - pd.vy*0.01) * dt
	}
}

// slope is the surface's slope at (x, y), from the grid.
func (p *pond) slope(x, y float64) (float64, float64) {
	i, j := int(x), int(y/2)
	if i < 1 || j < 1 || i >= p.W-1 || j >= p.H-1 {
		return 0, 0
	}
	k := j*p.W + i
	return (p.cur[k+1] - p.cur[k-1]) / 2, (p.cur[k+p.W] - p.cur[k-p.W]) / 4
}

// koiWidth is a koi's half width along its body (u from the nose, 0, to
// where the tail fin starts, 0.8), in lengths.
func koiWidth(u float64) float64 {
	if u <= 0 || u >= 0.8 {
		return 0
	}
	return koiWidthT[int(u/0.8*float64(len(koiWidthT)-1))]
}

var koiWidthT = func() (t [512]float64) {
	for i := range t {
		s := float64(i) / float64(len(t)-1)
		t[i] = 0.16 * math.Pow(math.Sin(math.Pi*math.Pow(s, 0.55)), 0.9) * (1 - 0.55*s)
	}
	return
}()

// rasterFish draws the koi into the frame's fish and shadow buffers:
// deepest first, so shallower ones swim over them.
func (p *pond) rasterFish(t float64) {
	clear(p.fc)
	clear(p.shadow)
	order := make([]int, len(p.fish))
	for i := range order {
		order[i] = i
	}
	for a := 1; a < len(order); a++ {
		for b := a; b > 0 && p.fish[order[b]].depth > p.fish[order[b-1]].depth; b-- {
			order[b], order[b-1] = order[b-1], order[b]
		}
	}
	dim := 1 - 0.55*p.night
	for _, fi := range order {
		f := &p.fish[fi]
		// The spine as drawn: the tail beating side to side.
		var sp [koiSeg][2]float64
		amp := f.length * (0.035 + 0.012*f.speed)
		for k := range sp {
			sp[k] = f.pts[k]
			if k == 0 {
				continue
			}
			k0 := max(0, k-1)
			dx, dy := f.pts[k][0]-f.pts[k0][0], f.pts[k][1]-f.pts[k0][1]
			d := math.Max(1e-9, math.Hypot(dx, dy))
			s := float64(k) / (koiSeg - 1)
			off := amp * math.Sin(f.phase*2-float64(k)*0.75) * s * s
			sp[k][0] += -dy / d * off
			sp[k][1] += dx / d * off
		}
		kind := koiKinds[f.kind]
		lift := (1 - f.depth) * 7 // how far its shadow falls
		sx, sy := -pondSun[0]*lift, -pondSun[1]*lift
		minX, minY, maxX, maxY := 1e9, 1e9, -1e9, -1e9
		for _, q := range sp {
			minX, maxX = math.Min(minX, q[0]), math.Max(maxX, q[0])
			minY, maxY = math.Min(minY, q[1]), math.Max(maxY, q[1])
		}
		pad := f.length * 0.3
		// Its shadow is its outline, moved.
		si, sj := int(math.Round(sx)), int(math.Round(sy/2))
		shadowAmt := 0.55 * (1 - 0.5*f.depth)
		{
			for j := max(0, int((minY-pad)/2)); j <= min(p.H-1, int((maxY+pad)/2)); j++ {
				for i := max(0, int(minX-pad)); i <= min(p.W-1, int(maxX+pad)); i++ {
					qx, qy := float64(i)+0.5, 2*float64(j)+1
					u, v := spineCoords(&sp, qx, qy, f.length)
					cell := j*p.W + i
					body := koiWidth(u)
					inBody := math.Abs(v) < body
					// Pectoral fins, and the tail's forked fan.
					fin := false
					var rayX, rayY float64 // which way the fin's rays run, from its root
					if u > 0.16 && u < 0.3 {
						fu := (u - 0.16) / 0.14
						reach := body + 0.075*math.Sin(math.Pi*fu)*(1-fu*0.5)
						fin = math.Abs(v) < reach
						k := 2 // near where the fins grow
						rayX, rayY = qx-sp[k][0], qy-sp[k][1]
					}
					if u >= 0.78 && u < 1.02 {
						tu := (u - 0.78) / 0.24
						fin = math.Abs(v) < 0.02+0.1*tu && !(tu > 0.6 && math.Abs(v) < (tu-0.6)*0.16)
						rayX, rayY = qx-sp[koiSeg-3][0], qy-sp[koiSeg-3][1]
					}
					if !inBody && !fin {
						continue
					}
					if ti, tj := i+si, j+sj; ti >= 0 && tj >= 0 && ti < p.W && tj < p.H {
						sc := tj*p.W + ti
						p.shadow[sc] = math.Max(p.shadow[sc], shadowAmt)
					}
					if !inBody {
						// Fins: translucent, the body's color, faint, in rays.
						p.fc[cell] = lineGlyph(rayX, rayY/2)
						p.fcol[cell] = mzPick(kind.base, 0.6*dim*(1-0.5*f.depth))
						continue
					}
					// The back, lit from the sun's side and rounded.
					across := v / body
					b := (0.55 + 0.45*(1-across*across)) * (1 - 0.5*f.depth) * dim
					b += 0.08 * across * boolF(f.dir > 0)
					ramp := kind.base
					n := snoise(u*6+f.seed, across*1.6, f.seed*1.7)
					if kind.patch != nil && n > kind.patchAt {
						ramp = kind.patch
					}
					if kind.spot != nil && snoise(u*14+f.seed, across*3, f.seed+9) > kind.spotAt {
						ramp = kind.spot
					}
					if u < 0.06 {
						b *= 0.8 // the nose
					}
					const solid = "=+#%@"
					p.fc[cell] = solid[int(clamp01(b)*4+0.5)]
					p.fcol[cell] = mzPick(ramp, b)
				}
			}
		}
	}
}

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return -1
}

// spineCoords finds where (x, y) lies along a koi's spine: u from the nose
// (0) to the tail's tip (1), and v across it, both in body lengths.
func spineCoords(sp *[koiSeg][2]float64, x, y, length float64) (float64, float64) {
	best, bu, bv := math.Inf(1), -1.0, 0.0
	for k := 0; k < koiSeg-1; k++ {
		ax, ay := sp[k][0], sp[k][1]
		dx, dy := sp[k+1][0]-ax, sp[k+1][1]-ay
		l2 := dx*dx + dy*dy
		t := ((x-ax)*dx + (y-ay)*dy) / math.Max(l2, 1e-9)
		lo, hi := 0.0, 1.0
		if k == 0 {
			lo = -0.3 // past the nose, to round it
		}
		t = math.Max(lo, math.Min(hi, t))
		px, py := ax+dx*t, ay+dy*t
		d := (x-px)*(x-px) + (y-py)*(y-py)
		if d < best {
			best = d
			l := math.Sqrt(l2)
			bu = (float64(k) + t) / (koiSeg - 1)
			bv = ((x-ax)*dy - (y-ay)*dx) / math.Max(l, 1e-9) / length
		}
	}
	if bu < 0 {
		return -1, 0
	}
	return bu, bv
}

func (p *pond) render(v *View, t float64) {
	p.rasterFish(t)
	w, h := p.W, p.H
	night := p.night
	day := 1 - night
	moonX, moonY := float64(w)*0.7, float64(h)*0.45
	parallelRows(h, func(y int) {
		for x := 0; x < w; x++ {
			k := y*w + x
			fx, fy := float64(x)+0.5, 2*float64(y)+1
			e := p.edge[k]
			if e > 0 {
				dim := 1 - 0.6*night
				if b := p.bankB[k]; b >= 1 {
					v.Set(x, y, p.bankC[k], mzPick(pondStone, (b-1)*dim))
				} else {
					v.Set(x, y, p.bankC[k], mzPick(pondGrass, b*dim))
				}
				continue
			}
			// The surface: ripples, and a breeze's small waves.
			gx, gy := 0.0, 0.0
			if x > 0 && x < w-1 && y > 0 && y < h-1 {
				gx = (p.cur[k+1] - p.cur[k-1]) / 2
				gy = (p.cur[k+w] - p.cur[k-w]) / 4
			}
			bz := 0.07
			gx += bz * (snoise(fx*0.18+0.4, fy*0.18, t*0.4) - snoise(fx*0.18-0.4, fy*0.18, t*0.4))
			gy += bz * (snoise(fx*0.18, fy*0.18+0.4, t*0.4) - snoise(fx*0.18, fy*0.18-0.4, t*0.4))
			// The bottom, seen through it, bent by its slopes.
			rx, ry := fx+gx*6, fy+gy*6
			deep := smoothstep(0, 16, -e)
			ch, col := byte(0), uint8(0)
			// The koi, also bent.
			fi, fj := int(fx+gx*3), int((fy+gy*3)/2)
			if fi >= 0 && fj >= 0 && fi < w && fj < h && p.fc[fj*w+fi] != 0 {
				ch, col = p.fc[fj*w+fi], p.fcol[fj*w+fi]
			} else {
				shade := 1.0
				if si, sj := int(rx), int(ry/2); si >= 0 && sj >= 0 && si < w && sj < h {
					shade -= p.shadow[sj*w+si]
				}
				ch, col = p.bottom(rx, ry, t, deep, shade, day)
			}
			// Surface glints: the sun (or the moon) in the ripples' slopes.
			sl := gx*pondSun[0] + gy*pondSun[1]
			if day > 0.5 {
				if g := sl*6 - 0.55 + 0.25*snoise(fx*0.5, fy*0.5, t*2); g > 0 {
					ch, col = "-~*"[min(2, int(g*4))], mzPick(pondGlintRamp, g*2)
				}
			} else {
				mx, my := rx-moonX, (ry-moonY)*0.8
				if d := math.Hypot(mx, my); d < 5+sl*6 {
					b := 1 - d/(5+math.Max(0, sl*6))
					ch, col = "-~=*"[min(3, int(b*4))], mzPick(pondMoon, 0.4+0.6*b)
				} else if g := sl*6 - 0.8; g > 0 {
					ch, col = '~', mzPick(pondMoon, 0.3+g)
				}
			}
			v.Set(x, y, ch, col)
		}
	})
	p.drawPads(v, t)
	for _, pe := range p.food {
		v.Set(int(pe.x), int(pe.y/2), 'o', 179)
	}
}

// bottom is the pond's floor at (x, y): pebbles, darker in the deep middle,
// under the sun's caustics (by day), in the shadows over it.
func (p *pond) bottom(x, y, t, deep, shade, day float64) (byte, uint8) {
	peb := snoise(x*0.3, y*0.3, p.seed) + 0.5*snoise(x*0.9, y*0.9, p.seed+4)
	b := (0.4 + 0.3*peb) * (1 - 0.5*deep) * shade
	// Caustics: bright threads where two slowly moving fields cross zero.
	c1 := math.Abs(snoise(x*0.09+t*0.05, y*0.09, t*0.3))
	c2 := math.Abs(snoise(x*0.13, y*0.13-t*0.04, t*0.25+5))
	c := (1-smoothstep(0.02, 0.1, c1))*0.8 + (1-smoothstep(0.02, 0.07, c2))*0.5
	c *= day * shade * shade * (1 - 0.4*deep)
	if day < 0.5 {
		b *= 0.8
		return ".,:"[int(clamp01(b)*2.99)], mzPick(pondNight, b*0.9)
	}
	if c > 0.3 {
		// Drawn along the thread: across the stronger field's slope.
		fx, fy, sc := 0.09, 0.09, 0.0
		if c2 > c1 {
			fx, fy, sc = 0.13, 0.13, 1
		}
		n := func(dx, dy float64) float64 {
			if sc == 0 {
				return snoise((x+dx)*fx+t*0.05, (y+dy)*fy, t*0.3)
			}
			return snoise((x+dx)*fx, (y+dy)*fy-t*0.04, t*0.25+5)
		}
		gx, gy := n(0.5, 0)-n(-0.5, 0), n(0, 0.5)-n(0, -0.5)
		return lineGlyph(-gy, gx/2), mzPick(pondCaustic, b*0.3+(c-0.3)*1.2)
	}
	chars := ".,:;"
	return chars[int(clamp01(peb*0.5+0.5)*3.99)], mzPick(pondFloor, b)
}

// bank is the pond's rocky edge and the grass beyond: a character and a
// brightness, stones' plus 1.
func (p *pond) bank(x, y, e float64) (byte, float64) {
	if e < 5 {
		// Stones: the nearest of scattered centers, each rounded.
		cs := 5.0
		gi, gj := math.Floor(x/cs), math.Floor(y/cs)
		d1, d2 := math.Inf(1), math.Inf(1)
		var near [2]float64
		for dj := -1.0; dj <= 1; dj++ {
			for di := -1.0; di <= 1; di++ {
				ci, cj := gi+di, gj+dj
				jx := 0.5 + 0.4*snoise(ci*1.7, cj*1.3, p.seed+11)
				jy := 0.5 + 0.4*snoise(ci*1.1, cj*1.9, p.seed+13)
				px, py := (ci+jx)*cs, (cj+jy)*cs
				d := math.Hypot(x-px, y-py)
				if d < d1 {
					d2, d1, near = d1, d, [2]float64{x - px, y - py}
				} else if d < d2 {
					d2 = d
				}
			}
		}
		if d2-d1 > 0.9 {
			// Rounded: lit from the sun's side.
			r := d1 / (cs * 0.7)
			lit := 0.5 + 0.5*(-(near[0]*pondSun[0]+near[1]*pondSun[1])/math.Max(d1, 1e-9))*math.Min(1, r*1.5)
			b := clamp01(0.35 + 0.5*lit*(1-r*r*0.6))
			const solid = "=+#%@"
			return solid[int(b*4+0.5)], 1 + b
		}
		return '.', 0.4
	}
	g := snoise(x*0.4, y*0.4, p.seed+20)
	chars := `.,'"`
	return chars[int(clamp01(g*0.5+0.5)*3.99)], clamp01(0.45 + 0.3*g)
}

// drawPads floats the lily pads over the water: each a disc with a notch
// and veins from the middle; flowers on some.
func (p *pond) drawPads(v *View, t float64) {
	dim := 1 - 0.55*p.night
	for _, pd := range p.pads {
		for j := max(0, int((pd.y-pd.r)/2)); j <= min(p.H-1, int((pd.y+pd.r)/2)); j++ {
			for i := max(0, int(pd.x-pd.r)); i <= min(p.W-1, int(pd.x+pd.r)); i++ {
				dx, dy := float64(i)+0.5-pd.x, 2*float64(j)+1-pd.y
				d := math.Hypot(dx, dy)
				if d > pd.r || p.edge[j*p.W+i] > 0 {
					continue
				}
				th := math.Atan2(dy, dx)
				if math.Abs(angleTo(pd.rot, th)) < 0.28 && d > pd.r*0.08 {
					continue // the notch
				}
				// Lit from the sun's side, with a rim, and veins.
				lit := 0.6 + 0.25*(-(dx*pondSun[0]+dy*pondSun[1])/math.Max(d, 1e-9))*(d/pd.r)
				b := lit * (1 - 0.3*smoothstep(0.8, 1, d/pd.r)) * dim
				ch := "=+#%@"[int(clamp01(b)*4+0.5)]
				vein := math.Mod(th-pd.rot+10*math.Pi, 2*math.Pi/9)
				if vein < 0.1 && d > pd.r*0.2 && d < pd.r*0.85 {
					ch = lineGlyph(dx, dy/2)
					b *= 0.75
				}
				if pd.flower && d < pd.r*0.55 {
					// A lotus: petals around a golden heart.
					pr := pd.r * 0.55 * (0.55 + 0.45*math.Abs(math.Cos(th*4+pd.rot)))
					if d < pr {
						pb := (0.5 + 0.5*(1-d/pr)) * dim
						if d < pd.r*0.12 {
							v.Set(i, j, '@', mzPick([]uint8{136, 178, 220, 226}, pb))
						} else {
							v.Set(i, j, "+#%@"[int(clamp01(pb)*3+0.5)], mzPick(pondLotus, pb))
						}
						continue
					}
				}
				v.Set(i, j, ch, mzPick(pondPad, b))
			}
		}
	}
}
