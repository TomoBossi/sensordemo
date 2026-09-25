package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"
)

func init() {
	register(entry{
		name: "gestures",
		desc: "Moto gestures and steps burst onto the screen as big letters",
		uses: []string{"chop_chop", "flip_twist", "flip", "wake_gesture", "tilt_detector",
			"significant_motion", "significant_move", "step_detector"},
		new: func(specs []string) Demo { return &gestures{specs: specs} },
	})
}

type gesture struct {
	spec  string
	word  func(v []float64) string
	color uint8
	seen  int
	s     *Stream
}

type burst struct {
	text   string
	color  uint8
	born   float64
	sparks []spark
}

type spark struct{ x, y, vx, vy float64 }

type gestures struct {
	specs   []string
	list    []*gesture
	bursts  []*burst
	log     []string
	steps   int
	pending []string // test bursts queued by the space key
	rng     *rand.Rand
}

var allGestures = []gesture{
	{spec: "CHOP_CHOP", word: func([]float64) string { return "CHOP!" }, color: 214},
	{spec: "FLIP_TWIST", word: func([]float64) string { return "TWIST!" }, color: 213},
	{spec: "FLIP", word: func(v []float64) string {
		if len(v) > 0 && v[0] == 2 {
			return "FLIP UP"
		}
		return "FLIP DOWN"
	}, color: 81},
	{spec: "WAKE_GESTURE", word: func([]float64) string { return "WAKE" }, color: 229},
	{spec: "TILT_DETECTOR", word: func([]float64) string { return "TILT" }, color: 159},
	{spec: "SIGNIFICANT_MOTION", word: func([]float64) string { return "MOVING" }, color: 120},
	{spec: "SIGNIFICANT_MOVE", word: func([]float64) string { return "MOVE" }, color: 120},
	{spec: "step_detector", word: nil, color: 250}, // word set in Draw: STEP n
}

const burstLife = 1.6 // seconds a banner stays up

func (g *gestures) Setup(ss *Streams) ([]*Gauge, error) {
	g.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	want := map[string]bool{}
	for _, sp := range g.specs {
		if s, ok := ss.Lookup(sp); ok {
			want[strings.ToLower(s.Name)] = true
			want[s.Type] = true
		}
	}
	for i := range allGestures {
		ge := allGestures[i]
		info, ok := ss.Lookup(ge.spec)
		if !ok || (len(want) > 0 && !want[strings.ToLower(info.Name)] && !want[info.Type]) {
			continue
		}
		s, err := ss.Subscribe(ge.spec, 0)
		if err != nil {
			continue // e.g. step sensors without the activity permission
		}
		ge.s = s
		g.list = append(g.list, &ge)
	}
	if len(g.list) == 0 {
		return nil, fmt.Errorf("no gesture sensors available")
	}
	return nil, nil
}

func (g *gestures) Help() []string {
	return []string{
		"Moto gestures fire their sensors even with Moto",
		"Actions doing their own thing (flashlight, camera):",
		"",
		"chop twice        CHOP!",
		"twist wrist twice TWIST!",
		"place face down   FLIP DOWN (and up again)",
		"lift from table   WAKE / TILT",
		"walk              STEP n",
		"",
		"space  test burst",
	}
}

func (g *gestures) Key(k byte) {
	if k == ' ' {
		g.pending = append(g.pending, "TEST")
	}
}

func (g *gestures) spawn(text string, color uint8, t float64, v *View) {
	b := &burst{text: text, color: color, born: t}
	cx, cy := float64(v.W)/2, float64(v.H)*0.4
	for i := 0; i < 80; i++ {
		a := g.rng.Float64() * 2 * math.Pi
		sp := 10 + g.rng.Float64()*40
		b.sparks = append(b.sparks, spark{cx, cy, math.Cos(a) * sp, math.Sin(a) * sp * 0.5})
	}
	g.bursts = append(g.bursts, b)
	g.log = append(g.log, fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), text))
	if len(g.log) > 50 {
		g.log = g.log[1:]
	}
}

func (g *gestures) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 8 {
		return
	}
	for _, text := range g.pending {
		g.spawn(text, 250, t, v)
	}
	g.pending = nil
	for _, ge := range g.list {
		r := ge.s.Read()
		for ge.seen < r.Count {
			ge.seen++
			text := ""
			if ge.word == nil {
				g.steps++
				text = fmt.Sprintf("STEP %d", g.steps)
			} else {
				text = ge.word(r.V)
			}
			g.spawn(text, ge.color, t, v)
		}
	}

	// Sparks, under the banner.
	live := g.bursts[:0]
	for _, b := range g.bursts {
		age := t - b.born
		if age > burstLife {
			continue
		}
		live = append(live, b)
		for i := range b.sparks {
			s := &b.sparks[i]
			s.vy += 30 * dt // gravity, in rows/s^2
			s.x += s.vx * dt
			s.y += s.vy * dt
			c := byte('*')
			switch {
			case age > burstLife*0.7:
				c = '.'
			case age > burstLife*0.4:
				c = '+'
			}
			v.Set(int(s.x), int(s.y), c, b.color)
		}
	}
	g.bursts = live

	// The newest banner, fading through lighter characters as it ages.
	logH := min(6, v.H/4)
	area := v.H - logH - 2
	if n := len(g.bursts); n > 0 {
		b := g.bursts[n-1]
		age := t - b.born
		fade := "@@@@##++:."
		fill := fade[min(int(age/burstLife*float64(len(fade))), len(fade)-1)]
		s := fitScale(b.text, v.W*9/10, area*7/10)
		bw, bh := bannerSize(b.text, s)
		drawBanner(v, b.text, (v.W-bw)/2, (area-bh)/2+1, s, fill, b.color)
	} else {
		msg := "chop, twist, flip, lift or walk"
		v.Text((v.W-len(msg))/2, area/2, msg, 244)
	}

	// Counters and the event log.
	y := v.H - logH - 1
	for x := 0; x < v.W; x++ {
		v.Set(x, y, '-', 238)
	}
	var b strings.Builder
	for _, ge := range g.list {
		fmt.Fprintf(&b, " %s:%d ", strings.ToLower(ge.spec), ge.seen)
	}
	v.Text(0, y, b.String()[:min(b.Len(), v.W)], 244)
	for i := 0; i < logH && i < len(g.log); i++ {
		v.Text(1, y+1+i, g.log[len(g.log)-1-i], 250-uint8(min(i, 5))*2)
	}
}
