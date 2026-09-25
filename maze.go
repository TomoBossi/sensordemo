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
		desc: "a marble maze: tilt the phone to roll the ball to the flag, avoiding the holes",
		uses: []string{"gravity", "accelerometer"},
		new:  func(specs []string) Demo { return &maze{} },
	})
}

// maze is a tilt game: a random maze (recursive backtracker) drawn in the
// classic +--+ style, a marble that rolls with the phone's tilt (friction,
// bounces off walls), holes in some dead ends that send you back to the
// start, and a flag in the far corner. Each solve shows your time.
type maze struct {
	w, h       int    // screen cells
	cols, rows int    // maze cells
	wall       []bool // per screen cell: solid
	holes      [][2]float64
	goal       [2]float64
	start      [2]float64
	x, y       float64 // ball, screen cells (y in rows)
	vx, vy     float64 // cells per second (y in rows per second)
	trail      [][2]int
	began      float64 // when this run started
	best       float64
	won, fell  float64 // times of the last win / fall, for animations
	rng        *rand.Rand
	seed       int64
}

const (
	cellW = 4 // screen columns per maze cell
	cellH = 2 // screen rows per maze cell
)

func (m *maze) Setup(ss *Streams) ([]*Gauge, error) {
	m.seed = time.Now().UnixNano()
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
		"Tilt the phone to roll the marble to the flag (*) in the far corner. The holes (@) send you back to the start.",
		"n  new maze    r  back to the start",
	}
}

func (m *maze) Key(k byte) {
	switch k {
	case 'n':
		m.seed++
		m.w = 0
	case 'r':
		m.reset(0)
	}
}

// generate carves a maze with a randomized depth-first search, then puts
// holes in some dead ends.
func (m *maze) generate(w, h int) {
	m.w, m.h = w, h
	m.rng = rand.New(rand.NewSource(m.seed))
	m.cols, m.rows = max(2, (w-1)/cellW), max(2, (h-1)/cellH)
	m.wall = make([]bool, w*h)
	// Start with every wall standing: the +--+ grid.
	for r := 0; r <= m.rows; r++ {
		for c := 0; c <= m.cols*cellW; c++ {
			m.setWall(c, r*cellH, true)
		}
	}
	for c := 0; c <= m.cols; c++ {
		for r := 0; r <= m.rows*cellH; r++ {
			m.setWall(c*cellW, r, true)
		}
	}
	visited := make([]bool, m.cols*m.rows)
	degree := make([]int, m.cols*m.rows)
	stack := [][2]int{{0, 0}}
	visited[0] = true
	for len(stack) > 0 {
		c, r := stack[len(stack)-1][0], stack[len(stack)-1][1]
		var next [][3]int
		for _, d := range [][3]int{{1, 0, 0}, {-1, 0, 1}, {0, 1, 2}, {0, -1, 3}} {
			nc, nr := c+d[0], r+d[1]
			if nc >= 0 && nr >= 0 && nc < m.cols && nr < m.rows && !visited[nr*m.cols+nc] {
				next = append(next, [3]int{nc, nr, d[2]})
			}
		}
		if len(next) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		n := next[m.rng.Intn(len(next))]
		// knock down the wall between (c, r) and n
		switch n[2] {
		case 0:
			m.clearSpan((c+1)*cellW, r*cellH+1, 1, cellH-1)
		case 1:
			m.clearSpan(c*cellW, r*cellH+1, 1, cellH-1)
		case 2:
			m.clearSpan(c*cellW+1, (r+1)*cellH, cellW-1, 1)
		case 3:
			m.clearSpan(c*cellW+1, r*cellH, cellW-1, 1)
		}
		degree[r*m.cols+c]++
		degree[n[1]*m.cols+n[0]]++
		visited[n[1]*m.cols+n[0]] = true
		stack = append(stack, [2]int{n[0], n[1]})
	}
	center := func(c, r int) [2]float64 {
		return [2]float64{float64(c*cellW) + float64(cellW)/2, float64(r*cellH) + float64(cellH)/2}
	}
	m.start = center(0, 0)
	m.goal = center(m.cols-1, m.rows-1)
	m.holes = m.holes[:0]
	for r := 0; r < m.rows; r++ {
		for c := 0; c < m.cols; c++ {
			if degree[r*m.cols+c] == 1 && (c+r) > 2 && !(c == m.cols-1 && r == m.rows-1) && m.rng.Intn(3) == 0 {
				m.holes = append(m.holes, center(c, r))
			}
		}
	}
	m.reset(0)
}

func (m *maze) setWall(x, y int, on bool) {
	if x >= 0 && y >= 0 && x < m.w && y < m.h {
		m.wall[y*m.w+x] = on
	}
}

func (m *maze) clearSpan(x, y, w, h int) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			m.setWall(xx, yy, false)
		}
	}
}

func (m *maze) solid(x, y float64) bool {
	xi, yi := int(math.Floor(x)), int(math.Floor(y))
	if xi < 0 || yi < 0 || xi >= m.w || yi >= m.h {
		return true
	}
	return m.wall[yi*m.w+xi]
}

func (m *maze) reset(t float64) {
	m.x, m.y = m.start[0], m.start[1]
	m.vx, m.vy = 0, 0
	m.trail = m.trail[:0]
	m.began = t
}

func (m *maze) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 8 {
		return
	}
	area := v.H - 1
	if m.w != v.W || m.h != area {
		m.generate(v.W, area)
		m.began = t
	}
	gx, gy := 0.0, 0.0
	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 2 {
		a := toScreen(ss, r.V)
		gx, gy = -a[0], a[1] // down on screen
	}

	switch {
	case m.won > 0 && t-m.won > 3: // show the win, then a new maze
		m.won = 0
		m.seed++
		m.generate(v.W, area)
		m.began = t
	case m.fell > 0 && t-m.fell > 0.8: // after the fall animation
		m.fell = 0
		m.reset(t)
	case m.won == 0 && m.fell == 0:
		m.roll(gx, gy, dt, t)
	}

	// Walls: +--+ style from the solid grid.
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if !m.wall[y*m.w+x] {
				continue
			}
			h := x > 0 && m.wall[y*m.w+x-1] || x+1 < m.w && m.wall[y*m.w+x+1]
			vv := y > 0 && m.wall[(y-1)*m.w+x] || y+1 < m.h && m.wall[(y+1)*m.w+x]
			c := byte('+')
			switch {
			case h && !vv:
				c = '-'
			case vv && !h:
				c = '|'
			}
			if x%cellW != 0 && y%cellH == 0 {
				c = '-'
			} else if x%cellW == 0 && y%cellH != 0 {
				c = '|'
			}
			v.Set(x, y, c, 67)
		}
	}
	for _, hl := range m.holes {
		v.Set(int(hl[0])-1, int(hl[1]), '(', 237)
		v.Set(int(hl[0]), int(hl[1]), '@', 235)
		v.Set(int(hl[0])+1, int(hl[1]), ')', 237)
	}
	flag := byte('*')
	if int(t*3)%2 == 0 {
		flag = '+'
	}
	v.Set(int(m.goal[0]), int(m.goal[1]), flag, 214)

	// The marble and a short fading trail.
	for i, p := range m.trail {
		if i < len(m.trail)-1 {
			v.Set(p[0], p[1], '.', 240+uint8(i*12/len(m.trail)))
		}
	}
	ball := byte('O')
	if m.fell > 0 { // shrinking into the hole
		ball = "Oo."[min(int((t-m.fell)/0.8*3), 2)]
	}
	v.Set(int(m.x), int(m.y), ball, 231)

	// Status line.
	run := t - m.began
	line := fmt.Sprintf("time %s", clock(time.Duration(run*float64(time.Second))))
	if m.best > 0 {
		line += fmt.Sprintf("   best %s", clock(time.Duration(m.best*float64(time.Second))))
	}
	col := uint8(250)
	if m.won > 0 {
		line = fmt.Sprintf("SOLVED in %s!", clock(time.Duration((m.won-m.began)*float64(time.Second))))
		col = 214
	}
	v.Text(max(0, (v.W-len(line))/2), v.H-1, line, col)
	if m.won > 0 {
		text := "SOLVED"
		s := max(1, fitScale(text, v.W-4, area/3))
		bw, bh := bannerSize(text, s)
		drawBanner(v, text, (v.W-bw)/2, (area-bh)/2, s, '#', 214)
	}
}

// roll integrates the marble: tilt accelerates it, friction slows it, walls
// bounce it, holes swallow it, the flag wins.
func (m *maze) roll(gx, gy, dt, t float64) {
	const accel = 3.0 // cells/s^2 per m/s^2 of tilt
	const friction = 1.6
	m.vx += (gx*accel - m.vx*friction) * dt
	m.vy += (gy*accel/2 - m.vy*friction) * dt // rows are twice as tall
	steps := int(math.Ceil(math.Max(math.Abs(m.vx), math.Abs(m.vy))*dt*4)) + 1
	for i := 0; i < steps; i++ {
		nx := m.x + m.vx*dt/float64(steps)
		if m.solid(nx, m.y) {
			m.vx = -m.vx * 0.35
		} else {
			m.x = nx
		}
		ny := m.y + m.vy*dt/float64(steps)
		if m.solid(m.x, ny) {
			m.vy = -m.vy * 0.35
		} else {
			m.y = ny
		}
	}
	p := [2]int{int(m.x), int(m.y)}
	if len(m.trail) == 0 || m.trail[len(m.trail)-1] != p {
		m.trail = append(m.trail, p)
		if len(m.trail) > 12 {
			m.trail = m.trail[1:]
		}
	}
	for _, hl := range m.holes {
		if math.Hypot(m.x-hl[0], (m.y-hl[1])*2) < 0.9 {
			m.fell = t
			m.x, m.y = hl[0], hl[1]
			return
		}
	}
	if math.Hypot(m.x-m.goal[0], (m.y-m.goal[1])*2) < 1.2 {
		m.won = t
		if run := t - m.began; m.best == 0 || run < m.best {
			m.best = run
		}
	}
}
