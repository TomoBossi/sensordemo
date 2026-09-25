package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "sky",
		desc: "the light sensor as the sky: sun and clouds in bright light, moon and stars in the dark",
		uses: []string{"light"},
		new:  func(specs []string) Demo { return &sky{} },
	})
}

// sky maps light on the asymptotic scale (lux/(lux+300)) to a scene: the sun
// climbs as it gets brighter, dusk sets in around dim room light, and in the
// dark the moon, stars and lit windows come out.
type sky struct {
	day   float64 // smoothed 0 (dark) .. 1 (bright)
	stars []struct{ x, y, phase float64 }
	towns []int // building heights, per column
	w, h  int
}

func (s *sky) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("light", 0)
	if err != nil {
		return nil, err
	}
	return gaugesFor(st, 1), nil
}

func (s *sky) Help() []string {
	return []string{
		"Cover the light sensor (top of the phone) for night,",
		"point it at a lamp or window for day.",
		"The data strip bar is asymptotic: 300 lux sits",
		"in the middle, sunlight crowds toward the end.",
	}
}

func (s *sky) Key(k byte) {}

func (s *sky) layout(w, h int) {
	s.w, s.h = w, h
	rng := rand.New(rand.NewSource(3))
	s.stars = s.stars[:0]
	for i := 0; i < w*h/25; i++ {
		s.stars = append(s.stars, struct{ x, y, phase float64 }{
			rng.Float64() * float64(w), rng.Float64() * float64(h) * 0.7, rng.Float64() * 6,
		})
	}
	s.towns = make([]int, w)
	for x := 0; x < w; {
		bw, bh := 3+rng.Intn(6), 2+rng.Intn(max(2, h/4))
		for i := 0; i < bw && x < w; i++ {
			s.towns[x] = bh
			x++
		}
		if x < w && rng.Intn(3) == 0 {
			s.towns[x] = 0
			x++
		}
	}
}

func (s *sky) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 10 || v.H < 8 {
		return
	}
	if s.w != v.W || s.h != v.H {
		s.layout(v.W, v.H)
	}
	target := 0.0
	if r := ss.Get("light").Read(); r.OK && len(r.V) > 0 {
		target = r.V[0] / (r.V[0] + 300)
	}
	s.day += (target - s.day) * math.Min(1, dt*3)
	night := math.Max(0, 1-s.day*2.2) // 1 in the dark, 0 from ~450 lux up
	ground := v.H - 1

	// Stars fade in with the night and twinkle.
	for _, st := range s.stars {
		if night < 0.2 {
			break
		}
		tw := math.Sin(t*2 + st.phase)
		c := byte('.')
		if tw > 0.7 {
			c = '*'
		} else if tw > 0.3 {
			c = '+'
		}
		if math.Mod(st.phase*10, 1) < night {
			v.Set(int(st.x), int(st.y), c, 229)
		}
	}

	// Sun or moon, rising with the brightness.
	cx := float64(v.W) * 0.7
	top, bottom := float64(v.H)*0.18, float64(v.H)*0.75
	if s.day > 0.25 {
		cy := bottom - (bottom-top)*math.Min(1, (s.day-0.25)/0.6)
		rr := math.Max(2, float64(v.H)/8)
		for y := 0; y < v.H; y++ {
			for x := 0; x < v.W; x++ {
				d := math.Hypot((float64(x)-cx)/2, float64(y)-cy)
				switch {
				case d < rr:
					v.Set(x, y, '@', 226)
				case d < rr*1.9:
					// rays: spokes that turn slowly
					a := math.Atan2(float64(y)-cy, (float64(x)-cx)/2) + t*0.2
					if math.Mod(a*8/math.Pi+16, 2) < 0.35 {
						v.Set(x, y, '*', 220)
					}
				}
			}
		}
	} else {
		cy := top + 2
		rr := math.Max(2, float64(v.H)/10)
		for y := 0; y < v.H; y++ {
			for x := 0; x < v.W; x++ {
				d := math.Hypot((float64(x)-cx)/2, float64(y)-cy)
				d2 := math.Hypot((float64(x)-cx-rr*0.9)/2, float64(y)-cy+rr*0.3)
				if d < rr && d2 > rr*0.85 { // crescent
					v.Set(x, y, '(', 230)
				}
			}
		}
	}

	// Clouds drift by in daylight.
	if s.day > 0.35 {
		for i := 0; i < 4; i++ {
			w := 10 + i*3
			x0 := int(math.Mod(t*(1.5+float64(i)*0.7)+float64(i*v.W/3), float64(v.W+w))) - w
			y0 := int(float64(v.H) * (0.12 + 0.12*float64(i)))
			shape := []string{"   .--.   ", " .(    ). ", "(___.___)_"}
			for r, line := range shape {
				for k := 0; k < len(line) && k < w; k++ {
					if line[k] != ' ' {
						v.Set(x0+k, y0+r, line[k], 255)
					}
				}
			}
		}
	}

	// Skyline: dark shapes by day, lit windows at night.
	for x := 0; x < v.W; x++ {
		for k := 0; k < s.towns[x] && ground-k >= 0; k++ {
			y := ground - k
			c, col := byte('#'), uint8(240)
			if k > 0 && k < s.towns[x]-1 && x%2 == 1 {
				if night > 0.3 && (x*7+k*13)%5 != 0 {
					c, col = ':', 227 // lit window
				} else {
					c = '.'
				}
			}
			v.Set(x, y, c, col)
		}
		if s.towns[x] == 0 {
			v.Set(x, ground, '_', 240)
		}
	}
}
