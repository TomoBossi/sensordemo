package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

func init() {
	register(entry{
		name: "maze",
		desc: "a marble maze: tilt the phone to roll a ball to the goal, across narrow bridges over the void",
		uses: []string{"gravity", "accelerometer"},
		new:  func(specs []string) Demo { return &maze{level: 1} },
	})
}

// maze is a tilt game on a small board seen from above. The ball moves in
// continuous space with momentum, rolls (its stripes turn with it) and
// bounces off walls. Some stretches have no walls at all: the passage is
// as wide as anywhere else, but where a wall would stand there is a drop
// into the void, and falling sends the ball back to the start. Holes in
// dead ends do the same. Each solved board has more and longer bridges.
//
// The picture is a tiny 3D renderer, per character, without ray marching:
// the view looks straight down in the room, so tilting the phone shows the
// walls' sides and the edges of the board (parallax), a light from above
// shades walls and ball and casts shadows, and the ball is a real sphere.
//
// World units are cells; x right, y down the screen, z up out of the board.
type maze struct {
	seed       int64
	level      int
	cols, rows int
	cells      []mcell
	walls      []box
	near       [][]int // per cell: the walls that can touch it
	holes      [][2]float64
	start      [2]float64
	goal       [2]float64

	x, y, z    float64 // ball: center over the board; z < 0 when falling
	vx, vy, vz float64
	rot        Mat3 // ball orientation, for its stripes
	state      int
	since      float64 // when the state began
	target     [2]float64
	began      float64
	best       float64
	falls      int
	pending    byte // key to act on at the next frame ('n', 'r')
	up         Vec3 // smoothed room up, board frame
	hasUp      bool
	beep       bool
	lastBeep   float64
	out        interface{ Write([]byte) (int, error) }
}

type box struct{ x0, y0, x1, y1 float64 }

type mcell struct {
	open   [4]bool // right, left, down, up
	bridge bool
	hole   bool
}

const (
	wallT   = 0.2  // wall thickness
	wallH   = 0.3  // wall height
	ballR   = 0.27 // ball radius
	holeR   = 0.31
	goalR   = 0.33
	tiltK   = 1.2 // cells/s^2 per m/s^2 of downhill gravity
	maxAmp  = 0.7 // cap on the parallax shift per unit of height
	parAmp  = 1.4 // parallax exaggeration: real would be 1
	fallDur = 1.3
	winDur  = 3.5
)

const (
	rolling = iota
	falling
	solved
)

var dirs = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func (m *maze) Setup(ss *Streams) ([]*Gauge, error) {
	m.seed = clock().UnixNano()
	st, err := ss.Subscribe("gravity", 60)
	if err != nil {
		if st, err = ss.Subscribe("accelerometer", 60); err != nil {
			return nil, err
		}
	}
	return gaugesFor(st, 2), nil
}

func (m *maze) Help() []string {
	return []string{
		"Tilt the phone to roll the ball into the glowing goal. It keeps its momentum, so brake by tilting back.",
		"Bridges are passages without walls: roll off one, or into a hole, and the ball falls and goes back to the start. Every solved board adds bridges.",
		"n  new board    r  back to the start    b  vibrate on hard bumps (Termux bell)",
		"b rings the terminal bell, which Termux turns into a short vibration; it needs vibration on in Android's Sound & vibration settings.",
	}
}

func (m *maze) Key(k byte) {
	switch k {
	case 'n', 'r':
		m.pending = k
	case 'b':
		m.beep = !m.beep
	}
}

// SetOut gives the demo the terminal, for the bell.
func (m *maze) SetOut(w interface{ Write([]byte) (int, error) }) { m.out = w }

// generate makes a board that fits a w x h view: a perfect maze (randomized
// depth-first search), the goal in the cell farthest from the start, runs of
// bridge cells along the way there, holes in some dead ends.
func (m *maze) generate(w, h int) {
	rng := rand.New(rand.NewSource(m.seed))
	m.cols = max(3, min(7, int(math.Round(float64(w)/15))))
	cw := float64(w) / float64(m.cols)
	m.rows = max(3, min(9, int(float64(h)*2/cw)))
	n := m.cols * m.rows
	m.cells = make([]mcell, n)
	idx := func(c, r int) int { return r*m.cols + c }

	visited := make([]bool, n)
	stack := []int{0}
	visited[0] = true
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		c, r := cur%m.cols, cur/m.cols
		var next []int
		for d, dv := range dirs {
			nc, nr := c+dv[0], r+dv[1]
			if nc >= 0 && nr >= 0 && nc < m.cols && nr < m.rows && !visited[idx(nc, nr)] {
				next = append(next, d)
			}
		}
		if len(next) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		d := next[rng.Intn(len(next))]
		ni := idx(c+dirs[d][0], r+dirs[d][1])
		m.cells[cur].open[d] = true
		m.cells[ni].open[d^1] = true
		visited[ni] = true
		stack = append(stack, ni)
	}

	// The goal is the farthest cell; the path to it gets the bridges.
	dist := make([]int, n)
	parent := make([]int, n)
	for i := range dist {
		dist[i] = -1
	}
	dist[0] = 0
	queue := []int{0}
	far := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if dist[cur] > dist[far] {
			far = cur
		}
		for d, dv := range dirs {
			if !m.cells[cur].open[d] {
				continue
			}
			ni := idx(cur%m.cols+dv[0], cur/m.cols+dv[1])
			if dist[ni] < 0 {
				dist[ni], parent[ni] = dist[cur]+1, cur
				queue = append(queue, ni)
			}
		}
	}
	var path []int // start to goal
	for c := far; c != 0; c = parent[c] {
		path = append([]int{c}, path...)
	}
	path = append([]int{0}, path...)
	inner := path[1 : len(path)-1] // bridges never at the start or the goal
	runs, runLen := 1+(m.level-1)/2, min(2+(m.level-1)/3, 4)
	for i := 0; i < runs && len(inner) > 0; i++ {
		s := rng.Intn(len(inner))
		for k := s; k < min(s+runLen, len(inner)); k++ {
			m.cells[inner[k]].bridge = true
		}
	}
	for i := 0; i < m.level-3; i++ { // later, bridges off the path too
		if c := rng.Intn(n); c != 0 && c != far {
			m.cells[c].bridge = true
		}
	}

	center := func(i int) [2]float64 { return [2]float64{float64(i%m.cols) + 0.5, float64(i/m.cols) + 0.5} }
	m.start, m.goal = center(0), center(far)
	m.holes = m.holes[:0]
	nh := 0
	for i := range m.cells {
		c := &m.cells[i]
		deg := 0
		for _, o := range c.open {
			if o {
				deg++
			}
		}
		if deg == 1 && i != 0 && i != far && !c.bridge && nh < m.level && rng.Intn(2) == 0 {
			c.hole = true
			m.holes = append(m.holes, center(i))
			nh++
		}
	}
	m.buildWalls()
	m.reset()
}

// buildWalls turns the cells into wall boxes: a closed side gets a wall
// unless it borders a bridge cell, where it is a drop instead.
func (m *maze) buildWalls() {
	m.walls = m.walls[:0]
	normal := func(c, r int) bool { // outside the board counts as normal
		return c < 0 || r < 0 || c >= m.cols || r >= m.rows || !m.cells[r*m.cols+c].bridge
	}
	const t = wallT / 2
	wallAt := make(map[[2]int]bool) // grid corners that walls touch
	for r := 0; r < m.rows; r++ {
		for i := 0; i <= m.cols; i++ { // vertical edge left of cell (i, r)
			closed := i == 0 || i == m.cols || !m.cells[r*m.cols+i-1].open[0]
			if closed && normal(i-1, r) && normal(i, r) {
				m.walls = append(m.walls, box{float64(i) - t, float64(r), float64(i) + t, float64(r + 1)})
				wallAt[[2]int{i, r}], wallAt[[2]int{i, r + 1}] = true, true
			}
		}
	}
	for r := 0; r <= m.rows; r++ {
		for i := 0; i < m.cols; i++ { // horizontal edge above cell (i, r)
			closed := r == 0 || r == m.rows || !m.cells[(r-1)*m.cols+i].open[2]
			if closed && normal(i, r-1) && normal(i, r) {
				m.walls = append(m.walls, box{float64(i), float64(r) - t, float64(i + 1), float64(r) + t})
				wallAt[[2]int{i, r}], wallAt[[2]int{i + 1, r}] = true, true
			}
		}
	}
	for p := range wallAt { // posts fill the corners
		x, y := float64(p[0]), float64(p[1])
		m.walls = append(m.walls, box{x - t, y - t, x + t, y + t})
	}
	m.near = make([][]int, m.cols*m.rows)
	const reach = 0.7
	for ci := range m.near {
		cx, cy := float64(ci%m.cols), float64(ci/m.cols)
		for wi, b := range m.walls {
			if b.x1 > cx-reach && b.x0 < cx+1+reach && b.y1 > cy-reach && b.y0 < cy+1+reach {
				m.near[ci] = append(m.near[ci], wi)
			}
		}
	}
}

func (m *maze) reset() {
	m.x, m.y, m.z = m.start[0], m.start[1], 0
	m.vx, m.vy, m.vz = 0, 0, 0
	m.rot = Identity()
	m.state = rolling
}

func (m *maze) cellAt(x, y float64) int {
	c := max(0, min(m.cols-1, int(math.Floor(x))))
	r := max(0, min(m.rows-1, int(math.Floor(y))))
	return r*m.cols + c
}

// floorAt reports whether the board is solid at (x, y). Every passage has
// the same width: a cell's floor stops where a wall would stand on its
// closed sides, wall or not. So without walls, the drop starts exactly
// where the wall would have been. Holes and the goal are open too.
func (m *maze) floorAt(x, y float64) bool {
	if x < 0 || y < 0 || x >= float64(m.cols) || y >= float64(m.rows) {
		return false
	}
	if math.Hypot(x-m.goal[0], y-m.goal[1]) < goalR {
		return false
	}
	c := &m.cells[m.cellAt(x, y)]
	fx, fy := x-math.Floor(x)-0.5, y-math.Floor(y)-0.5
	if c.hole && math.Hypot(fx, fy) < holeR {
		return false
	}
	h := 0.5 - wallT/2
	inX, inY := math.Abs(fx) <= h, math.Abs(fy) <= h
	switch {
	case inX && inY:
		return true
	case !inX && !inY: // corners: under posts, or open to the void
		return false
	case !inX:
		return fx > 0 && c.open[0] || fx < 0 && c.open[1]
	default:
		return fy > 0 && c.open[2] || fy < 0 && c.open[3]
	}
}

// step integrates the rolling ball for dt under a downhill acceleration
// (ax, ay) in m/s^2, and returns the hardest impact speed.
func (m *maze) step(ax, ay, dt, t float64) float64 {
	// Small fixed substeps: one step's worth of tilt stays below the bounce
	// threshold, so a ball held against a wall rests instead of chattering,
	// and holes and edges are checked often enough that a fast ball can't
	// skip over them.
	n := int(math.Ceil(dt / 0.004))
	h := dt / float64(n)
	impact := 0.0
	for i := 0; i < n && m.state == rolling; i++ {
		gx, gy := ax*tiltK, ay*tiltK
		// Near a hole the ball starts to dip over its lip and is drawn in.
		for _, c := range append(m.holes, m.goal) {
			dx, dy := c[0]-m.x, c[1]-m.y
			if d := math.Hypot(dx, dy); d > 1e-6 && d < holeR+ballR*0.5 {
				gx, gy = gx+3*dx/d, gy+3*dy/d
			}
		}
		m.vx += gx * h
		m.vy += gy * h
		// Rolling resistance and a little drag, capped speed: a heavy ball
		// that keeps its momentum.
		sp := math.Hypot(m.vx, m.vy)
		if dec := (0.15 + 0.06*sp) * h; sp <= dec {
			m.vx, m.vy = 0, 0
		} else {
			f := math.Min(sp-dec, 4.5) / sp
			m.vx, m.vy = m.vx*f, m.vy*f
		}
		m.x += m.vx * h
		m.y += m.vy * h
		impact = math.Max(impact, m.collide())
		m.roll(h)
		m.check(t)
	}
	return impact
}

// collide pushes the ball out of the walls and bounces it.
func (m *maze) collide() float64 {
	impact := 0.0
	for _, wi := range m.near[m.cellAt(m.x, m.y)] {
		b := m.walls[wi]
		cx, cy := math.Max(b.x0, math.Min(m.x, b.x1)), math.Max(b.y0, math.Min(m.y, b.y1))
		dx, dy := m.x-cx, m.y-cy
		d2 := dx*dx + dy*dy
		if d2 >= ballR*ballR {
			continue
		}
		d := math.Sqrt(d2)
		var nx, ny float64
		if d < 1e-9 { // center inside the box: out the nearest side
			l, r, u, dn := m.x-b.x0, b.x1-m.x, m.y-b.y0, b.y1-m.y
			switch math.Min(math.Min(l, r), math.Min(u, dn)) {
			case l:
				nx, d = -1, -l
			case r:
				nx, d = 1, -r
			case u:
				ny, d = -1, -u
			default:
				ny, d = 1, -dn
			}
		} else {
			nx, ny = dx/d, dy/d
		}
		m.x += nx * (ballR - d)
		m.y += ny * (ballR - d)
		if vn := m.vx*nx + m.vy*ny; vn < 0 {
			e := 0.5 // a bounce; none for gentle touches, so it rests
			if -vn < 0.25 {
				e = 0
			}
			m.vx -= (1 + e) * vn * nx
			m.vy -= (1 + e) * vn * ny
			m.vx, m.vy = m.vx*0.98, m.vy*0.98 // scrubbing against the wall
			impact = math.Max(impact, -vn)
		}
	}
	return impact
}

// roll turns the ball as it rolls without slipping: about the axis
// up x velocity, by distance / radius.
func (m *maze) roll(dt float64) {
	sp := math.Hypot(m.vx, m.vy)
	if sp < 1e-6 {
		return
	}
	// The board frame has y down, so up x v comes out as (vy, -vx) here.
	a := Vec3{m.vy / sp, -m.vx / sp, 0}
	m.rot = rotAxis(a, sp*dt/ballR).Mul(m.rot)
}

// rotAxis is the rotation by angle about the unit axis a (Rodrigues).
func rotAxis(a Vec3, angle float64) Mat3 {
	c, s := math.Cos(angle), math.Sin(angle)
	k := 1 - c
	return Mat3{
		{c + a[0]*a[0]*k, a[0]*a[1]*k - a[2]*s, a[0]*a[2]*k + a[1]*s},
		{a[1]*a[0]*k + a[2]*s, c + a[1]*a[1]*k, a[1]*a[2]*k - a[0]*s},
		{a[2]*a[0]*k - a[1]*s, a[2]*a[1]*k + a[0]*s, c + a[2]*a[2]*k},
	}
}

// check starts a win or a fall after a step.
func (m *maze) check(t float64) {
	switch {
	case math.Hypot(m.x-m.goal[0], m.y-m.goal[1]) < goalR:
		m.state, m.since, m.target = solved, t, m.goal
		if run := t - m.began; m.best == 0 || run < m.best {
			m.best = run
		}
	case !m.floorAt(m.x, m.y):
		m.state, m.since, m.target = falling, t, [2]float64{-1, -1}
		m.falls++
		for _, h := range m.holes {
			if math.Hypot(m.x-h[0], m.y-h[1]) < holeR {
				m.target = h
			}
		}
	}
}

// drop moves a falling ball: down, a little onward, and into the middle of
// a hole or the goal.
func (m *maze) drop(dt float64) {
	m.vz -= 14 * dt
	m.z += m.vz * dt
	m.x += m.vx * dt
	m.y += m.vy * dt
	f := math.Max(0, 1-3*dt)
	m.vx, m.vy = m.vx*f, m.vy*f
	if m.target[0] >= 0 {
		k := math.Min(1, dt*8)
		m.x += (m.target[0] - m.x) * k
		m.y += (m.target[1] - m.y) * k
	}
	m.roll(dt)
}

func (m *maze) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	area := v.H - 1
	dt = math.Min(dt, 0.1)
	if m.cells == nil || m.pending == 'n' {
		m.generate(v.W, area)
		m.began, m.falls = t, 0
	}
	if m.pending == 'r' {
		m.reset()
		m.began, m.falls = t, 0
	}
	m.pending = 0

	a := Vec3{0, 0, 9.81}
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 3 {
		a = toScreen(ss, r.V)
	}
	// Screen frame (y up) to board frame (y down).
	up := Vec3{a[0], -a[1], a[2]}.Norm()
	if !m.hasUp {
		m.up, m.hasUp = up, true
	}
	m.up = m.up.Add(up.Sub(m.up).Scale(math.Min(1, dt*12))).Norm()

	switch m.state {
	case rolling:
		if imp := m.step(-a[0], a[1], dt, t); imp > 1.5 && m.beep && m.out != nil && t-m.lastBeep > 0.15 {
			m.out.Write([]byte("\a"))
			m.lastBeep = t
		}
	case falling:
		m.drop(dt)
		if t-m.since > fallDur {
			m.reset()
		}
	case solved:
		m.drop(dt)
		if t-m.since > winDur {
			m.level++
			m.seed++
			m.generate(v.W, area)
			m.began, m.falls = t, 0
		}
	}

	m.render(v, area, t)

	run := t - m.began
	if m.state == solved {
		run = m.since - m.began
	}
	line := fmt.Sprintf("level %d   %s", m.level, hms(time.Duration(run*float64(time.Second))))
	if m.best > 0 {
		line += "   best " + hms(time.Duration(m.best*float64(time.Second)))
	}
	if m.falls > 0 {
		line += fmt.Sprintf("   falls %d", m.falls)
	}
	v.Text(max(0, (v.W-len(line))/2), v.H-1, line, 250)

	switch m.state {
	case solved:
		text := "SOLVED"
		s := max(1, fitScale(text, v.W-4, area/4))
		bw, bh := bannerSize(text, s)
		col := []uint8{226, 220, 214, 220}[int(t*8)%4]
		drawBanner(v, text, (v.W-bw)/2, (area-bh)/2, s, '#', col)
	case falling:
		msg := " fell! back to the start "
		v.Text((v.W-len(msg))/2, area/2, msg, 203)
	}
}

// view holds what one frame's rendering needs.
type view struct {
	dx, dy float64 // parallax: a point at height z shows at xy - z*d
	light  Vec3    // toward the light, board frame
	eye    Vec3    // toward the viewer
	bz, br float64 // ball center height and drawn radius
	fog    float64 // ball dimming while it falls
}

// Ramps, dark to bright.
var (
	mzShade    = ".:-=+*#%@"
	mzWood     = []uint8{52, 94, 94, 130, 136, 172, 179}
	mzWall     = []uint8{58, 94, 137, 180, 223, 230, 231}
	mzEdge     = []uint8{234, 52, 58, 94, 95}
	mzBall     = []uint8{17, 18, 19, 25, 26, 32, 38, 75, 117, 153, 195}
	mzStripe   = []uint8{52, 88, 124, 160, 196, 202, 209, 216, 223}
	mzGoal     = []uint8{22, 28, 34, 40, 46, 83, 120, 157}
	mzVoidStar = []uint8{234, 236, 238, 240}
)

func mzPick(ramp []uint8, i float64) uint8 {
	return ramp[int(clamp01(i)*float64(len(ramp)-1)+0.5)]
}

func shadeChar(i float64) byte {
	return mzShade[int(clamp01(i)*float64(len(mzShade)-1)+0.5)]
}

func (m *maze) render(v *View, area int, t float64) {
	s := math.Min(float64(v.W)/(float64(m.cols)+0.3), 2*float64(area)/(float64(m.rows)+0.3))
	ox := (float64(v.W) - float64(m.cols)*s) / 2
	oy := (float64(area) - float64(m.rows)*s/2) / 2

	u := m.up
	uz := math.Max(u[2], 0.3)
	var vw view
	vw.dx, vw.dy = -parAmp*u[0], -parAmp*u[1]
	if l := math.Hypot(vw.dx, vw.dy); l > maxAmp {
		vw.dx, vw.dy = vw.dx*maxAmp/l, vw.dy*maxAmp/l
	}
	// A lamp above and a little toward the top left of the board, fixed in
	// the room, with the tilt exaggerated so the shading visibly follows it.
	vw.light = Vec3{-0.35 + 1.8*u[0], -0.5 + 1.8*u[1], 1.0 * uz}.Norm()
	vw.eye = Vec3{-vw.dx, -vw.dy, 1}.Norm()
	depth := math.Max(0, -m.z)
	vw.br = ballR / (1 + depth*0.5)
	vw.bz = vw.br + m.z
	vw.fog = 1 / (1 + depth*1.2)

	parallelRows(area, func(y int) {
		for x := 0; x < v.W; x++ {
			qx := (float64(x) + 0.5 - ox) / s
			qy := (float64(y) + 0.5 - oy) / (s / 2)
			c, col := m.shade(qx, qy, &vw, t)
			v.Set(x, y, c, col)
		}
	})
}

// shade renders the point of the board under screen position (qx, qy):
// the highest thing along the view ray wins.
func (m *maze) shade(qx, qy float64, vw *view, t float64) (byte, uint8) {
	L := vw.light
	list := m.near[m.cellAt(qx, qy)]
	wz, wn, wok := m.wallHit(qx, qy, vw.dx, vw.dy, list)
	bz, bn, bok := m.ballHit(qx, qy, vw)
	if bok && (!wok || bz > wz) && bz >= 0 {
		return m.ballColor(bn, vw)
	}
	if wok {
		px, py := qx-wz*vw.dx, qy-wz*vw.dy
		i := 0.2 + 0.8*math.Max(0, wn.Dot(L))
		if wn[2] > 0.5 && m.ballShadow(px, py, wz, L) {
			i *= 0.45
		}
		c := byte('#')
		switch {
		case wn[2] > 0.5:
			c = "=+*#"[int(clamp01(i)*3.99)]
		case wn[0] != 0:
			c = '|'
		default:
			c = '='
		}
		return c, mzPick(mzWall, i)
	}
	if m.floorAt(qx, qy) {
		i := 0.25 + 0.55*L[2]
		shadow := m.ballShadow(qx, qy, 0, L)
		if !shadow {
			sx, sy := -L[0]/L[2], -L[1]/L[2]
			_, _, shadow = m.wallHit(qx, qy, sx, sy, list)
		}
		if shadow {
			i *= 0.35
		}
		// The goal's glow: rings running outward.
		if d := math.Hypot(qx-m.goal[0], qy-m.goal[1]); d < goalR+0.35 {
			w := 0.5 + 0.5*math.Cos((d-goalR)*18-t*6)
			if m.state == solved {
				w = 0.5 + 0.5*math.Cos(t*14)
			}
			return "..oO"[int(w*3.99)], mzPick(mzGoal, 0.3+0.7*w*(1-(d-goalR)/0.35))
		}
		// A dark lip around holes.
		for _, c := range m.holes {
			if d := math.Hypot(qx-c[0], qy-c[1]); d < holeR+0.1 {
				n := Vec3{(qx - c[0]) / d, (qy - c[1]) / d, 0}
				return 'o', mzPick(mzWood, 0.15+0.35*math.Max(0, -n.Dot(L))*(i/0.8))
			}
		}
		// Planks: a seam every half cell and a faint grain.
		c := byte('.')
		if g := math.Mod(qy*2+0.25*math.Sin(qx*1.7), 1); g < 0.08 {
			c = '_'
		} else if math.Sin(qx*9+3*math.Sin(qy*2.3)) > 0.93 {
			c = ','
		}
		return c, mzPick(mzWood, i)
	}
	// Void: the ball falling below the board, then darkness. Narrow drops
	// (where a wall would stand) stay solid black to read clearly; the
	// round holes, the goal and the space around the board show the deep
	// starfield.
	if bok {
		return m.ballColor(bn, vw)
	}
	inBoard := qx >= 0 && qy >= 0 && qx < float64(m.cols) && qy < float64(m.rows)
	open := !inBoard || math.Hypot(qx-m.goal[0], qy-m.goal[1]) < goalR
	for _, c := range m.holes {
		open = open || math.Hypot(qx-c[0], qy-c[1]) < holeR
	}
	if !open {
		return ' ', 0
	}
	for k, d := range []float64{2, 5} {
		sx, sy := qx+d*vw.dx, qy+d*vw.dy
		h := hash2(int(math.Floor(sx*7)), int(math.Floor(sy*14)), k)
		if h%53 == 0 {
			return '.', mzVoidStar[(h>>8)%uint32(len(mzVoidStar))]
		}
	}
	return ' ', 0
}

func hash2(x, y, k int) uint32 {
	h := uint32(x)*0x8da6b343 ^ uint32(y)*0xd8163841 ^ uint32(k)*0xcb1ab31f
	h ^= h >> 13
	h *= 0x5bd1e995
	return h ^ h>>15
}

// wallHit finds the highest point in [0, wallH] where the ray through
// (qx, qy) - z*(dx, dy) is inside a wall, and that face's normal.
func (m *maze) wallHit(qx, qy, dx, dy float64, list []int) (float64, Vec3, bool) {
	best, ok := -1.0, false
	var bn Vec3
	for _, wi := range list {
		b := m.walls[wi]
		lo, hi := 0.0, wallH
		n := Vec3{0, 0, 1}
		// Along one axis the ray is at q - z*d; it is inside [a0, a1] for z
		// between (q-a1)/d and (q-a0)/d.
		slab := func(q, d, a0, a1 float64, neg, pos Vec3) bool {
			if d == 0 {
				return q >= a0 && q <= a1
			}
			z0, z1 := (q-a0)/d, (q-a1)/d // where it crosses a0, a1
			f0, f1 := neg, pos
			if z0 > z1 {
				z0, z1, f0, f1 = z1, z0, f1, f0
			}
			// Entering from above means crossing at the upper z.
			if z1 < hi {
				hi, n = z1, f1
			}
			lo = math.Max(lo, z0)
			return true
		}
		if !slab(qx, dx, b.x0, b.x1, Vec3{-1, 0, 0}, Vec3{1, 0, 0}) ||
			!slab(qy, dy, b.y0, b.y1, Vec3{0, -1, 0}, Vec3{0, 1, 0}) || lo > hi {
			continue
		}
		if hi > best {
			best, bn, ok = hi, n, true
		}
	}
	return best, bn, ok
}

// ballHit finds the ball under screen position (qx, qy). It is drawn as a
// circle around where its center appears, shaded as seen along the view
// direction: the sheared projection that shows the walls' sides would
// stretch it into an ellipse. It returns the height of the surface point.
func (m *maze) ballHit(qx, qy float64, vw *view) (float64, Vec3, bool) {
	u := (qx - m.x - vw.bz*vw.dx) / vw.br
	w := (qy - m.y - vw.bz*vw.dy) / vw.br
	r2 := u*u + w*w
	if r2 >= 1 {
		return 0, Vec3{}, false
	}
	E := vw.eye
	X := Vec3{1, 0, 0}.Sub(E.Scale(E[0])).Norm()
	Y := E.Cross(X)
	n := X.Scale(u).Add(Y.Scale(w)).Add(E.Scale(math.Sqrt(1 - r2)))
	return vw.bz + vw.br*n[2], n, true
}

// ballShadow reports whether the ball blocks the light at (x, y, z).
func (m *maze) ballShadow(x, y, z float64, L Vec3) bool {
	if m.z < 0 {
		return false
	}
	o := Vec3{x - m.x, y - m.y, z - ballR}
	b := o.Dot(L)
	c := o.Dot(o) - ballR*ballR
	return b < 0 && b*b-c > 0
}

// ballColor shades the ball: diffuse and a sharp highlight from the lamp, a
// hint of the bright room reflected on top, and a stripe that turns as it
// rolls.
func (m *maze) ballColor(n Vec3, vw *view) (byte, uint8) {
	L, E := vw.light, vw.eye
	diff := math.Max(0, n.Dot(L))
	h := L.Add(E).Norm()
	spec := math.Pow(math.Max(0, n.Dot(h)), 50)
	if spec > 0.5 && vw.fog > 0.9 {
		return '@', 231
	}
	r := E.Scale(-1).Add(n.Scale(2 * n.Dot(E)))
	env := 0.5 + 0.5*r[2]
	i := (0.08 + 0.7*diff + 0.25*env*env + 0.6*spec) * vw.fog
	local := m.rot.T().Apply(n)
	ramp := mzBall
	if math.Abs(local[2]) < 0.3 || math.Abs(local[0]) > 0.93 {
		ramp = mzStripe
	}
	return shadeChar(0.12 + 0.88*i), mzPick(ramp, i)
}
