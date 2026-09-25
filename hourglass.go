package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

func init() {
	register(entry{
		name: "hourglass",
		desc: "a sand timer: flip the phone to start it, sand that really falls (optional duration: 5m, 90s)",
		uses: []string{"gravity"},
		new:  func(specs []string) Demo { return &hourglass{} },
	})
}

// hourglass is a falling-sand simulation, one grain per character cell,
// inside a glass drawn to fill the screen. Sand falls toward wherever gravity
// points on the screen, so flipping the phone starts it again and tilting it
// piles the sand to one side. The neck lets grains through at a metered rate,
// so the top bulb empties in the chosen time (default one minute).
type hourglass struct {
	dur time.Duration

	w, h   int
	cx, cy int     // neck position
	halfW  []int   // glass half-width per row (0 outside the glass)
	sand   []bool  // grain per cell
	total  int     // grains in the glass
	budget float64 // grains allowed through the neck, accumulated
	rng    *rand.Rand

	gx, gy float64 // smoothed gravity on screen
	done   float64 // time the sand ran out (for the flash), 0 = not yet
}

func (g *hourglass) Setup(ss *Streams) ([]*Gauge, error) {
	g.dur = time.Minute
	if demoArg != "" {
		d, err := time.ParseDuration(demoArg)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("hourglass: %q is not a duration (try 5m, 90s, 1h)", demoArg)
		}
		g.dur = d
	}
	g.rng = rand.New(rand.NewSource(3))
	st, err := ss.Subscribe("gravity", 30)
	if err != nil {
		if st, err = ss.Subscribe("accelerometer", 30); err != nil {
			return nil, err
		}
	}
	return gaugesFor(st, 2), nil
}

func (g *hourglass) Help() []string {
	return []string{
		"Flip the phone to turn the hourglass over and start the timer; tilt it and the sand piles up to one side.",
		fmt.Sprintf("The neck is metered so a full bulb empties in %s. Start with a duration: sensordemo hourglass 5m", g.dur),
		"r  fill the top bulb again",
	}
}

func (g *hourglass) Key(k byte) {
	if k == 'r' {
		g.w = 0
	}
}

// layout shapes the glass for the screen and fills the top bulb.
func (g *hourglass) layout(w, h int) {
	g.w, g.h = w, h
	g.cx, g.cy = w/2, h/2
	g.halfW = make([]int, h)
	g.sand = make([]bool, w*h)
	top, bot := 2, h-3 // inside the wooden caps
	half := float64(bot-top) / 2
	maxW := math.Min(float64(w)/2-3, half*1.6) // rows are twice as tall
	for y := top; y <= bot; y++ {
		d := math.Abs(float64(y-g.cy)) / half // 0 at the neck, 1 at the caps
		// round bulbs pinched to a one-cell neck
		shape := math.Pow(math.Sin(math.Min(d, 1)*math.Pi*0.62), 0.8) / math.Sin(math.Pi*0.62)
		g.halfW[y] = max(1, int(maxW*math.Min(shape, 1)))
	}
	g.halfW[g.cy] = 0 // the neck itself: one cell, x == cx
	// Fill the top bulb, leaving some air at the top.
	g.total = 0
	for y := top + max(1, int(half*0.18)); y < g.cy; y++ {
		for x := g.cx - g.halfW[y]; x <= g.cx+g.halfW[y]; x++ {
			g.sand[y*w+x] = true
			g.total++
		}
	}
	g.done = 0
}

func (g *hourglass) inside(x, y int) bool {
	if y < 0 || y >= g.h || x < 0 || x >= g.w {
		return false
	}
	if y == g.cy {
		return x == g.cx
	}
	return g.halfW[y] > 0 && x >= g.cx-g.halfW[y] && x <= g.cx+g.halfW[y]
}

func (g *hourglass) free(x, y int) bool { return g.inside(x, y) && !g.sand[y*g.w+x] }

// step moves every grain one cell toward gravity if it can, else slides it
// diagonally. Cells are visited far side first so grains fall in order.
func (g *hourglass) step(dx, dy int) {
	// the two diagonal alternatives: gravity turned by +-45 degrees
	ax1, ay1 := sgn(dx-dy), sgn(dy+dx)
	ax2, ay2 := sgn(dx+dy), sgn(dy-dx)
	ys, ye, ystep := 0, g.h, 1
	if dy > 0 {
		ys, ye, ystep = g.h-1, -1, -1
	}
	xs, xe, xstep := 0, g.w, 1
	if dx > 0 {
		xs, xe, xstep = g.w-1, -1, -1
	}
	for y := ys; y != ye; y += ystep {
		for x := xs; x != xe; x += xstep {
			if !g.sand[y*g.w+x] {
				continue
			}
			tryMove := func(nx, ny int) bool {
				if !g.free(nx, ny) {
					return false
				}
				// grains crossing the neck row spend the metered budget
				if (y == g.cy) != (ny == g.cy) && ny == g.cy {
					if g.budget < 1 {
						return false
					}
					g.budget--
				}
				g.sand[y*g.w+x] = false
				g.sand[ny*g.w+nx] = true
				return true
			}
			if tryMove(x+dx, y+dy) {
				continue
			}
			if g.rng.Intn(2) == 0 {
				ax1, ay1, ax2, ay2 = ax2, ay2, ax1, ay1
			}
			if !tryMove(x+ax1, y+ay1) {
				tryMove(x+ax2, y+ay2)
			}
		}
	}
}

func sgn(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

var (
	sandCols  = []uint8{136, 172, 178, 214, 220, 222, 223}
	glassCol  = uint8(116)
	woodChars = []byte("=#=")
)

func (g *hourglass) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 12 {
		return
	}
	area := v.H - 1 // bottom row: the timer
	if g.w != v.W || g.h != area {
		g.layout(v.W, area)
	}

	spec := "gravity"
	if ss.Get(spec) == nil {
		spec = "accelerometer"
	}
	if r := ss.Get(spec).Read(); r.OK && len(r.V) >= 2 {
		a := toScreen(ss, r.V)
		// the sensors read the reaction to gravity: down on screen is (-x, +y)
		g.gx += (-a[0] - g.gx) * math.Min(1, dt*8)
		g.gy += (a[1] - g.gy) * math.Min(1, dt*8)
	}
	// Gravity direction snapped to one of 8 neighbors; flat phone: no flow.
	var dx, dy int
	if math.Hypot(g.gx, g.gy) > 2.5 {
		ang := math.Atan2(g.gy, g.gx)
		oct := int(math.Round(ang/(math.Pi/4))) & 7
		dx = []int{1, 1, 0, -1, -1, -1, 0, 1}[oct]
		dy = []int{0, 1, 1, 1, 0, -1, -1, -1}[oct]
	}

	// Meter the neck: a full bulb passes in g.dur, however big the screen.
	g.budget = math.Min(g.budget+float64(g.total)/g.dur.Seconds()*dt, 6)
	if dx != 0 || dy != 0 {
		for i := 0; i < 3; i++ {
			g.step(dx, dy)
		}
	}

	// Count what is left in the bulb gravity drains from.
	upper, lower := 0, 0
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			if g.sand[y*g.w+x] {
				if y < g.cy {
					upper++
				} else if y > g.cy {
					lower++
				}
			}
		}
	}
	src := upper
	if dy < 0 {
		src = lower
	}
	if src == 0 && g.done == 0 && dy != 0 {
		g.done = t
	} else if src > 0 {
		g.done = 0
	}

	g.drawFrame(v)
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			if !g.sand[y*g.w+x] {
				continue
			}
			// shade: surface grains (air above them, against gravity) are
			// light; buried ones darker toward the bottom of the pile
			c, col := byte('%'), sandCols[2+int(3*cellNoise(x, y))]
			if g.free(x-dx, y-dy) || !g.inside(x-dx, y-dy) && g.free(x-dx, y) {
				c, col = ':', sandCols[5+int(2*cellNoise(x, y))]
			}
			if y == g.cy {
				c = '|' // the stream through the neck
			}
			v.Set(x, y, c, col)
		}
	}

	// The timer.
	left := time.Duration(float64(g.dur) * float64(src) / math.Max(1, float64(g.total))).Round(time.Second)
	line := fmt.Sprintf("%s left of %s", clock(left), clock(g.dur))
	col := uint8(250)
	switch {
	case g.done > 0:
		line = "time is up - flip to start again"
		if int((t-g.done)*2)%2 == 0 {
			col = 214
		}
	case dx == 0 && dy == 0:
		line += "  (paused: phone is flat)"
		col = 244
	}
	v.Text(max(0, (v.W-len(line))/2), v.H-1, line, col)
}

// drawFrame draws the glass outline and the wooden caps.
func (g *hourglass) drawFrame(v *View) {
	top, bot := 2, g.h-3
	for y := top; y <= bot; y++ {
		hw := g.halfW[y]
		if y == g.cy {
			v.Set(g.cx-1, y, ')', glassCol)
			v.Set(g.cx+1, y, '(', glassCol)
			continue
		}
		// the glass leans in toward the neck: connect this row's wall to
		// the next row's with a run of slashes, so the outline is unbroken
		next := g.halfW[min(y+1, bot)]
		if y+1 == g.cy {
			next = 0
		}
		lo, hi := min(hw, next), max(hw, next)
		lc, rc := byte('|'), byte('|')
		if next < hw {
			lc, rc = '\\', '/'
		} else if next > hw {
			lc, rc = '/', '\\'
		}
		prev := g.halfW[max(y-1, top)]
		if y-1 == g.cy {
			prev = 0
		}
		own := byte('|')
		switch {
		case next < hw || prev > hw:
			own = '\\'
		case next > hw || prev < hw:
			own = '/'
		}
		v.Set(g.cx-hw-1, y, own, glassCol)
		v.Set(g.cx+hw+1, y, map[byte]byte{'|': '|', '\\': '/', '/': '\\'}[own], glassCol)
		// The run goes on the narrower row, where it lies outside the
		// glass (the wider row's cells there hold sand).
		ry := y
		if next < hw {
			ry = y + 1
		}
		from, to := lo+1, hi // widening: just past this row's wall
		if next < hw {
			from, to = lo, hi-1 // narrowing: from the narrower row's own wall
		}
		for k := from; k <= to; k++ {
			v.Set(g.cx-k-1, ry, lc, glassCol)
			v.Set(g.cx+k+1, ry, rc, glassCol)
		}
		if hi == lo {
			v.Set(g.cx-hw-1, y, '|', glassCol)
			v.Set(g.cx+hw+1, y, '|', glassCol)
		}
	}
	// wooden caps and posts
	hw := 0
	for _, w := range g.halfW {
		hw = max(hw, w)
	}
	for _, y := range []int{top - 1, bot + 1} {
		for x := g.cx - hw - 3; x <= g.cx+hw+3; x++ {
			v.Set(x, y, woodChars[(x+y)%len(woodChars)], 130)
		}
	}
	for y := top; y <= bot; y++ {
		v.Set(g.cx-hw-3, y, '|', 94)
		v.Set(g.cx+hw+3, y, '|', 94)
	}
}

// clock formats a duration as m:ss, or h:mm:ss.
func clock(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
