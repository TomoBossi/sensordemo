package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "eye",
		desc: "an eye that shuts when you cover the proximity sensor, and looks where you tilt",
		uses: []string{"proximity"},
		new:  func(specs []string) Demo { return &eye{open: 1, rng: rand.New(rand.NewSource(7))} },
	})
}

// eye draws an almond eye with an iris. Covering the proximity sensor (top of
// the screen, near the earpiece) closes it; tilting the phone moves the pupil
// toward the low side; now and then it blinks.
type eye struct {
	open      float64 // 0 closed .. 1 open, animated
	lookX     float64
	lookY     float64
	nextBlink float64
	blinkEnd  float64
	rng       *rand.Rand
}

func (e *eye) Setup(ss *Streams) ([]*Gauge, error) {
	p, err := ss.Subscribe("proximity", 0)
	if err != nil {
		return nil, err
	}
	g := gaugesFor(p, 1)
	if a, err := ss.Subscribe("accelerometer", 30); err == nil {
		g = append(g, gaugesFor(a, 2)...)
	}
	return g, nil
}

func (e *eye) Help() []string {
	return []string{
		"Cover the top of the phone (near the earpiece):",
		"the eye closes. Tilt the phone and it looks",
		"toward the low side. It blinks on its own too.",
	}
}

func (e *eye) Key(k byte) {
	if k == ' ' {
		e.nextBlink = 0
	}
}

func (e *eye) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 6 {
		return
	}
	near := false
	if r := ss.Get("proximity").Read(); r.OK && len(r.V) > 0 {
		near = r.V[0] < 1 // this sensor reports 0 (near) or its max (far)
	}
	if t >= e.nextBlink {
		e.blinkEnd = t + 0.15
		e.nextBlink = t + 2.5 + e.rng.Float64()*4
	}
	target := 1.0
	if near || t < e.blinkEnd {
		target = 0
	}
	e.open += (target - e.open) * math.Min(1, dt*18)

	if a := ss.Get("accelerometer"); a != nil {
		if r := a.Read(); r.OK && len(r.V) >= 2 {
			// gravity in screen coords is (-ax, +ay); look toward it
			g := toScreen(ss, r.V)
			tx, ty := math.Max(-1, math.Min(1, -g[0]/6)), math.Max(-1, math.Min(1, (g[1]-6)/5))
			e.lookX += (tx - e.lookX) * math.Min(1, dt*6)
			e.lookY += (ty - e.lookY) * math.Min(1, dt*6)
		}
	}

	cx, cy := float64(v.W)/2, float64(v.H)/2
	rx := math.Min(float64(v.W)*0.45, float64(v.H)*1.6) // half-width in columns
	ry := rx * 0.42 / 2                                 // half-height in rows
	ri := ry * 0.95                                     // iris radius in rows
	ix := cx + e.lookX*rx*0.45
	iy := cy + e.lookY*ry*0.35

	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			nx := (float64(x) + 0.5 - cx) / rx
			ny := (float64(y) + 0.5 - cy) / ry
			if math.Abs(nx) > 1 {
				continue
			}
			lid := 1 - nx*nx // almond: tall in the middle, pointed at the ends
			open := lid * e.open
			if math.Abs(ny) > lid+0.12 {
				continue
			}
			if math.Abs(ny) > open {
				// skin over the eye; the closed lid shows as a line with lashes
				if e.open < 0.15 && math.Abs(ny) < 0.12 && math.Abs(nx) < 0.97 {
					v.Set(x, y, '=', 180)
					if int(float64(x)*0.7)%3 == 0 && y+1 < v.H {
						v.Set(x, y+1, '\'', 180)
					}
				}
				continue
			}
			if math.Abs(ny) > open-0.12 {
				v.Set(x, y, '~', 180) // lid edge
				continue
			}
			// iris and pupil, circular on screen
			dx := (float64(x) + 0.5 - ix) / 2
			dy := float64(y) + 0.5 - iy
			d := math.Hypot(dx, dy) / ri
			switch {
			case d < 0.38:
				v.Set(x, y, '@', 16+0)
			case d < 0.45:
				v.Set(x, y, '#', 22)
			case d < 1:
				rings := []byte("%*+*%o")
				v.Set(x, y, rings[int(d*12)%len(rings)], 34+uint8(d*3))
			case d < 1.08:
				v.Set(x, y, 'O', 28)
			default:
				v.Set(x, y, '.', 255) // white of the eye
			}
		}
	}
	// A highlight makes it look wet.
	v.Set(int(ix+ri*0.5), int(iy-ri*0.35), 'o', 231)
}
