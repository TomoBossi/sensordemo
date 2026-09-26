package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
// the phone is. Sweeping it over a wall shows steel studs, screws, pipes and
// magnets as the field bends around them.
//
// The raw magnetometer reading is Earth's field plus a fixed offset from
// the phone's own parts (speaker, motor, screws), and on some phones
// Android never manages to estimate that offset. Then the strength changes
// wildly just by turning the phone. So the demo calibrates itself: turned
// through six poses, the raw readings lie on a sphere around the offset,
// and a least-squares fit finds it. With the offset removed, the strength
// stays put as the phone turns and only metal changes it. The fit is saved
// in the state directory and reused.
//
// It can't tell steel from a magnet (both bend the field), and only iron,
// steel and nickel show; aluminum, copper, brass, gold and silver don't.
//
// Zeroing is a plain tare: it averages the field once the phone is still,
// and stays until the next zero. Changes within the noise read as zero.
type detector struct {
	cal     magCal
	hasCal  bool
	calMsg  string  // result of the last calibration, shown for a moment
	calMsgT float64 // when
	calib   *calibration

	base      float64
	noise     float64
	hasBase   bool
	zeroing   float64
	sum, sum2 float64
	n         int
	offWarn   bool // the zeroed field is far from the calibrated one

	needle   float64
	dev      float64
	hist     []float64
	lastPing float64
	pingAt   float64
	beep     bool
	out      interface{ Write([]byte) (int, error) }
	source   string // the magnetometer stream
}

// magCal is the phone's magnetometer offset, found by calibrating.
type magCal struct {
	Bias   Vec3    `json:"bias"`   // uT, in the phone's frame
	Radius float64 `json:"radius"` // Earth's field where it was calibrated, uT
}

// calibration collects raw readings while the phone is turned through six
// poses (each face of the phone pointing up).
type calibration struct {
	samples []Vec3
	held    [6]float64 // seconds each pose has been held
}

var calPoses = [6]struct {
	axis Vec3
	name string
}{
	{Vec3{0, 0, 1}, "screen up"},
	{Vec3{0, 0, -1}, "screen down"},
	{Vec3{0, 1, 0}, "upright"},
	{Vec3{0, -1, 0}, "upside down"},
	{Vec3{1, 0, 0}, "on its left edge"},
	{Vec3{-1, 0, 0}, "on its right edge"},
}

const (
	zeroTime  = 0.6 // seconds of stillness a zero averages
	zeroStill = 1.5 // uT: more spread than this is movement, not noise
	maxBand   = 3.0 // uT: the widest noise band a zero may set
	poseHold  = 0.5 // seconds each calibration pose must be held
	dialMax   = 200.0
)

func calPath() string {
	if p := os.Getenv("SENSORDEMO_MAGCAL"); p != "" {
		return p
	}
	d := os.Getenv("XDG_STATE_HOME")
	if d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(d, "sensordemo", "magcal.json")
}

func (d *detector) Setup(ss *Streams) ([]*Gauge, error) {
	d.source = "magnetic_field_uncalibrated"
	st, err := ss.Subscribe(d.source, 0)
	if err != nil {
		d.source = "magnetic_field"
		if st, err = ss.Subscribe(d.source, 0); err != nil {
			return nil, err
		}
	}
	if _, err := ss.Subscribe("gravity", 30); err != nil {
		ss.Subscribe("accelerometer", 30)
	}
	if b, err := os.ReadFile(calPath()); err == nil && json.Unmarshal(b, &d.cal) == nil && d.cal.Radius > 0 {
		d.hasCal = true
		d.startZero()
	} else {
		d.calib = &calibration{}
	}
	return gaugesFor(st, 3), nil
}

func (d *detector) Help() []string {
	return []string{
		"First it calibrates: turn the phone through the six poses on screen and hold each a moment. That's saved; press k to do it again if turning the phone moves the needle.",
		"Hold the phone still and press z to zero it, then sweep it slowly over a wall, a desk or an object. The sensor sits near the top of the phone. The zero stays until you press z again.",
		"It finds iron, steel and nickel, and magnets: it can't tell them apart, since both bend the magnetic field. Aluminum, copper, brass, gold and silver don't show. Steel touching the phone reads very strong, magnetized by the phone's own magnets.",
		"z  zero here    k  calibrate    b  vibrate on strong signals (Termux bell)",
		"b rings the terminal bell, which Termux turns into a short vibration; it needs vibration on in Android's Sound & vibration settings.",
	}
}

func (d *detector) Key(k byte) {
	switch k {
	case 'z':
		if d.hasCal {
			d.startZero()
		}
	case 'k':
		d.calib = &calibration{}
	case 'b':
		d.beep = !d.beep
	}
}

func (d *detector) startZero() {
	d.zeroing, d.sum, d.sum2, d.n = zeroTime, 0, 0, 0
}

// SetOut gives the demo the terminal, for the bell.
func (d *detector) SetOut(w interface{ Write([]byte) (int, error) }) { d.out = w }

// fitSphere finds the center and radius of the sphere the points lie on,
// by linear least squares: |p|^2 = 2 p.c + (r^2 - |c|^2). It returns the
// RMS distance of the points from the sphere too.
func fitSphere(pts []Vec3) (c Vec3, r, rms float64, ok bool) {
	var a [4][5]float64 // normal equations, augmented
	for _, p := range pts {
		row := [4]float64{2 * p[0], 2 * p[1], 2 * p[2], 1}
		y := p.Dot(p)
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				a[i][j] += row[i] * row[j]
			}
			a[i][4] += row[i] * y
		}
	}
	for col := 0; col < 4; col++ { // Gauss-Jordan with partial pivoting
		piv := col
		for i := col + 1; i < 4; i++ {
			if math.Abs(a[i][col]) > math.Abs(a[piv][col]) {
				piv = i
			}
		}
		if math.Abs(a[piv][col]) < 1e-9 {
			return c, 0, 0, false
		}
		a[col], a[piv] = a[piv], a[col]
		for i := 0; i < 4; i++ {
			if i == col {
				continue
			}
			f := a[i][col] / a[col][col]
			for j := col; j < 5; j++ {
				a[i][j] -= f * a[col][j]
			}
		}
	}
	c = Vec3{a[0][4] / a[0][0], a[1][4] / a[1][1], a[2][4] / a[2][2]}
	r2 := a[3][4]/a[3][3] + c.Dot(c)
	if r2 <= 0 {
		return c, 0, 0, false
	}
	r = math.Sqrt(r2)
	for _, p := range pts {
		e := p.Sub(c).Len() - r
		rms += e * e
	}
	return c, r, math.Sqrt(rms / float64(len(pts))), true
}

// calibrate collects a reading and checks the poses; when all are done it
// fits the offset and saves it.
func (d *detector) calibrate(raw Vec3, up Vec3, hasUp bool, dt, t float64) {
	c := d.calib
	if len(c.samples) < 4000 {
		c.samples = append(c.samples, raw)
	}
	if hasUp {
		for i, p := range calPoses {
			if up.Dot(p.axis) > 0.9 {
				c.held[i] += dt
			}
		}
	}
	for _, h := range c.held {
		if h < poseHold {
			return
		}
	}
	bias, r, rms, ok := fitSphere(c.samples)
	d.calib = nil
	d.calMsgT = t
	switch {
	case !ok || r < 15 || r > 80:
		d.calMsg = "calibration failed: try again away from metal (k)"
		return
	case rms > 3:
		d.calMsg = fmt.Sprintf("calibration was rough (+-%.0f uT): k to try again away from metal", rms)
	default:
		d.calMsg = fmt.Sprintf("calibrated: Earth's field here is %.0f uT", r)
	}
	d.cal, d.hasCal = magCal{Bias: bias, Radius: r}, true
	if b, err := json.MarshalIndent(d.cal, "", "  "); err == nil {
		os.MkdirAll(filepath.Dir(calPath()), 0o755)
		os.WriteFile(calPath(), append(b, '\n'), 0o644)
	}
	d.startZero()
	d.hasBase = false
}

func (d *detector) drawCalibration(v *View) {
	c := d.calib
	lines := []string{
		"CALIBRATING",
		"",
		"Turn the phone slowly through these six poses,",
		"holding each still for a moment, away from metal:",
	}
	y := max(0, v.H/2-8)
	for _, l := range lines {
		for _, w := range wrap(l, v.W-2) {
			v.Text(max(0, (v.W-len(w))/2), y, w, 252)
			y++
		}
	}
	y++
	colW := max(20, v.W/2)
	cols := max(1, min(2, v.W/colW))
	for i, p := range calPoses {
		frac := clamp01(c.held[i] / poseHold)
		mark, col := "[ ]", uint8(244)
		if frac >= 1 {
			mark, col = "[x]", 82
		} else if frac > 0 {
			mark, col = "[.]", 220
		}
		x := (i%cols)*colW + max(1, (colW-20)/2)
		v.Text(x, y+(i/cols)*2, mark+" "+p.name, col)
	}
	y += (len(calPoses)+cols-1)/cols*2 + 1
	hint := "screen up = lying on a table; upright = held in front of you"
	for _, w := range wrap(hint, v.W-2) {
		v.Text(max(0, (v.W-len(w))/2), y, w, 244)
		y++
	}
}

var heatCols = []uint8{34, 70, 106, 142, 178, 214, 208, 202, 196}

func heat(p float64) uint8 {
	return heatCols[min(int(clamp01(p)*float64(len(heatCols))), len(heatCols)-1)]
}

// dialPos is where a deviation sits on the dial: a log scale, so small
// changes show and big ones still fit.
func dialPos(dev float64) float64 {
	return clamp01(math.Log10(1+math.Abs(dev)) / math.Log10(1+dialMax))
}

func (d *detector) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	r := ss.Get(d.source).Read()
	if !r.OK || len(r.V) < 3 {
		msg := "waiting for the magnetometer..."
		v.Text((v.W-len(msg))/2, v.H/2, msg, 244)
		return
	}
	raw := Vec3{r.V[0], r.V[1], r.V[2]}
	if d.calib != nil {
		var up Vec3
		hasUp := false
		spec := "gravity"
		if ss.Get(spec) == nil {
			spec = "accelerometer"
		}
		if s := ss.Get(spec); s != nil {
			if g := s.Read(); g.OK && len(g.V) >= 3 {
				up, hasUp = Vec3{g.V[0], g.V[1], g.V[2]}.Norm(), true
			}
		}
		d.calibrate(raw, up, hasUp, dt, t)
		if d.calib != nil {
			d.drawCalibration(v)
			return
		}
	}
	if !d.hasCal {
		return
	}
	field := raw.Sub(d.cal.Bias).Len()

	if d.zeroing > 0 {
		d.sum += field
		d.sum2 += field * field
		d.n++
		mean := d.sum / float64(d.n)
		spread := math.Sqrt(math.Max(0, d.sum2/float64(d.n)-mean*mean))
		if spread > zeroStill { // moving: start over
			d.sum, d.sum2, d.n = field, field*field, 1
			d.zeroing, spread = zeroTime, 0
		}
		d.zeroing -= dt
		if d.zeroing <= 0 && d.n > 1 {
			d.zeroing = 0
			d.base, d.noise, d.hasBase = mean, spread, true
			d.offWarn = math.Abs(mean-d.cal.Radius) > 0.3*d.cal.Radius
			d.dev, d.needle = 0, 0
			d.hist = d.hist[:0]
		} else if d.zeroing <= 0 {
			d.zeroing = dt
		}
		if d.zeroing > 0 {
			msg := "zeroing: hold still..."
			v.Text((v.W-len(msg))/2, v.H/2-1, msg, 250)
			if !d.hasBase {
				return
			}
		}
	}
	if !d.hasBase {
		return
	}

	// Deviation beyond the noise: within three standard deviations of the
	// zero (at most maxBand), it reads zero.
	diff := field - d.base
	band := math.Min(3*math.Max(d.noise, 0.1), maxBand)
	dev := math.Copysign(math.Max(0, math.Abs(diff)-band), diff)
	d.dev += (dev - d.dev) * math.Min(1, dt*10)
	d.needle += (dialPos(d.dev) - d.needle) * math.Min(1, dt*8)
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
	for _, tick := range []float64{0, 1, 3, 10, 30, 100, 200} {
		p := dialPos(tick)
		x, y := at(p, R-1)
		v.Set(x, y, '|', 250)
		label := fmt.Sprintf("%g", tick)
		lx, ly := at(p, R+1.5)
		v.Text(lx-len(label)/2, ly, label, 244)
	}
	for rr := 1.0; rr < R-1.5; rr += 0.5 {
		x, y := at(d.needle, rr)
		a := math.Pi * (1 - d.needle)
		v.Set(x, y, lineChar(math.Cos(a), -math.Sin(a)/2), hc)
	}
	v.Text(int(cx)-1, int(cy), "(O)", 250)

	// Readout and verdict.
	verdict, vcol := "quiet", uint8(244)
	switch a := math.Abs(d.dev); {
	case a > 60:
		verdict, vcol = "STRONG!", 196
	case a > 8:
		verdict, vcol = "metal nearby", 214
	case a > 1.5:
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

	// Messages at the top: the calibration result for a while, or a
	// warning if the zero is far from the calibrated field.
	switch {
	case d.calMsg != "" && t-d.calMsgT < 6:
		v.Text(max(0, (v.W-len(d.calMsg))/2), 0, d.calMsg, 82)
	case d.offWarn:
		w := "zero far from the calibrated field: metal here, or press k"
		v.Text(max(0, (v.W-len(w))/2), 0, w, 208)
	}

	// Pings: faster near metal, like a real detector.
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

	// Strip chart of the last seconds, newest at the right; the zero is
	// the bottom.
	top := v.H - max(5, v.H/5)
	for x := 0; x < v.W; x++ {
		v.Set(x, top-1, '-', 238)
	}
	chartH := v.H - top
	for i, dv := range d.hist {
		x := v.W - len(d.hist) + i
		hgt := int(dialPos(dv)*float64(chartH) + 0.5)
		for k := 0; k < hgt; k++ {
			ch := byte(':')
			if k == hgt-1 {
				ch = '*'
			}
			v.Set(x, v.H-1-k, ch, heat(float64(k+1)/float64(chartH)))
		}
	}
	info := fmt.Sprintf(" field %.1f uT, zero %.1f, noise +-%.1f ", field, d.base, band)
	v.Text(0, top-1, info, 244)
}
