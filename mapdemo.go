package main

import (
	"fmt"
	"math"
	"sync"
)

func init() {
	register(entry{
		name: "map",
		desc: "an OpenStreetMap of where you are, drawn in ASCII, turning with the phone",
		uses: []string{"location", "gps"},
		new:  func(specs []string) Demo { return &mapDemo{specs: specs, mpc: 4, headingUp: true} },
	})
}

// mapDemo draws OpenStreetMap ways around the current fix: buildings filled,
// streets as lines whose character follows their direction, names on long
// stretches, water and parks, and the position with its accuracy ring. By
// default the map turns so the phone's heading is up.
type mapDemo struct {
	specs     []string
	source    string
	mpc       float64 // meters per column (rows are two columns tall)
	headingUp bool
	heading   float64

	mu        sync.Mutex
	ways      []way
	fetched   latLon // center of the loaded data
	radius    int
	loading   bool
	lastErr   string
	hasCenter bool
}

var zooms = []float64{1, 1.5, 2, 3, 4, 6, 8, 12, 16, 24, 32}

func (m *mapDemo) Setup(ss *Streams) ([]*Gauge, error) {
	m.source = "location"
	for _, sp := range m.specs {
		if s, ok := ss.Lookup(sp); ok && s.Type == "gps" {
			m.source = sp
		}
	}
	st, err := ss.Subscribe(m.source, 1)
	if err != nil {
		return nil, err
	}
	ss.Subscribe("rotation_vector", 20) // heading; the map works without it
	return []*Gauge{
		{Spec: st.Spec, Index: 0, Label: "latitude", Unit: "deg", Scale: &Symmetric{Max: 90}},
		{Spec: st.Spec, Index: 1, Label: "longitude", Unit: "deg", Scale: &Symmetric{Max: 180}},
		{Spec: st.Spec, Index: 2, Label: "accuracy", Unit: "m", Scale: &Asymptotic{K: 20}},
	}, nil
}

func (m *mapDemo) Help() []string {
	return []string{
		"OpenStreetMap around your position. @ is you, the",
		"dotted ring is the fix accuracy, ^ is where you face.",
		"Map data comes from overpass-api.de (your approximate",
		"position is sent) and is cached on the phone.",
		"",
		"+ -  zoom      n  north-up / heading-up",
	}
}

func (m *mapDemo) Key(k byte) {
	i := 0
	for i < len(zooms) && zooms[i] < m.mpc {
		i++
	}
	switch k {
	case '+', '=':
		m.mpc = zooms[max(i-1, 0)]
	case '-', '_':
		m.mpc = zooms[min(i+1, len(zooms)-1)]
	case 'n':
		m.headingUp = !m.headingUp
	}
}

// ensure loads map data around c in the background when c has moved too far
// from the loaded area, or the view needs a bigger radius.
func (m *mapDemo) ensure(c latLon, need int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loading {
		return
	}
	moved := metersBetween(c, m.fetched)
	if m.ways != nil && moved < float64(m.radius)/3 && need <= m.radius {
		return
	}
	r := min(max(need*3/2, 400), 1500)
	m.loading = true
	go func() {
		ways, err := fetchWays(latLon{round4(c.Lat), round4(c.Lon)}, r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.loading = false
		if err != nil {
			m.lastErr = err.Error()
			return
		}
		m.ways, m.fetched, m.radius, m.lastErr = ways, c, r, ""
	}()
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

func metersBetween(a, b latLon) float64 {
	dy := (a.Lat - b.Lat) * 110540
	dx := (a.Lon - b.Lon) * 111320 * math.Cos(a.Lat*math.Pi/180)
	return math.Hypot(dx, dy)
}

func (m *mapDemo) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 6 {
		return
	}
	r := ss.Get(m.source).Read()
	if !r.OK || len(r.V) < 2 {
		msg := "waiting for a location fix... (outdoors is faster)"
		v.Text((v.W-len(msg))/2, v.H/2, msg, 244)
		return
	}
	here := latLon{r.V[0], r.V[1]}
	acc := 0.0
	if len(r.V) > 2 && !math.IsNaN(r.V[2]) {
		acc = r.V[2]
	}

	if rv := ss.Get("rotation_vector"); rv != nil {
		if rr := rv.Read(); rr.OK {
			if R, ok := FromRotationVector(rr.V); ok {
				h := Heading(R)
				m.heading += math.Remainder(h-m.heading, 2*math.Pi) * math.Min(1, dt*6)
			}
		}
	}
	rot := 0.0
	if m.headingUp {
		rot = m.heading
	}

	halfDiag := math.Hypot(float64(v.W)/2*m.mpc, float64(v.H)*m.mpc)
	m.ensure(here, int(halfDiag))

	// Projection: meters east/north of here, rotated so the heading is up,
	// then to cells (rows are two columns tall).
	cosLat := math.Cos(here.Lat * math.Pi / 180)
	cr, sr := math.Cos(rot), math.Sin(rot)
	cx, cy := float64(v.W)/2, float64(v.H)/2
	proj := func(p latLon) (float64, float64) {
		e := (p.Lon - here.Lon) * 111320 * cosLat
		n := (p.Lat - here.Lat) * 110540
		x := e*cr - n*sr
		y := e*sr + n*cr
		return cx + x/m.mpc, cy - y/(2*m.mpc)
	}

	m.mu.Lock()
	ways, loading, lastErr := m.ways, m.loading, m.lastErr
	m.mu.Unlock()

	// Areas first, then buildings, then lines on top.
	for _, pass := range []wayKind{kindPark, kindWater, kindBuilding} {
		for _, w := range ways {
			if w.kind != pass {
				continue
			}
			if w.closed {
				fillPolygon(v, w, proj)
			} else if w.kind == kindWater {
				drawLine(v, w, proj, '~', 39)
			}
		}
	}
	labels := newLabelMask(v)
	for _, pass := range []wayKind{kindRail, kindFoot, kindMinor, kindMajor} {
		for _, w := range ways {
			if w.kind != pass {
				continue
			}
			switch w.kind {
			case kindRail:
				drawLine(v, w, proj, '+', 136)
			case kindFoot:
				if m.mpc <= 2 { // sidewalks are often mapped too; only up close
					drawLine(v, w, proj, ':', 244)
				}
			case kindMinor:
				drawLine(v, w, proj, 0, 252)
			case kindMajor:
				drawLine(v, w, proj, 0, 214)
			}
		}
	}
	for _, w := range ways {
		if w.name != "" && (w.kind == kindMinor || w.kind == kindMajor) {
			labels.place(v, w, proj)
		}
	}

	// Accuracy ring, position, heading arrow.
	if acc > 0 {
		rr := acc / m.mpc
		for a := 0.0; a < 2*math.Pi; a += 0.5 / math.Max(rr, 1) {
			v.Set(int(cx+rr*math.Cos(a)+0.5), int(cy+rr*math.Sin(a)/2+0.5), '.', 203)
		}
	}
	v.Set(int(cx), int(cy), '@', 196)
	ha := m.heading - rot // heading relative to screen up
	ax, ay := int(cx+2.5*math.Sin(ha)+0.5), int(cy-1.25*math.Cos(ha)+0.5)
	v.Set(ax, ay, arrowChar(ha), 196)

	status := fmt.Sprintf(" %.0f m/col  %s", m.mpc, map[bool]string{true: "heading-up", false: "north-up"}[m.headingUp])
	switch {
	case loading:
		status += "  loading map..."
	case lastErr != "":
		status += "  map error: " + lastErr
	case ways == nil:
		status += "  no map data yet"
	}
	v.Text(0, v.H-1, status[:min(len(status), v.W)], 244)
	if !m.headingUp {
		v.Text(v.W-3, 0, " N", 196)
	}
}

func arrowChar(a float64) byte {
	d := math.Mod(a*180/math.Pi+360+22.5, 360)
	return "^/>\\v/<\\"[int(d/45)]
}

// lineChar picks the character that looks like a segment of this direction.
func lineChar(dx, dy float64) byte {
	a := math.Mod(math.Atan2(-dy*2, dx)*180/math.Pi+180, 180) // screen-true angle
	switch {
	case a < 22.5 || a >= 157.5:
		return '-'
	case a < 67.5:
		return '/'
	case a < 112.5:
		return '|'
	default:
		return '\\'
	}
}

func drawLine(v *View, w way, proj func(latLon) (float64, float64), c byte, col uint8) {
	x0, y0 := proj(w.pts[0])
	for _, p := range w.pts[1:] {
		x1, y1 := proj(p)
		ch := c
		if ch == 0 {
			ch = lineChar(x1-x0, y1-y0)
		}
		steps := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
		if steps < 4000 { // skip absurd segments far off screen
			for i := 0; i <= steps; i++ {
				f := float64(i) / float64(steps)
				v.Set(int(x0+(x1-x0)*f), int(y0+(y1-y0)*f), ch, col)
			}
		}
		x0, y0 = x1, y1
	}
}

func fillPolygon(v *View, w way, proj func(latLon) (float64, float64)) {
	xs := make([]float64, len(w.pts))
	ys := make([]float64, len(w.pts))
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i, p := range w.pts {
		xs[i], ys[i] = proj(p)
		minX, maxX = math.Min(minX, xs[i]), math.Max(maxX, xs[i])
		minY, maxY = math.Min(minY, ys[i]), math.Max(maxY, ys[i])
	}
	if maxX < 0 || maxY < 0 || minX >= float64(v.W) || minY >= float64(v.H) {
		return
	}
	c, col := byte('#'), uint8(240)
	switch w.kind {
	case kindPark:
		c, col = '"', 71
	case kindWater:
		c, col = '~', 39
	}
	for y := max(int(minY), 0); y <= min(int(maxY), v.H-1); y++ {
		for x := max(int(minX), 0); x <= min(int(maxX), v.W-1); x++ {
			if inPolygon(float64(x)+0.5, float64(y)+0.5, xs, ys) {
				v.Set(x, y, c, col)
			}
		}
	}
}

func inPolygon(x, y float64, xs, ys []float64) bool {
	in := false
	for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
		if (ys[i] > y) != (ys[j] > y) && x < (xs[j]-xs[i])*(y-ys[i])/(ys[j]-ys[i])+xs[i] {
			in = !in
		}
	}
	return in
}

// labelMask keeps street names from overlapping each other, and places
// each name once, on its longest roughly horizontal stretch.
type labelMask struct {
	used []bool
	w    int
	done map[string]bool
}

func newLabelMask(v *View) *labelMask {
	return &labelMask{used: make([]bool, v.W*v.H), w: v.W, done: map[string]bool{}}
}

func (l *labelMask) place(v *View, w way, proj func(latLon) (float64, float64)) {
	if l.done[w.name] {
		return
	}
	best, bx, by := 0.0, 0.0, 0.0
	for i := 0; i+1 < len(w.pts); i++ {
		x0, y0 := proj(w.pts[i])
		x1, y1 := proj(w.pts[i+1])
		if math.Abs(y1-y0)*2 > math.Abs(x1-x0)*0.4 { // only near-horizontal
			continue
		}
		if n := math.Abs(x1 - x0); n > best {
			best, bx, by = n, (x0+x1)/2, (y0+y1)/2
		}
	}
	text := " " + w.name + " "
	if best < float64(len(text)+2) {
		return
	}
	x0, y := int(bx)-len(text)/2, int(by)
	if y < 0 || y >= v.H || x0 < 0 || x0+len(text) > v.W {
		return
	}
	for i := range text {
		if l.used[y*l.w+x0+i] {
			return
		}
	}
	for i := range text {
		l.used[y*l.w+x0+i] = true
	}
	v.Text(x0, y, text, 230)
	l.done[w.name] = true
}
