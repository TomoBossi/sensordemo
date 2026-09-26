package main

import (
	"fmt"
	"math"
)

func init() {
	register(entry{
		name: "detector",
		desc: "a metal detector: sweep the phone over walls and objects to find steel and magnets",
		uses: []string{"magnetic_field_uncalibrated"},
		new:  func(specs []string) Demo { return &detector{} },
	})
}

// detector compares the magnetic field's strength with a baseline taken
// where nothing is around. Strength doesn't change when the phone just
// rotates, so sweeping it over a wall shows steel studs, screws, pipes and
// magnets as the field bends around them. A needle dial, a pinging indicator
// that speeds up near metal, and a strip chart of the last seconds.
type detector struct {
	base     float64 // baseline field strength, uT
	hasBase  bool
	needle   float64 // smoothed dial position 0..1
	dev      float64 // smoothed deviation, uT
	hist     []float64
	lastPing float64
	pingAt   float64
	beep     bool
	out      interface{ Write([]byte) (int, error) }
}

const detectorMax = 200.0 // uT at the end of the dial

func (d *detector) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("magnetic_field", 0)
	if err != nil {
		return nil, err
	}
	return gaugesFor(st, 3), nil
}

func (d *detector) Help() []string {
	return []string{
		"Hold the phone away from metal and press z to zero it, then sweep it slowly over a wall, a desk or an object.",
		"The needle and the pings respond to the field bending around steel (studs, screws, pipes) and magnets (speakers, headphones, bike trainers). The sensor sits near the top of the phone.",
		"z  zero here    b  vibrate on strong signals (Termux bell)",
	}
}

func (d *detector) Key(k byte) {
	switch k {
	case 'z':
		d.hasBase = false
	case 'b':
		d.beep = !d.beep
	}
}

// SetOut gives the demo the terminal, for the bell.
func (d *detector) SetOut(w interface{ Write([]byte) (int, error) }) { d.out = w }

func dialPos(dev float64) float64 {
	return clamp01(math.Log10(1+math.Abs(dev)) / math.Log10(1+detectorMax))
}

var heatCols = []uint8{34, 70, 106, 142, 178, 214, 208, 202, 196}

func (d *detector) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	r := ss.Get("magnetic_field").Read()
	if !r.OK || len(r.V) < 3 {
		msg := "waiting for the magnetometer..."
		v.Text((v.W-len(msg))/2, v.H/2, msg, 244)
		return
	}
	field := math.Sqrt(r.V[0]*r.V[0] + r.V[1]*r.V[1] + r.V[2]*r.V[2])
	if !d.hasBase {
		d.base, d.hasBase = field, true
	}
	dev := field - d.base
	d.dev += (dev - d.dev) * math.Min(1, dt*10)
	d.needle += (dialPos(d.dev) - d.needle) * math.Min(1, dt*8)
	d.hist = append(d.hist, d.dev)
	if len(d.hist) > v.W {
		d.hist = d.hist[len(d.hist)-v.W:]
	}
	heat := heatCols[min(int(d.needle*float64(len(heatCols))), len(heatCols)-1)]

	// Dial: a half circle across the upper part of the screen.
	dialH := v.H * 11 / 20
	cx, cy := float64(v.W)/2, float64(dialH)
	R := math.Min(float64(v.W)/2-3, float64(dialH-2)*2)
	at := func(pos, rr float64) (int, int) { // pos 0 (left) .. 1 (right)
		a := math.Pi * (1 - pos)
		return int(cx + rr*math.Cos(a) + 0.5), int(cy - rr*math.Sin(a)/2 + 0.5)
	}
	for p := 0.0; p <= 1.0001; p += 0.4 / R {
		x, y := at(p, R)
		c := heatCols[min(int(p*float64(len(heatCols))), len(heatCols)-1)]
		v.Set(x, y, '.', c)
	}
	for _, tick := range []float64{0, 1, 3, 10, 30, 100, 200} {
		p := dialPos(tick)
		x, y := at(p, R-1)
		v.Set(x, y, '|', 250)
		label := fmt.Sprintf("%g", tick)
		lx, ly := at(p, R+1.5)
		v.Text(lx-len(label)/2, ly, label, 244)
	}
	// The needle.
	for rr := 1.0; rr < R-1.5; rr += 0.5 {
		x, y := at(d.needle, rr)
		a := math.Pi * (1 - d.needle)
		v.Set(x, y, lineChar(math.Cos(a), -math.Sin(a)/2), heat)
	}
	v.Text(int(cx)-1, int(cy), "(O)", 250)

	// Readout and verdict.
	verdict, vcol := "quiet", uint8(244)
	switch a := math.Abs(d.dev); {
	case a > 60:
		verdict, vcol = "MAGNET!", 196
	case a > 8:
		verdict, vcol = "metal nearby", 214
	case a > 2.5:
		verdict, vcol = "something faint", 178
	}
	num := fmt.Sprintf("%+.1f uT", d.dev)
	s := max(1, fitScale(verdict, v.W-4, v.H/5))
	if s > 1 && verdict != "quiet" {
		bw, bh := bannerSize(verdict, s)
		drawBanner(v, verdict, (v.W-bw)/2, dialH+1, s, '#', vcol)
		v.Text((v.W-len(num))/2, dialH+2+bh, num, 250)
	} else {
		v.Text((v.W-len(verdict))/2, dialH+1, verdict, vcol)
		v.Text((v.W-len(num))/2, dialH+2, num, 250)
	}

	// Pings: faster near metal, like a real detector.
	rate := 0.4 + 7*d.needle*d.needle // pings per second
	if t-d.lastPing > 1/rate {
		d.lastPing, d.pingAt = t, t
		if d.beep && d.needle > 0.45 && d.out != nil {
			d.out.Write([]byte("\a"))
		}
	}
	if age := t - d.pingAt; age < 0.15 {
		p := "((( o )))"
		v.Text((v.W-len(p))/2, int(cy)-2, p, heat)
	}

	// Strip chart of the last seconds, newest at the right.
	top := v.H - max(5, v.H/5)
	for x := 0; x < v.W; x++ {
		v.Set(x, top-1, '-', 238)
	}
	chartH := v.H - top
	for i, dv := range d.hist {
		x := v.W - len(d.hist) + i
		hgt := int(dialPos(dv) * float64(chartH))
		for k := 0; k < hgt; k++ {
			c := heatCols[min(k*len(heatCols)/max(chartH, 1), len(heatCols)-1)]
			ch := byte(':')
			if k == hgt-1 {
				ch = '*'
			}
			v.Set(x, v.H-1-k, ch, c)
		}
	}
	info := fmt.Sprintf(" field %.0f uT, zeroed at %.0f ", field, d.base)
	v.Text(0, top-1, info, 244)
}
