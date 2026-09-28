package scope

import (
	"fmt"
	"math"
	"strings"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "scope",
		Desc: "oscilloscope for any sensors: every value as a scrolling trace",
		New: func(specs []string) Demo {
			if len(specs) == 0 {
				specs = []string{"accelerometer", "gyroscope", "magnetic_field"}
			}
			return &scope{specs: specs}
		},
	})
}

// scope is the fallback demo: it works for any sensor, one panel per sensor,
// each value a trace. Traces differ by character as well as color.
type scope struct {
	specs  []string
	paused bool
	frozen map[string][][]float64
}

var (
	traceChar  = []byte("*o+#%&@x")
	traceColor = []uint8{203, 114, 75, 220, 177, 51, 213, 250}
)

func (s *scope) Setup(ss *Streams) ([]*Gauge, error) {
	var gauges []*Gauge
	for _, sp := range s.specs {
		info, _ := ss.Lookup(sp)
		hz := 50.0 // 512 samples of history is ~10 s at 50 Hz
		if info.MaxHz > 0 && info.MaxHz < hz {
			hz = 0
		}
		if info.Mode != "continuous" {
			hz = 0
		}
		st, err := ss.Subscribe(sp, hz)
		if err != nil {
			return nil, err
		}
		gauges = append(gauges, GaugesFor(st, 0)...)
	}
	return gauges, nil
}

func (s *scope) Help() []string {
	return []string{
		"One panel per sensor; each value is a trace,",
		"auto-scaled to what is on screen. Newest at the right.",
		"",
		"space  pause / resume",
	}
}

func (s *scope) Key(k byte) {
	if k == ' ' {
		s.paused = !s.paused
		s.frozen = nil
	}
}

func (s *scope) Draw(v *View, ss *Streams, t, dt float64) {
	n := len(s.specs)
	if n == 0 || v.H < 3 {
		return
	}
	if s.paused && s.frozen == nil {
		s.frozen = map[string][][]float64{}
	}
	ph := v.H / n
	for i, sp := range s.specs {
		st := ss.Get(sp)
		if st == nil {
			continue
		}
		y0 := i * ph
		h := ph
		if i == n-1 {
			h = v.H - y0
		}
		s.panel(v, st, y0, h)
	}
}

func (s *scope) panel(v *View, st *Stream, y0, h int) {
	const labelW = 9
	w := v.W - labelW
	if w < 4 || h < 3 {
		return
	}
	r := st.Read()
	nv := len(r.V)
	if a, ok := Axes[st.Info.Type]; ok && len(a.Labels) < nv {
		nv = len(a.Labels)
	}
	var traces [][]float64
	if s.paused && s.frozen[st.Spec] != nil {
		traces = s.frozen[st.Spec]
	} else {
		for i := 0; i < nv; i++ {
			traces = append(traces, st.History(i, w))
		}
		if s.paused {
			s.frozen[st.Spec] = traces
		}
	}

	// Title: sensor, rate, current values.
	var b strings.Builder
	fmt.Fprintf(&b, " %s (%s) %s ", st.Sensor, st.Info.Type, RateText(st, r))
	v.Text(0, y0, b.String(), 252)
	x := b.Len()
	for i := 0; i < nv && i < len(r.V); i++ {
		label, _, _ := AxisFor(st.Info.Type, i)
		item := fmt.Sprintf(" %c %s=%.3g", traceChar[i%len(traceChar)], label, r.V[i])
		v.Text(x, y0, item, traceColor[i%len(traceColor)])
		x += len(item)
	}
	if s.paused {
		v.Text(v.W-9, y0, " PAUSED ", 214)
	}

	// Vertical range over what is visible.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, tr := range traces {
		for _, val := range tr {
			lo, hi = math.Min(lo, val), math.Max(hi, val)
		}
	}
	if math.IsInf(lo, 0) {
		v.Text(labelW, y0+h/2, "waiting for data...", 244)
		return
	}
	if hi-lo < 1e-6 {
		pad := math.Max(math.Abs(hi)*0.1, 0.5)
		lo, hi = lo-pad, hi+pad
	} else {
		pad := (hi - lo) * 0.08
		lo, hi = lo-pad, hi+pad
	}
	top, bot := y0+1, y0+h-1 // plot rows, inclusive
	rows := bot - top
	rowOf := func(val float64) int {
		return top + int(math.Round((hi-val)/(hi-lo)*float64(rows)))
	}

	v.Text(0, top, fmt.Sprintf("%8.3g", hi), 244)
	v.Text(0, bot, fmt.Sprintf("%8.3g", lo), 244)
	for yy := top; yy <= bot; yy++ {
		v.Set(labelW-1, yy, '|', 238)
	}
	if lo < 0 && hi > 0 {
		zr := rowOf(0)
		for xx := labelW; xx < v.W; xx++ {
			v.Set(xx, zr, '.', 238)
		}
	}
	for i, tr := range traces {
		c, col := traceChar[i%len(traceChar)], traceColor[i%len(traceColor)]
		off := w - len(tr) // right-align: newest at the right edge
		prev := -1
		for k, val := range tr {
			row := rowOf(val)
			xx := labelW + off + k
			if prev >= 0 { // join steep steps into a continuous line
				for yy := min(prev, row) + 1; yy < max(prev, row); yy++ {
					v.Set(xx, yy, c, col)
				}
			}
			v.Set(xx, row, c, col)
			prev = row
		}
	}
}
