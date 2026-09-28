package sky

import (
	"fmt"
	"math"
	"math/rand"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "sky",
		Desc: "the light sensor as one disc: a moon that waxes with light, glows, and turns into a blazing sun",
		Uses: []string{"light"},
		New:  func(specs []string) Demo { return &sky{} },
	})
}

// sky maps the light level, on a log scale, onto a single disc. In the dark
// it is a new moon lit only by earthshine among stars; more light waxes it
// through crescent and half to full; brighter still it glows and the faint
// stars go out; toward sunlight it turns, cell by cell, into the sun, with a
// churning surface, a corona and flares.
//
// The moon is drawn from the real near side (maria, craters, Tycho's rays),
// upside down as seen from the southern hemisphere.
type sky struct {
	logLux float64 // smoothed log10(lux+1)
	has    bool

	// cached per size
	w, h   int
	albedo []float64 // moon surface brightness per disc cell
	stars  []skyStar
}

type skyStar struct {
	x, y int
	mag  float64 // 0 bright .. 1 faint
	c    byte
	fg   uint8
}

func (s *sky) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("light", 0)
	if err != nil {
		return nil, err
	}
	return GaugesFor(st, 1), nil
}

func (s *sky) Help() []string {
	return []string{
		"One disc driven by the light sensor (top of the phone): cover it for a new moon among the stars; room light waxes it to full; bright light makes it glow; sunlight turns it into the sun.",
		"The moon is the real near side (seas, craters, Tycho's rays), upside down as seen from the southern hemisphere.",
	}
}

func (s *sky) Key(k byte) {}

// Moon features in disc coordinates (x right, y up), as seen from the
// northern hemisphere; flipped for the southern one when drawn.
var maria = []struct{ x, y, rx, ry float64 }{
	{-0.28, 0.42, 0.3, 0.24},   // Imbrium
	{0.27, 0.37, 0.16, 0.15},   // Serenitatis
	{0.38, 0.08, 0.2, 0.17},    // Tranquillitatis
	{0.72, 0.28, 0.1, 0.09},    // Crisium
	{-0.62, 0.05, 0.22, 0.42},  // Procellarum
	{-0.18, -0.33, 0.17, 0.13}, // Nubium
	{-0.52, -0.4, 0.11, 0.1},   // Humorum
	{0.6, -0.1, 0.12, 0.17},    // Fecunditatis
	{0.44, -0.33, 0.1, 0.1},    // Nectaris
	{0.0, 0.72, 0.4, 0.07},     // Frigoris
	{0.02, 0.02, 0.12, 0.1},    // Vaporum and Sinus Medii
}

var tycho = [2]float64{-0.13, -0.72}

type crater struct{ x, y, r float64 }

func makeCraters() []crater {
	rng := rand.New(rand.NewSource(11))
	cs := []crater{{-0.31, 0.16, 0.06}, {tycho[0], tycho[1], 0.055}, {-0.46, 0.33, 0.04}, {0.05, -0.55, 0.07}}
	for len(cs) < 60 {
		x, y := rng.Float64()*2-1, rng.Float64()*2-1
		if x*x+y*y > 0.92 {
			continue
		}
		cs = append(cs, crater{x, y, 0.015 + 0.05*math.Pow(rng.Float64(), 2)})
	}
	return cs
}

var craters = makeCraters()

// moonAlbedo is the brightness of the moon's surface at disc point (x, y):
// bright highlands, dark maria with irregular edges, craters with bright
// rims and darker floors, and Tycho's rays.
func moonAlbedo(x, y float64) float64 {
	a := 0.78 + 0.12*FBM(x*6, y*6, 0.5, 4) // highland texture
	edge := 0.25 * FBM(x*5+7, y*5, 1.5, 3) // ragged sea shores
	for _, m := range maria {
		d := math.Hypot((x-m.x)/m.rx, (y-m.y)/m.ry) + edge
		a -= 0.36 * Clamp01((1.15-d)/0.3)
	}
	for _, c := range craters {
		d := math.Hypot(x-c.x, y-c.y) / c.r
		switch {
		case d < 0.8:
			a -= 0.08
		case d < 1.15:
			a += 0.1
		}
	}
	// Tycho's rays: bright streaks radiating across the southern half.
	dx, dy := x-tycho[0], y-tycho[1]
	if r := math.Hypot(dx, dy); r > 0.06 && r < 1.3 {
		ang := math.Atan2(dy, dx)
		ray := math.Pow(math.Max(0, math.Sin(ang*7+1.3)*math.Sin(ang*11)), 6)
		a += 0.22 * ray * (1 - r/1.3)
	}
	return Clamp01(a)
}

var (
	skyRamp   = []byte(".,:-=+*#%@")
	moonCols  = []uint8{235, 237, 239, 241, 243, 245, 247, 250, 252, 255}
	glowCols  = []uint8{238, 241, 244, 247, 250, 253, 230, 230, 231, 231}
	sunCols   = []uint8{52, 88, 124, 160, 202, 208, 214, 220, 226, 229}
	coronaCol = []uint8{52, 88, 124, 160, 166, 202, 208, 214}
)

func (s *sky) layout(w, h int) {
	s.w, s.h = w, h
	rng := rand.New(rand.NewSource(5))
	s.stars = s.stars[:0]
	for i := 0; i < w*h/18; i++ {
		m := rng.Float64()
		st := skyStar{x: rng.Intn(w), y: rng.Intn(h), mag: m, c: '.', fg: 244}
		switch {
		case m < 0.04:
			st.c, st.fg = '*', 231
		case m < 0.14:
			st.c, st.fg = '+', 252
		case m < 0.35:
			st.fg = 249
		}
		if rng.Intn(10) == 0 {
			st.fg = []uint8{153, 217, 229, 159}[rng.Intn(4)]
		}
		s.stars = append(s.stars, st)
	}
	s.albedo = nil // recomputed for the new disc size
}

func (s *sky) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 6 {
		return
	}
	if s.w != v.W || s.h != v.H {
		s.layout(v.W, v.H)
	}
	lux := 0.0
	if r := ss.Get("light").Read(); r.OK && len(r.V) > 0 {
		lux = r.V[0]
	}
	target := math.Log10(math.Max(0, lux) + 1)
	if !s.has {
		s.logLux, s.has = target, true
	}
	s.logLux += (target - s.logLux) * math.Min(1, dt*2.5)
	l := s.logLux

	phase := Clamp01(l / 2.3)          // new (0 lux) .. full (~200 lux)
	glow := Clamp01((l - 2.3) / 1.2)   // ~200 .. ~3000 lux
	sunT := Clamp01((l - 3.3) / 1.0)   // ~2000 .. ~20000 lux: moon becomes sun
	starVis := (1 - glow) * (1 - sunT) // stars fade as the sky brightens

	cx, cy := float64(v.W)/2, float64(v.H)/2-0.5
	R := math.Min(float64(v.W)/2, float64(v.H)) * 0.47 // radius in columns
	if sunT > 0 {
		R *= 1 - 0.25*sunT // leave room for the corona
	}

	// Stars first; the disc and its glow cover them. Faint stars go first.
	for _, st := range s.stars {
		if st.mag < starVis*0.9+0.1*starVis {
			d := math.Hypot((float64(st.x)-cx)/R, (float64(st.y)-cy)*2/R)
			if d > 1.05 {
				v.Set(st.x, st.y, st.c, st.fg)
			}
		}
	}

	// The moon is lit by a sun that swings from behind it (new) to the
	// side (half) to in front (full), from the right: a waxing moon as seen
	// from the south is lit on its left, so light comes from -x here.
	th := phase * math.Pi
	light := Vec3{-math.Sin(th), 0, -math.Cos(th)}
	earthshine := 0.05 * (1 - phase)

	if len(s.albedo) != v.W*v.H {
		s.albedo = make([]float64, v.W*v.H)
		ParallelRows(v.H, func(y int) {
			for x := 0; x < v.W; x++ {
				px := (float64(x) + 0.5 - cx) / R
				py := -(float64(y) + 0.5 - cy) * 2 / R
				if px*px+py*py < 1 {
					s.albedo[y*v.W+x] = moonAlbedo(-px, -py) // southern view: upside down
				}
			}
		})
	}

	ParallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			px := (float64(x) + 0.5 - cx) / R
			py := -(float64(y) + 0.5 - cy) * 2 / R
			r := math.Hypot(px, py)
			if r < 1 {
				s.drawDisc(v, x, y, px, py, r, light, earthshine, glow, sunT, t)
			} else {
				s.drawHalo(v, x, y, px, py, r, glow, sunT, t)
			}
		}
	})

	caption := s.caption(lux, phase, glow, sunT)
	v.Text(max(0, (v.W-len(caption))/2), v.H-1, caption, 244)
}

func (s *sky) drawDisc(v *View, x, y int, px, py, r float64, light Vec3, earthshine, glow, sunT, t float64) {
	z := math.Sqrt(1 - r*r)
	n := Vec3{px, py, z}

	// Moon: sunlight on the albedo map, earthshine on the dark side, and
	// brighter as it glows.
	alb := s.albedo[y*v.W+x]
	moon := alb * (math.Max(0, n.Dot(light))*(0.85+0.4*glow) + earthshine)

	// Sun: limb darkening and a churning granulated surface.
	gran := 0.8 + 0.4*FBM(px*7, py*7, t*0.35, 4)
	flow := 0.5 + FBM(px*2.5+t*0.05, py*2.5, t*0.12, 3)
	sun := (0.45 + 0.55*math.Pow(z, 0.5)) * gran * (0.85 + 0.3*flow)

	// The morph: each cell turns sun at its own moment (fixed noise), so
	// the surface transforms patch by patch rather than all at once.
	isSun := sunT > 0 && sunT >= 0.02+0.96*CellNoise(x, y)
	lum := moon
	cols := moonCols
	switch {
	case isSun:
		lum, cols = sun*(0.7+0.3*sunT), sunCols
	case glow > 0.3 || sunT > 0:
		cols = glowCols
		lum = math.Min(1, moon*(1+0.5*sunT))
	}
	if lum < 0.03 {
		return
	}
	k := min(int(lum*float64(len(skyRamp))), len(skyRamp)-1)
	v.Set(x, y, skyRamp[k], cols[k])
}

// drawHalo draws around the disc: the moon's glow as it brightens, and the
// sun's corona (flickering rays) and prominences (loops off the limb).
func (s *sky) drawHalo(v *View, x, y int, px, py, r, glow, sunT, t float64) {
	ang := math.Atan2(py, px)
	lum, cols := 0.0, glowCols
	if glow > 0 && sunT < 1 {
		lum = glow * (1 - sunT) * 0.45 * math.Exp(-(r-1)*5)
	}
	if sunT > 0 {
		// Corona: rays of varying length that flicker slowly.
		rays := 0.75 + 1.6*FBM(math.Cos(ang)*3+10, math.Sin(ang)*3, t*0.4, 3)
		rays += 0.8 * math.Pow(math.Max(0, math.Sin(ang*9+t*0.3)), 6)
		c := sunT * rays * math.Exp(-(r-1)*2.1)
		// Prominences: a few loops standing on the limb, gently pulsing.
		for i, base := range []float64{0.6, 2.3, 3.9, 5.2} {
			h := 0.22 + 0.06*math.Sin(t*0.7+float64(i))
			a := math.Remainder(ang-base, 2*math.Pi)
			// a loop is an arch: a half ring standing on the limb
			lx, ly := a/0.22, (r-1)/h
			if ly > -0.05 && math.Abs(math.Hypot(lx, ly)-1) < 0.32 {
				c = math.Max(c, sunT*(0.9+0.1*math.Sin(t*2+float64(i))))
			}
		}
		if c > lum {
			lum, cols = c, coronaCol
		}
	}
	if lum < 0.06 || lum < CellNoise(x, y)*0.2 {
		return
	}
	k := min(int(lum*float64(len(skyRamp))), len(skyRamp)-1)
	v.Set(x, y, skyRamp[k], cols[min(k, len(cols)-1)])
}

func (s *sky) caption(lux, phase, glow, sunT float64) string {
	name := ""
	switch {
	case sunT > 0.85:
		name = "the sun"
	case sunT > 0.05:
		name = "the moon turning into the sun"
	case glow > 0.3:
		name = "a glowing full moon"
	case phase > 0.93:
		name = "full moon"
	case phase > 0.6:
		name = "waxing gibbous"
	case phase > 0.4:
		name = "first quarter (half moon)"
	case phase > 0.07:
		name = "waxing crescent"
	default:
		name = "new moon"
	}
	return fmt.Sprintf("%s  -  %.0f lux", name, lux)
}
