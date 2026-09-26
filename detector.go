package main

import (
	"fmt"
	"math"
)

func init() {
	register(entry{
		name: "detector",
		desc: "a metal detector: sweep the phone over walls and objects to find iron, steel and magnets",
		uses: []string{"magnetic_field_uncalibrated"},
		new:  func(specs []string) Demo { return &detector{} },
	})
}

// detector compares the magnetic field's strength with a zero taken where
// the phone is. Strength doesn't change when the phone just rotates, so
// sweeping it over a wall shows steel studs, screws, pipes and magnets as
// the field bends around them.
//
// The magnetometer only sees the total field, so it can't tell steel from a
// magnet: both change the field the same way. Only iron-based metals do
// (iron, steel, nickel); aluminum, copper, brass, gold and silver leave a
// steady field alone and are invisible to it. Very strong readings mean a
// magnet, or steel right against the phone, magnetized by the phone's own
// speaker and vibration motor magnets.
//
// Zeroing works like a scale's tare: it averages the field for a moment
// while you hold still, measuring the noise too. Changes within the noise
// read as zero, and the dial starts just above the noise, so it's as
// sensitive as the spot allows, and widens when bigger readings arrive.
type detector struct {
	base      float64 // zero, uT
	noise     float64 // spread of the field while zeroing, uT
	hasBase   bool
	zeroing   float64 // seconds of zeroing left; 0 = done
	sum, sum2 float64
	n         int
	scale     float64 // full scale of the dial, uT; only grows until the next zero
	shown     float64 // the scale drawn, easing toward scale
	needle    float64 // smoothed dial position 0..1
	dev       float64 // smoothed deviation beyond the noise, uT (signed)
	hist      []float64
	lastPing  float64
	pingAt    float64
	beep      bool
	out       interface{ Write([]byte) (int, error) }
}

const zeroTime = 0.6 // seconds

func (d *detector) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("magnetic_field", 0)
	if err != nil {
		return nil, err
	}
	d.zeroing = zeroTime
	return gaugesFor(st, 3), nil
}

func (d *detector) Help() []string {
	return []string{
		"Hold the phone still and press z to zero it, then sweep it slowly over a wall, a desk or an object. The sensor sits near the top of the phone.",
		"Zeroing is a tare: the dial and the chart start at the field where you zeroed, noise there reads as zero, and the scale starts small and widens for bigger readings. Zero far from metal to find faint things; zero next to something to ignore it.",
		"It finds iron, steel and nickel, and magnets: it can't tell them apart, since both bend the magnetic field. Aluminum, copper, brass, gold and silver don't show. Steel touching the phone reads very strong, magnetized by the phone's own magnets.",
		"z  zero here    b  vibrate on strong signals (Termux bell)",
	}
}

func (d *detector) Key(k byte) {
	switch k {
	case 'z':
		d.zeroing, d.sum, d.sum2, d.n = zeroTime, 0, 0, 0
	case 'b':
		d.beep = !d.beep
	}
}

// SetOut gives the demo the terminal, for the bell.
func (d *detector) SetOut(w interface{ Write([]byte) (int, error) }) { d.out = w }

var heatCols = []uint8{34, 70, 106, 142, 178, 214, 208, 202, 196}

func heat(p float64) uint8 {
	return heatCols[min(int(clamp01(p)*float64(len(heatCols))), len(heatCols)-1)]
}

// detLabel prints a dial value with a sensible number of digits.
func detLabel(x float64) string {
	if x >= 10 || x == math.Trunc(x) {
		return fmt.Sprintf("%.0f", x)
	}
	return fmt.Sprintf("%.1f", x)
}

// niceScale rounds a full scale up to 1, 2 or 5 times a power of ten.
func niceScale(x float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(x)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*p >= x {
			return m * p
		}
	}
	return 10 * p
}

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

	if d.zeroing > 0 {
		d.sum += field
		d.sum2 += field * field
		d.n++
		d.zeroing -= dt
		if d.zeroing <= 0 {
			d.zeroing = 0
			mean := d.sum / float64(d.n)
			d.base = mean
			d.noise = math.Sqrt(math.Max(0, d.sum2/float64(d.n)-mean*mean))
			d.hasBase = true
			d.scale = niceScale(math.Max(2, 8*math.Max(d.noise, 0.15)))
			d.shown = d.scale
			d.dev, d.needle = 0, 0
			d.hist = d.hist[:0]
		}
	}
	if !d.hasBase {
		msg := "zeroing: hold still..."
		v.Text((v.W-len(msg))/2, v.H/2, msg, 250)
		return
	}

	// Deviation beyond the noise: within three standard deviations of the
	// zero, it reads zero.
	raw := field - d.base
	band := 3 * math.Max(d.noise, 0.1)
	dev := math.Copysign(math.Max(0, math.Abs(raw)-band), raw)
	d.dev += (dev - d.dev) * math.Min(1, dt*10)
	if a := math.Abs(d.dev); a > d.scale*0.9 {
		d.scale = niceScale(a / 0.9) // widen for bigger readings
	}
	d.shown += (d.scale - d.shown) * math.Min(1, dt*4)
	d.needle += (clamp01(math.Abs(d.dev)/d.shown) - d.needle) * math.Min(1, dt*8)
	d.hist = append(d.hist, d.dev)
	if len(d.hist) > v.W {
		d.hist = d.hist[len(d.hist)-v.W:]
	}
	hc := heat(d.needle)

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
		v.Set(x, y, '.', heat(p))
	}
	for i := 0; i <= 4; i++ {
		p := float64(i) / 4
		x, y := at(p, R-1)
		v.Set(x, y, '|', 250)
		label := detLabel(p * d.shown)
		if i == 0 {
			label = "0"
		}
		lx, ly := at(p, R+1.5)
		v.Text(lx-len(label)/2, ly, label, 244)
	}
	for rr := 1.0; rr < R-1.5; rr += 0.5 {
		x, y := at(d.needle, rr)
		a := math.Pi * (1 - d.needle)
		v.Set(x, y, lineChar(math.Cos(a), -math.Sin(a)/2), hc)
	}
	v.Text(int(cx)-1, int(cy), "(O)", 250)

	// Readout and verdict, by absolute strength.
	verdict, vcol := "quiet", uint8(244)
	switch a := math.Abs(d.dev); {
	case a > 60:
		verdict, vcol = "STRONG!", 196
	case a > 8:
		verdict, vcol = "metal nearby", 214
	case a > 1:
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
	if verdict == "STRONG!" {
		hint := "a magnet, or steel up close"
		v.Text((v.W-len(hint))/2, dialH, hint, 244)
	}

	// Pings: faster as the needle climbs, like a real detector.
	rate := 0.4 + 7*d.needle*d.needle // pings per second
	if t-d.lastPing > 1/rate {
		d.lastPing, d.pingAt = t, t
		if d.beep && d.needle > 0.45 && d.out != nil {
			d.out.Write([]byte("\a"))
		}
	}
	if t-d.pingAt < 0.15 {
		p := "((( o )))"
		v.Text((v.W-len(p))/2, int(cy)-2, p, hc)
	}

	// Strip chart of the last seconds on the same scale, newest at the
	// right; the zero is the bottom.
	top := v.H - max(5, v.H/5)
	for x := 0; x < v.W; x++ {
		v.Set(x, top-1, '-', 238)
	}
	chartH := v.H - top
	for i, dv := range d.hist {
		x := v.W - len(d.hist) + i
		hgt := int(clamp01(math.Abs(dv)/d.shown)*float64(chartH) + 0.5)
		for k := 0; k < hgt; k++ {
			ch := byte(':')
			if k == hgt-1 {
				ch = '*'
			}
			v.Set(x, v.H-1-k, ch, heat(float64(k+1)/float64(chartH)))
		}
	}
	info := fmt.Sprintf(" field %.0f uT, zero %.1f, noise +-%.1f, scale %s uT ", field, d.base, band, detLabel(d.shown))
	if len(info) > v.W {
		info = fmt.Sprintf(" zero %.1f  noise +-%.1f ", d.base, band)
	}
	v.Text(0, top-1, info, 244)
}
