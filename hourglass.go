package main

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
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

// hourglass is a falling-sand simulation in a glass drawn to fill the
// screen. Each character cell holds a 2x4 grid of square sand cells, so
// grains settle at natural angles and the sand is drawn with partial-fill
// characters. Grains fall along the phone's actual down direction, not a
// snapped one: a grain slips toward a free neighbor with a probability that
// A grain that can't fall slips to any free cell within four whose way is
// clear, if it lies within about 55 degrees of straight down (each try
// draws its own limit between 49 and 61, as real grains differ). That sets the
// slope sand rests at to about 35 degrees, like real sand, and with so many
// directions available a slight tilt already starts a slight avalanche. The glass is a smooth curve, steep
// enough near the neck that sand never lodges on it. The neck lets grains
// through at a metered rate, so a full bulb empties in the chosen time
// (default one minute).
//
// Resting grains sleep; a grain wakes when a neighboring cell empties, or
// all do when the tilt changes, so the work follows the moving sand.
type hourglass struct {
	dur time.Duration

	w, h       int     // view, characters (without the timer row)
	fw, fh     int     // sand grid, cells
	cxF        float64 // glass axis, cells
	nr         int     // the neck's row, cells
	top, bot   int     // glass rows, cells
	halfH      float64 // glass half height, cells
	wmax, neck float64 // widest and neck half widths, cells
	halfW      []float64
	cell       []int32 // grain index, or empty / wall
	gx, gy     []int16 // grain positions
	tone       []uint8
	awake      []bool
	active     []int32
	order      []hgKey    // scratch for step
	slips      []hgSlip   // the moves a grain may slip by, best first
	down       [2]float64 // the current down direction
	total      int
	upper      int     // grains above the neck row
	budget     float64 // grains allowed through the neck, accumulated
	rng        *rand.Rand

	grav   [2]float64 // smoothed gravity on screen, m/s^2 (y down)
	wakeU  [2]float64 // down direction when all grains were last woken
	done   float64    // when the sand ran out (for the flash), 0 = not yet
	refill bool
}

const (
	hgSX, hgSY = 2, 4 // sand cells per character
	hgEmpty    = -1
	hgWall     = -2
	hgSteps    = 4 // sand moves per frame
	hgReach    = 4 // cells a grain can slip in one move
)

var hgDirs = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}

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
	g.grav = [2]float64{0, 9.8}
	return gaugesFor(st, 2), nil
}

func (g *hourglass) Help() []string {
	return []string{
		"Flip the phone to turn the hourglass over and start the timer; tilt it and the sand slides and piles up to one side, a little more the more you tilt.",
		fmt.Sprintf("The neck is metered so a full bulb empties in %s. Start with a duration: sensordemo hourglass 5m", g.dur),
		"r  fill the top bulb again",
	}
}

func (g *hourglass) Key(k byte) {
	if k == 'r' {
		g.refill = true
	}
}

// layout shapes the glass for the view and fills the upper bulb.
func (g *hourglass) layout(w, h int) {
	g.w, g.h = w, h
	g.fw, g.fh = w*hgSX, h*hgSY
	g.cxF = float64(w/2*hgSX) + hgSX/2.0 // the middle of a character column
	g.top, g.bot = 2*hgSY, (h-2)*hgSY-1  // inside the wooden plates
	g.nr = (g.top + g.bot) / 2
	g.halfH = float64(g.bot-g.top) / 2
	g.neck = 1.01 // two cells: one character, widened below if needed
	// Sand here holds a slope of up to 45 degrees, so the walls must be
	// clearly steeper for none to stay behind: at least 60 degrees. The
	// profile's slope peaks at 1.25, next to the neck.
	g.wmax = math.Min(float64(g.fw)/2-5*hgSX, g.halfH*math.Tan(math.Pi/6)/1.25+g.neck)
	g.shape()
	// A neck wide enough to carry the flow the timer needs: each of its
	// cells passes at most a grain per move, and some slack for the sand
	// arriving unevenly. A fast timer on a big screen gets a wide neck, as
	// real ones have.
	vol := 0
	for y := g.top; y < g.nr; y++ {
		vol += int(2 * g.halfW[y])
	}
	perMove := float64(vol) * 0.6 / g.dur.Seconds() / (30 * hgSteps)
	if cells := math.Ceil(perMove * 4 / 2); cells > 1 {
		g.neck = cells + 0.01
		g.wmax = math.Min(float64(g.fw)/2-5*hgSX, g.halfH*math.Tan(math.Pi/6)/1.25+g.neck)
		g.shape()
	}
	g.cell = make([]int32, g.fw*g.fh)
	for y := 0; y < g.fh; y++ {
		for x := 0; x < g.fw; x++ {
			c := int32(hgWall)
			if g.inside(x, y) {
				c = hgEmpty
			}
			g.cell[y*g.fw+x] = c
		}
	}
	g.fill(g.grav[1] >= 0)
}

// shape computes the glass's half width per row from the neck and the
// widest half width.
func (g *hourglass) shape() {
	g.halfW = make([]float64, g.fh)
	for y := g.top; y <= g.bot; y++ {
		s := math.Abs(float64(y)+0.5-float64(g.nr)-0.5) / g.halfH
		f := 1 - math.Pow(1-math.Min(s, 1), 1.25)
		hw := g.neck + (g.wmax-g.neck)*f
		if s > 0.88 { // rounded shoulders
			k := (s - 0.88) / 0.12
			hw *= math.Sqrt(math.Max(0, 1-0.45*k*k))
		}
		g.halfW[y] = hw
	}
}

func (g *hourglass) inside(x, y int) bool {
	if y < g.top || y > g.bot || x < 0 || x >= g.fw {
		return false
	}
	return math.Abs(float64(x)+0.5-g.cxF) < g.halfW[y]
}

// fill puts the sand in the upper bulb (the top one when topUp), packed
// down against the neck, as a freshly turned hourglass.
func (g *hourglass) fill(topUp bool) {
	for i, c := range g.cell {
		if c >= 0 {
			g.cell[i] = hgEmpty
		}
	}
	// Bulb volume, to fill it about 60%.
	vol := 0
	for y := g.top; y < g.nr; y++ {
		for x := 0; x < g.fw; x++ {
			if g.inside(x, y) {
				vol++
			}
		}
	}
	n := vol * 6 / 10
	g.gx, g.gy, g.tone, g.awake = g.gx[:0], g.gy[:0], g.tone[:0], g.awake[:0]
	for k := 0; len(g.gx) < n; k++ {
		y := g.nr - 1 - k // rows from the neck outward
		if !topUp {
			y = g.nr + 1 + k
		}
		if y < g.top || y > g.bot {
			break
		}
		for x := 0; x < g.fw && len(g.gx) < n; x++ {
			if g.cell[y*g.fw+x] == hgEmpty {
				g.cell[y*g.fw+x] = int32(len(g.gx))
				g.gx, g.gy = append(g.gx, int16(x)), append(g.gy, int16(y))
				g.tone = append(g.tone, uint8(g.rng.Intn(len(hgSand))))
				g.awake = append(g.awake, true)
			}
		}
	}
	g.total = len(g.gx)
	g.upper = 0
	for _, y := range g.gy {
		if int(y) < g.nr {
			g.upper++
		}
	}
	g.active = g.active[:0]
	for i := range g.gx {
		g.active = append(g.active, int32(i))
	}
	g.done = 0
}

func (g *hourglass) wake(i int32) {
	if !g.awake[i] {
		g.awake[i] = true
		g.active = append(g.active, i)
	}
}

func (g *hourglass) wakeAll() {
	for i := range g.gx {
		g.wake(int32(i))
	}
}

// wakeAround wakes the grains that could move into a cell that just
// emptied: its neighbors (falls) and those a slip away from it.
func (g *hourglass) wakeAround(x, y int) {
	w := func(xx, yy int) {
		if xx >= 0 && yy >= 0 && xx < g.fw && yy < g.fh {
			if c := g.cell[yy*g.fw+xx]; c >= 0 {
				g.wake(c)
			}
		}
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			w(x+dx, y+dy)
		}
	}
	for _, m := range g.slips {
		w(x-m.dx, y-m.dy)
	}
}

// try moves grain i by (dx, dy) if the target is free, the cells on the
// way are too, and, through the neck, if the meter allows.
func (g *hourglass) try(i int32, dx, dy int) bool {
	x, y := int(g.gx[i]), int(g.gy[i])
	nx, ny := x+dx, y+dy
	if !g.free(nx, ny) {
		return false
	}
	if n := max(abs(dx), abs(dy)); n > 1 {
		// The way a rolling grain goes: over the surface, so the line is
		// sampled half a cell uphill of straight.
		for k := 1; k < n; k++ {
			px := float64(dx*k)/float64(n) - 0.49*g.down[0]
			py := float64(dy*k)/float64(n) - 0.49*g.down[1]
			if !g.free(x+int(math.Round(px)), y+int(math.Round(py))) {
				return false
			}
		}
	}
	if cross := (y < g.nr) != (ny < g.nr); cross {
		if g.budget < 1 {
			return false
		}
		g.budget--
		if ny < g.nr {
			g.upper++
		} else {
			g.upper--
		}
	}
	g.cell[y*g.fw+x] = hgEmpty
	g.cell[ny*g.fw+nx] = i
	g.gx[i], g.gy[i] = int16(nx), int16(ny)
	g.wakeAround(x, y)
	return true
}

func (g *hourglass) free(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.fw && y < g.fh && g.cell[y*g.fw+x] == hgEmpty
}

// hgSlip is a move a grain may slip by, and how downhill it is.
type hgSlip struct {
	dx, dy int
	a      float64
}

// slipMoves lists the moves within reach that lie within 61 degrees of the
// down direction u (the loosest limit a try can draw), most downhill
// first, shortest first among equals.
func slipMoves(u [2]float64, out []hgSlip) []hgSlip {
	out = out[:0]
	for dy := -hgReach; dy <= hgReach; dy++ {
		for dx := -hgReach; dx <= hgReach; dx++ {
			if (dx == 0 && dy == 0) || gcd(abs(dx), abs(dy)) != 1 {
				continue // the same direction as a shorter move
			}
			l := math.Hypot(float64(dx), float64(dy))
			if a := (float64(dx)*u[0] + float64(dy)*u[1]) / l; a > math.Cos(61*math.Pi/180) {
				out = append(out, hgSlip{dx, dy, a})
			}
		}
	}
	slices.SortFunc(out, func(p, q hgSlip) int {
		switch {
		case p.a > q.a+1e-9:
			return -1
		case p.a < q.a-1e-9:
			return 1
		}
		return max(abs(p.dx), abs(p.dy)) - max(abs(q.dx), abs(q.dy))
	})
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// move lets grain i fall or slip once under the unit down direction u.
func (g *hourglass) move(i int32, u [2]float64) bool {
	// Fall: the neighbor closest to down, picked between the two around
	// it in proportion, so the grains' drift follows any angle.
	a := math.Atan2(u[1], u[0]) / (math.Pi / 4)
	o := int(math.Floor(a))
	if g.rng.Float64() < a-float64(o) {
		o++
	}
	o = (o%8 + 8) % 8
	if g.try(i, hgDirs[o][0], hgDirs[o][1]) {
		return true
	}
	// Slip: the most downhill free move; between two equally downhill
	// (mirror images), a random one first.
	flip := g.rng.Intn(2) == 0
	limit := math.Cos((49 + 12*g.rng.Float64()) * math.Pi / 180)
	for k := 0; k < len(g.slips); k++ {
		c := g.slips[k]
		if c.a < limit {
			break
		}
		if flip && k+1 < len(g.slips) && math.Abs(g.slips[k+1].a-c.a) < 1e-9 {
			if g.try(i, g.slips[k+1].dx, g.slips[k+1].dy) {
				return true
			}
			if g.try(i, c.dx, c.dy) {
				return true
			}
			k++
			continue
		}
		if g.try(i, c.dx, c.dy) {
			return true
		}
	}
	return false
}

// step moves every awake grain once, the ones farthest downhill first;
// grains that can't move go to sleep.
func (g *hourglass) step(u [2]float64) {
	g.down = u
	g.slips = slipMoves(u, g.slips)
	g.order = g.order[:0]
	for _, i := range g.active {
		g.order = append(g.order, hgKey{float32(float64(g.gx[i])*u[0] + float64(g.gy[i])*u[1]), i})
		g.awake[i] = false
	}
	slices.SortFunc(g.order, func(a, b hgKey) int {
		switch {
		case a.k > b.k:
			return -1
		case a.k < b.k:
			return 1
		}
		return 0
	})
	g.active = g.active[:0]
	for _, o := range g.order {
		if g.move(o.i, u) {
			g.wake(o.i)
		}
	}
}

// hgKey orders grains by how far downhill they are.
type hgKey struct {
	k float32
	i int32
}

var (
	hgSand  = []uint8{136, 172, 178, 179, 180, 214, 220, 221}
	hgGlass = uint8(152)
	hgRim   = uint8(67)
	hgShine = uint8(195)
	hgWood  = []uint8{52, 94, 130, 136, 172, 179}
)

func (g *hourglass) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 14 {
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
		// The sensors read the reaction to gravity: down on screen is (-x, +y).
		g.grav[0] += (-a[0] - g.grav[0]) * math.Min(1, dt*8)
		g.grav[1] += (a[1] - g.grav[1]) * math.Min(1, dt*8)
	}
	if g.refill {
		g.refill = false
		g.fill(g.grav[1] >= 0)
	}
	mag := math.Hypot(g.grav[0], g.grav[1])
	flowing := mag > 2.5 // lying flat, nothing flows
	var u [2]float64
	if flowing {
		u = [2]float64{g.grav[0] / mag, g.grav[1] / mag}
		// A new tilt can start any grain sliding.
		if math.Abs(math.Atan2(u[1], u[0])-math.Atan2(g.wakeU[1], g.wakeU[0])) > 0.01 {
			g.wakeAll()
			g.wakeU = u
		}
		// Meter the neck: a full bulb passes in g.dur, however big the
		// screen. Grains waiting at the neck wake when it lets more through.
		before := g.budget
		rate := float64(g.total) / g.dur.Seconds() * dt // grains this frame
		g.budget = math.Min(g.budget+rate, math.Max(4, 2*rate))
		if math.Floor(g.budget) > math.Floor(before) {
			for x := int(g.cxF - 3); x <= int(g.cxF+3); x++ {
				for y := g.nr - 2; y <= g.nr+1; y++ {
					g.wakeAround(x, y)
				}
			}
		}
		for s := 0; s < hgSteps && len(g.active) > 0; s++ {
			g.step(u)
		}
	}

	src := g.upper // the bulb gravity drains
	if u[1] < 0 {
		src = g.total - g.upper
	}
	if src == 0 && g.done == 0 && flowing {
		g.done = t
	} else if src > 0 {
		g.done = 0
	}

	g.drawFrame(v)
	g.drawGlass(v)
	g.drawSand(v)

	left := time.Duration(float64(g.dur) * float64(src) / math.Max(1, float64(g.total))).Round(time.Second)
	line := fmt.Sprintf("%s left of %s", clock(left), clock(g.dur))
	col := uint8(250)
	switch {
	case g.done > 0:
		line = "time is up - flip to start again"
		if int((t-g.done)*2)%2 == 0 {
			col = 214
		}
	case !flowing:
		line += "  (paused: phone is flat)"
		col = 244
	}
	v.Text(max(0, (v.W-len(line))/2), v.H-1, line, col)
}

// drawSand draws each character from the grains in its 2x4 cells: how many,
// and whether they sit high or low in it, pick the glyph; their tones the
// color.
func (g *hourglass) drawSand(v *View) {
	for cy := g.top / hgSY; cy <= g.bot/hgSY; cy++ {
		hw := 0.0 // the widest of the character's cell rows
		for sy := 0; sy < hgSY; sy++ {
			hw = math.Max(hw, g.halfW[cy*hgSY+sy])
		}
		x0 := max(0, int((g.cxF-hw)/hgSX))
		x1 := min(g.w-1, int((g.cxF+hw)/hgSX))
		for cx := x0; cx <= x1; cx++ {
			n, tone, rows := 0, 0, 0
			for sy := 0; sy < hgSY; sy++ {
				y := cy*hgSY + sy
				for sx := 0; sx < hgSX; sx++ {
					if c := g.cell[y*g.fw+cx*hgSX+sx]; c >= 0 {
						n++
						tone += int(g.tone[c])
						rows += sy
					}
				}
			}
			if n == 0 {
				continue
			}
			mid := float64(rows) / float64(n) // 0 top .. 3 bottom
			var ch byte
			switch {
			case n <= 2:
				ch = ':'
				if mid < 1.2 {
					ch = '\''
				} else if mid > 1.8 {
					ch = '.'
				}
			case n <= 4:
				ch = ':'
				if mid < 1.1 {
					ch = '"'
				} else if mid > 1.9 {
					ch = 'o'
				}
			case n == 5:
				ch = '*'
			default:
				ch = '%'
			}
			v.Set(cx, cy, ch, hgSand[tone/n])
		}
	}
}

// drawGlass draws the glass as two lines (its thickness) following the
// curve, and a streak of reflected light in each bulb.
func (g *hourglass) drawGlass(v *View) {
	col := func(fx float64) int { return int(math.Floor(fx / hgSX)) }
	for cy := g.top / hgSY; cy <= g.bot/hgSY; cy++ {
		y0, y1 := cy*hgSY, cy*hgSY+hgSY-1
		w0, w1 := g.halfW[y0], g.halfW[y1]
		for side := -1; side <= 1; side += 2 {
			// The column just outside the sand at the row's top and bottom.
			sd := float64(side)
			a := col(g.cxF + sd*(w0+0.5))
			b := col(g.cxF + sd*(w1+0.5))
			ch := byte('|')
			switch {
			case b < a:
				ch = '/'
			case b > a:
				ch = '\\'
			}
			lo, hi := min(a, b), max(a, b)
			for x := lo; x <= hi; x++ {
				v.Set(x, cy, ch, hgGlass)
				v.Set(x+side, cy, ch, hgRim)
			}
		}
		// Reflections: a streak down the left side of each bulb.
		// A short vertical shine where the bulb is widest and its wall
		// nearly upright, so it reads as glass, not as falling sand.
		s := math.Abs(float64(y0)-float64(g.nr)) / g.halfH
		if s > 0.6 && s < 0.84 {
			x := col(g.cxF - g.halfW[y0] + 2.5*hgSX)
			v.Set(x, cy, '|', hgShine)
		}
	}
	// The neck's collar.
	ny := g.nr / hgSY
	cx := int(g.cxF / hgSX)
	v.Set(cx-1, ny, ')', hgGlass)
	v.Set(cx+1, ny, '(', hgGlass)
}

// drawFrame draws the wooden plates and the turned posts.
func (g *hourglass) drawFrame(v *View) {
	half := int(g.wmax/hgSX) + 5
	cx := int(g.cxF / hgSX) // the glass's axis column
	l, r := cx-half, cx+half
	topY, botY := g.top/hgSY-2, g.bot/hgSY+1
	for x := l; x <= r; x++ {
		v.Set(x, topY, '_', hgWood[3])
		v.Set(x, topY+1, '=', hgWood[4])
		v.Set(x, botY, '=', hgWood[4])
		v.Set(x, botY+1, '"', hgWood[2])
	}
	v.Set(l-1, topY+1, '(', hgWood[1])
	v.Set(r+1, topY+1, ')', hgWood[1])
	v.Set(l-1, botY, '(', hgWood[1])
	v.Set(r+1, botY, ')', hgWood[1])
	for _, px := range []int{l + 1, r - 3} { // symmetric about cx
		n := botY - topY - 2
		for k := 0; k < n; k++ {
			y := topY + 2 + k
			t := (float64(k) + 0.5) / float64(n)
			seg := " | "
			switch {
			case k == 0 || k == n-1:
				seg = "(O)"
			case math.Abs(t-0.5) < 0.5/float64(n)+0.01:
				seg = ")O("
			case math.Abs(t-0.25) < 0.5/float64(n) || math.Abs(t-0.75) < 0.5/float64(n):
				seg = "{=}"
			}
			for i := 0; i < 3; i++ {
				if seg[i] != ' ' {
					v.Set(px+i, y, seg[i], []uint8{hgWood[1], hgWood[5], hgWood[2]}[i])
				}
			}
		}
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
