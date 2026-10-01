package flicker

import (
	"fmt"
	"math"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "flicker",
		Desc: "a light bulb that plays back your light's flicker, slowed down: point the back of the phone at a lamp",
		Uses: []string{"rearflk", "rearals", "game_rotation_vector"},
		New:  func(specs []string) Demo { return &flicker{} },
	})
}

// flicker shows how the light the phone's back faces flickers. The phone
// has a flicker sensor beside its rear camera (REAR_FLK, there for the
// camera to avoid banding): it reports the frequency the light flickers at,
// in Hz, about eight times a second, 2 in the dark and 4 to 8 for light
// that doesn't flicker. Mains lighting flickers at twice the grid's
// frequency: 100 Hz on a 50 Hz grid, 120 Hz on a 60 Hz one; LEDs dimmed by
// pulsing (PWM), and some screens, at other rates.
//
// A vintage bulb hangs from its cord, in 3D, ray marched: a clear glass
// teardrop on a brass screw base, and inside, the squirrel cage of an old
// filament bulb, glowing strands zigzagging between two rings of hooks
// round a glass stem. It plays the light back eight times slower: a
// 100 Hz lamp pulses it 12.5 times a second, a 60 Hz screen 7.5. A blink
// the screen can't show at that plays at the screen's limit instead,
// bright and dim on alternate frames. The rear light sensor (REAR_ALS)
// sets how bright the filament burns, and so its color, from a dull red
// glow through orange to white-hot.
//
// The glass is drawn as the snow globe's is: an unbroken outline in line
// characters following its edge, and two room lights glinting on it,
// sliding over it as the view turns. Lit, a warm haze fills it, thickest
// round the strands; the brass threads catch the light from below; on the
// dark wall behind, a halo. Turning the phone swings the view around the
// bulb, within limits.
//
// Below, an oscilloscope draws the light's waveform in real time, and a
// chart the last half minute of what the sensor found. The sensor gives the
// frequency only, not the waveform's shape or depth: the bulb and the trace
// use the usual ones, a rectified sine for mains lighting and a square wave
// for PWM.
//
// Scene units: the glass teardrop is about 1 across, its widest a little
// below the origin; y up.
type flicker struct {
	det   detector
	lux   float64
	luxOK bool
	last  int     // readings already taken from the flicker stream
	fps   float64 // frames actually drawn a second, smoothed
	frame int
	limit bool    // blinking at the screen's limit, not eight times slower
	glow  float64 // the filament's power this frame, 0..1
	twist float64 // the bulb's slow turn on its cord, radians
	t     float64

	rest   Mat3 // the phone's resting orientation, drifting toward the current one
	hasRot bool
	eye    Vec3

	zbuf []float64 // this frame's depth per cell
}

const slow = 8 // how many times slower the bulb plays the light back

func (s *flicker) Setup(ss *Streams) ([]*Gauge, error) {
	f, err := ss.Subscribe("rearflk", 0)
	if err != nil {
		return nil, fmt.Errorf("this demo needs the rear flicker sensor (REAR_FLK, on the moto g17 power): %w", err)
	}
	gs := []*Gauge{{Spec: f.Spec, Index: 0, Label: "flicker", Unit: "Hz", Scale: &Asymptotic{K: 120}}}
	if a, err := ss.Subscribe("rearals", 0); err == nil {
		gs = append(gs, &Gauge{Spec: a.Spec, Index: 0, Label: "rear light", Unit: "lux", Scale: &Asymptotic{K: 300}})
	}
	if _, err := ss.Subscribe("game_rotation_vector", 60); err != nil {
		ss.Subscribe("rotation_vector", 60)
	}
	s.fps = 30
	s.eye = Vec3{0, math.Sin(viewEl), math.Cos(viewEl)}
	return gs, nil
}

func (s *flicker) Help() []string {
	return []string{
		"Hold the phone under a light, its back (the cameras) facing it. The bulb plays that light back eight times slower: steady light burns steadily, a lamp flickering at 100 Hz pulses it 12.5 times a second. The brighter the light, the brighter and whiter it burns. The trace below is the light's waveform, as an oscilloscope shows it.",
		"Mains lights flicker at twice the grid's frequency (100 Hz on a 50 Hz grid); dimmed LEDs and some screens, at other rates. Flicker can tire the eyes and makes bands on camera.",
		"Turn the phone to look around the bulb.    r  face it again",
	}
}

func (s *flicker) Key(k byte) {
	if k == 'r' {
		s.hasRot = false // face it from the front again
	}
}

// level is the light's brightness, 0..1, from the rear light sensor on a
// log scale.
func (s *flicker) level() float64 {
	if !s.luxOK {
		return 0.8
	}
	return Clamp01(math.Log10(math.Max(0, s.lux)+1) / 3.8)
}

// wave is the light's brightness through one flicker cycle, at phase u
// (cycles), 0..1: mains light dips toward each zero crossing (a rectified
// sine, not all the way down), PWM switches fully on and off, a slow
// flicker swells and fades.
func wave(f, u float64) float64 {
	u -= math.Floor(u)
	switch {
	case f == 100 || f == 120:
		return 0.3 + 0.7*math.Sin(math.Pi*u)
	case f < 80:
		return 0.5 + 0.5*math.Cos(2*math.Pi*u)
	}
	return BoolF(u < 0.5)
}

// BoolF is 1 for true, 0 for false.
func BoolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (s *flicker) Draw(v *View, ss *Streams, t, dt float64) {
	if st := ss.Get("rearflk"); st != nil {
		r := st.Read()
		if n := r.Count - s.last; n > 0 {
			for _, x := range st.History(0, min(n, 64)) {
				s.det.add(t, x)
			}
			s.last = r.Count
		}
	}
	s.det.update(t)
	dt0 := dt
	dt = math.Min(dt, 0.1)
	if st := ss.Get("rearals"); st != nil {
		if r := st.Read(); r.OK && len(r.V) > 0 && !math.IsNaN(r.V[0]) {
			if !s.luxOK {
				s.lux = r.V[0]
			}
			s.lux += (r.V[0] - s.lux) * math.Min(1, dt*3)
			s.luxOK = true
		}
	}
	if dt0 > 0 && dt0 < 0.5 {
		s.fps += (1/dt0 - s.fps) * math.Min(1, dt0/3) // over seconds: steady
	}
	s.frame++
	s.t = t
	s.twist = 0.7 * math.Sin(t*2*math.Pi/24) // turning slowly on its cord, and back
	s.look(ss, dt)
	s.power(t)
	if v.W < 20 || v.H < 12 {
		return
	}
	// The bulb takes most of the screen; below it, the words, the scope,
	// and on a tall screen the chart.
	scopeH, textH, chartH := 6, 2, 4
	if v.H < 70 {
		chartH = 0
	}
	if v.H < 40 {
		scopeH = 0
	}
	top := v.H - scopeH - textH - chartH
	s.render(v.Sub(0, 0, v.W, top))
	f := s.det.flicker()
	s.words(v.Sub(0, top, v.W, textH), f)
	if scopeH > 0 {
		s.scope(v.Sub(2, top+textH, v.W-4, scopeH), f, s.level(), t)
	}
	if chartH > 0 {
		s.det.chart(v.Sub(0, top+textH+scopeH+1, v.W, chartH-1), t)
	}
}

// power sets the filament's power this frame: the light's level, played
// back eight times slower, or at the screen's limit when that's still too
// fast to show.
func (s *flicker) power(t float64) {
	lvl := s.level()
	f := s.det.flicker()
	switch {
	case s.det.state == stDark:
		s.glow = 0
		s.limit = false
		return
	case f <= 0:
		s.glow = lvl
		s.limit = false
		return
	}
	// A blink needs frames to show in: past 0.45 of the frame rate it plays
	// at the limit, bright and dim on alternate frames; back under 0.38,
	// slowed down again. The gap keeps it from switching back and forth as
	// the frame rate wavers. Near the limit, a smooth wave would blur into
	// a mess of in-between frames, so it blinks crisply, on and off.
	r := f / slow / s.fps
	if s.limit && r < 0.38 || !s.limit && r > 0.45 {
		s.limit = !s.limit
	}
	if s.limit {
		s.glow = lvl * (0.12 + 0.88*float64(s.frame%2))
		return
	}
	b := wave(f, t*f/slow)
	if r > 0.2 {
		b = 0.12 + 0.88*BoolF(b > 0.6)
	}
	s.glow = lvl * b
}

// look sets the view as the snow globe does. The eye is taken to stay
// where it was while the phone turns, so the bulb is seen from the
// matching side, the angle exaggerated a little and easing into a limit of
// 35 degrees, in any direction; the resting orientation drifts toward the
// current one over a few seconds, bringing the front view back.
func (s *flicker) look(ss *Streams, dt float64) {
	sf := ScreenFrame(ss)
	eye := Vec3{0, 0, 1}
	spec := "game_rotation_vector"
	if ss.Get(spec) == nil {
		spec = "rotation_vector"
	}
	if st := ss.Get(spec); st != nil {
		if r := st.Read(); r.OK {
			if R, ok := FromRotationVector(r.V); ok {
				if !s.hasRot {
					s.rest, s.hasRot = R, true
				}
				s.rest = BlendRotation(s.rest, R, math.Min(1, dt/4))
				eye = sf.T().Apply(R.T().Apply(s.rest.Apply(Vec3{0, 0, 1})))
			}
		}
	}
	s.eye = bulbView(eye)
}

// bulbView maps where the eye is, in the screen's frame at rest (z out of
// the screen), to where the camera looks from: the angle away from straight
// on exaggerated a little and easing into 35 degrees, then raised to the
// default seat, a little above the bulb's middle.
func bulbView(eye Vec3) Vec3 {
	if lxy := math.Hypot(eye[0], eye[1]); lxy > 1e-6 {
		const lim = 35 * math.Pi / 180
		a := lim * math.Tanh(1.3*math.Atan2(lxy, eye[2])/lim)
		eye = Vec3{eye[0] / lxy * math.Sin(a), eye[1] / lxy * math.Sin(a), math.Cos(a)}
	} else if eye[2] < 0 {
		eye = Vec3{0, 0, 1}
	}
	c, sn := math.Cos(viewEl), math.Sin(viewEl)
	return Vec3{eye[0], eye[1]*c + eye[2]*sn, -eye[1]*sn + eye[2]*c}
}

// scope draws the light's waveform as an oscilloscope would, on a faint
// grid, the trace glowing; its timebase picked for about five cycles.
// Steady light is a flat line at its level; dark lies on the bottom.
func (s *flicker) scope(v *View, f, lvl, t float64) {
	w, h := v.W, v.H
	if w < 10 || h < 3 {
		return
	}
	const divs = 8
	msPerDiv := timebase(f)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gx := x%(w/divs) == 0 || x == w-1
			gy := y == 0 || y == h-1 || y == h/2
			if gx && gy {
				v.Set(x, y, '+', 238)
			} else if gx && y%2 == 0 {
				v.Set(x, y, ':', 235)
			} else if gy && x%2 == 0 {
				v.Set(x, y, '-', 235)
			}
		}
	}
	scopeRamp := []uint8{22, 28, 34, 40, 46, 82, 118, 157}
	prev := -1
	for x := 0; x < w; x++ {
		ms := float64(x) / float64(w) * divs * msPerDiv
		b := 0.0
		switch {
		case s.det.state == stDark:
		case f > 0:
			b = lvl * wave(f, f*ms/1000+t*0.3)
		default:
			b = lvl
		}
		y := h - 1 - int(Clamp01(b)*float64(h-1)+0.5)
		if prev >= 0 {
			for yy := min(prev, y) + 1; yy < max(prev, y); yy++ {
				v.Set(x, yy, '|', Pick(scopeRamp, 0.6))
			}
		}
		v.Set(x, y, '-', Pick(scopeRamp, 1))
		prev = y
	}
	label := fmt.Sprintf(" %g ms/div ", msPerDiv)
	v.Text(w-len(label)-1, 0, label, 240)
}

// timebase picks the scope's milliseconds a division, as one would turn
// its knob: from 0.5, 1, 2, 5, 10..., the one that shows about five cycles
// of the flicker across the screen; 10 for light that doesn't flicker.
func timebase(f float64) float64 {
	if f <= 0 {
		return 10
	}
	want := 5 / f * 1000 / 8
	best := 10.0
	for _, d := range []float64{0.5, 1, 2, 5, 10, 20, 50, 100} {
		if math.Abs(math.Log(d/want)) < math.Abs(math.Log(best/want)) {
			best = d
		}
	}
	return best
}

// words says what the light does, and how the bulb plays it. Only settled
// things show, so it changes when the light does, not with the sensor's
// every reading.
func (s *flicker) words(v *View, f float64) {
	head, col := s.det.verdict()
	v.Text(max(0, (v.W-len(head))/2), 0, head, col)
	var info string
	switch {
	case f > 0 && s.limit:
		info = fmt.Sprintf("even %dx slower that's too fast to show: the bulb blinks at the screen's limit", slow)
	case f > 0:
		info = fmt.Sprintf("the bulb plays it %dx slower: %g blinks a second", slow, f/slow)
	case s.det.state == stSteady:
		info = "the bulb burns steadily, as bright as the light the phone sees"
	}
	if v.H > 1 && info != "" {
		v.Text(max(0, (v.W-len(info))/2), 1, info, 244)
	}
}
