package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Scale maps a reading onto [0, 1] along a gauge bar.
type Scale interface {
	Pos(v float64) float64
}

// Symmetric is centered on zero. With Fixed, values beyond ±Max pin to the
// ends; otherwise the range grows to fit a spike and then shrinks back to
// Max over a few seconds, so one hard shake doesn't flatten the bar for good.
type Symmetric struct {
	Max   float64
	Fixed bool
	cur   float64 // current range when not fixed
}

func (s *Symmetric) Pos(v float64) float64 {
	r := s.Max
	if !s.Fixed {
		if s.cur < s.Max {
			s.cur = s.Max
		}
		if a := math.Abs(v); a > s.cur {
			s.cur = a * 1.1
		}
		s.cur = math.Max(s.Max, s.cur*0.99) // called once per frame: ~4 s to settle
		r = s.cur
	}
	return 0.5 + v/(2*r)
}

// Linear runs from Min to Max; Max grows to fit what it has seen.
type Linear struct{ Min, Max float64 }

func (s *Linear) Pos(v float64) float64 {
	if v > s.Max {
		s.Max = v
	}
	return (v - s.Min) / (s.Max - s.Min)
}

// Relative starts at the first value seen and grows upward, for counters
// such as the step counter.
type Relative struct {
	Min, Max float64
	set      bool
}

func (s *Relative) Pos(v float64) float64 {
	if !s.set {
		s.Min, s.Max, s.set = v, v+10, true
	}
	if v > s.Max {
		s.Max = v + (v-s.Min)*0.25
	}
	return (v - s.Min) / (s.Max - s.Min)
}

// Asymptotic has no upper limit: v/(v+K) puts K at the middle of the bar, is
// most sensitive around typical values, and approaches the end without ever
// running off it. Meant for light.
type Asymptotic struct{ K float64 }

func (s *Asymptotic) Pos(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return v / (v + s.K)
}

// Gauge is one line of the data strip: a value of a stream on a bar.
type Gauge struct {
	Spec   string // stream key
	Index  int    // which value of the reading
	Label  string
	Unit   string
	Scale  Scale
	Pulse  bool // event sensor: the bar flashes full on each event, then fades
	Binary bool // two-state sensor (proximity here): shown as on/off
}

// DrawHUD draws the title line and one line per gauge, and returns how many
// rows it used.
// fps is the frame rate actually drawn (0 = unknown), shown in the title; the
// rates on the gauge lines are how often each sensor delivers data.
func DrawHUD(v *View, title, palName string, fps float64, gauges []*Gauge, ss *Streams) int {
	head := " sensordemo - " + title
	if fps > 0 {
		head += fmt.Sprintf("  %.0f fps", fps)
	}
	keys := "q quit  c " + palName + "  ? help "
	v.Text(0, 0, head, 250)
	if len(head)+len(keys) < v.W {
		v.Text(v.W-len(keys), 0, keys, 244)
	}
	row := 1
	lastSpec := ""
	for _, g := range gauges {
		if row >= v.H/3 { // never let the strip eat the demo
			break
		}
		s := ss.Get(g.Spec)
		if s == nil {
			continue
		}
		r := s.Read()
		hz := ""
		if g.Spec != lastSpec {
			hz = rateText(s, r)
			lastSpec = g.Spec
		}
		if g.Pulse {
			drawPulse(v, row, g, r)
		} else if g.Binary {
			drawBinary(v, row, g, r)
		} else {
			drawGauge(v, row, g, r, hz)
		}
		row++
	}
	for x := 0; x < v.W; x++ {
		v.Set(x, row, '-', 238)
	}
	return row + 1
}

func rateText(s *Stream, r Reading) string {
	switch {
	case !r.OK:
		return "waiting"
	case s.Info.Mode == "continuous":
		return fmt.Sprintf("%.0fHz", r.Hz)
	default:
		return fmt.Sprintf("%d ev", r.Count)
	}
}

// drawBinary shows a two-state sensor as a switch: a solid NEAR bar, or an
// empty "far". This phone's proximity sensor reports only 0 or its maximum.
func drawBinary(v *View, row int, g *Gauge, r Reading) {
	num := "      --"
	state := ""
	on := false
	if r.OK && g.Index < len(r.V) {
		num = fmt.Sprintf("%8.1f", r.V[g.Index])
		on = r.V[g.Index] < 1
		state = "far"
		if on {
			state = "NEAR"
		}
	}
	left := fmt.Sprintf(" %-10.10s%s %-5.5s ", g.Label, num, g.Unit)
	barW := v.W - len(left) - 11
	v.Text(0, row, left, 250)
	if barW < 6 {
		return
	}
	x0 := len(left)
	v.Set(x0, row, '[', 240)
	v.Set(x0+barW+1, row, ']', 240)
	fill, col := byte('-'), uint8(238)
	if on {
		fill, col = '#', 203
	}
	for i := 0; i < barW; i++ {
		v.Set(x0+1+i, row, fill, col)
	}
	if state != "" {
		label := " " + state + " "
		v.Text(x0+1+(barW-len(label))/2, row, label, map[bool]uint8{true: 231, false: 244}[on])
	}
}

// drawPulse shows an event sensor as a heartbeat: each event fills the bar,
// which then drains over about half a second, next to the event count.
func drawPulse(v *View, row int, g *Gauge, r Reading) {
	num := "      --"
	if r.OK {
		num = fmt.Sprintf("%8d", r.Count)
	}
	left := fmt.Sprintf(" %-10.10s%s %-5.5s ", g.Label, num, g.Unit)
	barW := v.W - len(left) - 11
	v.Text(0, row, left, 250)
	if barW < 4 {
		return
	}
	x0 := len(left)
	v.Set(x0, row, '[', 240)
	v.Set(x0+barW+1, row, ']', 240)
	v.Text(x0+1, row, strings.Repeat("-", barW), 238)
	if !r.OK {
		return
	}
	level := math.Exp(-time.Since(r.Arrived).Seconds() / 0.18)
	n := int(level*float64(barW) + 0.5)
	for i := 0; i < n; i++ {
		v.Set(x0+1+i, row, '#', 196)
	}
}

func drawGauge(v *View, row int, g *Gauge, r Reading, hz string) {
	val := math.NaN()
	if r.OK && g.Index < len(r.V) {
		val = r.V[g.Index]
	}
	num := "      --"
	if !math.IsNaN(val) {
		num = fmt.Sprintf("%8.3f", val)
		if math.Abs(val) >= 1000 {
			num = fmt.Sprintf("%8.0f", val)
		}
	}
	left := fmt.Sprintf(" %-10.10s%s %-5.5s ", g.Label, num, g.Unit)
	right := fmt.Sprintf(" %7s ", hz)
	barW := v.W - len(left) - len(right) - 2
	v.Text(0, row, left, 250)
	v.Text(v.W-len(right), row, right, 244)
	if barW < 4 {
		return
	}
	x0 := len(left)
	v.Set(x0, row, '[', 240)
	v.Set(x0+barW+1, row, ']', 240)
	v.Text(x0+1, row, strings.Repeat("-", barW), 238)
	if _, sym := g.Scale.(*Symmetric); sym {
		v.Set(x0+1+barW/2, row, '|', 240)
	}
	if math.IsNaN(val) {
		return
	}
	p := g.Scale.Pos(val)
	mark := byte('O')
	switch {
	case p < 0:
		p, mark = 0, '<' // beyond the bar's range
	case p > 1:
		p, mark = 1, '>'
	}
	v.Set(x0+1+int(p*float64(barW-1)+0.5), row, mark, 214)
}
