package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "eye",
		desc: "an eye that shuts when you cover the proximity sensor, looks where you tilt, and reacts to light",
		uses: []string{"proximity", "light"},
		new:  func(specs []string) Demo { return &eye{open: 1, rng: rand.New(rand.NewSource(7))} },
	})
}

// eye draws an almond eye with an iris. Covering the proximity sensor (top of
// the screen, near the earpiece) closes it; tilting the phone moves the pupil
// toward the low side; it blinks now and then. Light brightens it, shrinks the
// pupil and grows the highlight, and direct sunlight makes it squint.
type eye struct {
	open      float64 // 0 closed .. 1 open, animated
	lookX     float64
	lookY     float64
	light     float64 // smoothed brightness, 0 dark .. 1 blinding
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
	if l, err := ss.Subscribe("light", 0); err == nil {
		g = append(g, gaugesFor(l, 1)...)
	}
	if a, err := ss.Subscribe("accelerometer", 30); err == nil {
		g = append(g, gaugesFor(a, 2)...)
	}
	return g, nil
}

func (e *eye) Help() []string {
	return []string{
		"Cover the top of the phone (near the earpiece) and the eye closes; tilt the phone and it looks toward the low side. It blinks on its own too.",
		"Light: brighter light lights up the eye, shrinks its pupil and grows the glint; in direct sun it squints.",
		"space  blink",
	}
}

func (e *eye) Key(k byte) {
	if k == ' ' {
		e.nextBlink = 0
	}
}

// Brightness ramps, dark to bright.
var (
	scleraCol = []uint8{236, 238, 240, 243, 246, 249, 251, 253, 255}
	irisCol   = []uint8{22, 22, 28, 29, 35, 36, 42, 43, 49}
	lidCol    = []uint8{95, 131, 137, 173, 180}
)

func rampAt(ramp []uint8, f float64) uint8 {
	return ramp[min(int(math.Max(0, f)*float64(len(ramp))), len(ramp)-1)]
}

func (e *eye) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 6 {
		return
	}
	// Proximity is binary on this phone: 0 (near) or its maximum (far).
	near := false
	if r := ss.Get("proximity").Read(); r.OK && len(r.V) > 0 {
		near = r.V[0] < 1
	}
	target := 0.5 // no light sensor: medium
	if l := ss.Get("light"); l != nil {
		if r := l.Read(); r.OK && len(r.V) > 0 {
			target = r.V[0] / (r.V[0] + 300)
		}
	}
	e.light += (target - e.light) * math.Min(1, dt*3)

	if t >= e.nextBlink {
		e.blinkEnd = t + 0.15
		e.nextBlink = t + 2.5 + e.rng.Float64()*4
	}
	// Squint from about 3000 lux (lux/(lux+300) = 0.9) up.
	full := 1 - math.Max(0, (e.light-0.88)/0.12)*0.55
	want := full
	if near || t < e.blinkEnd {
		want = 0
	}
	e.open += (want - e.open) * math.Min(1, dt*18)

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
	ry := math.Max(rx*0.42/2, 2)                        // half-height in rows

	if e.open < 0.12 {
		e.drawClosed(v, cx, cy, rx)
		return
	}

	ri := ry * 0.95 // iris radius in rows
	ix := cx + e.lookX*rx*0.45
	iy := cy + e.lookY*ry*0.35
	pupil := 0.5 - 0.28*e.light // dilated in the dark, small in bright light
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			nx := (float64(x) + 0.5 - cx) / rx
			ny := (float64(y) + 0.5 - cy) / ry
			if math.Abs(nx) > 1 {
				continue
			}
			open := (1 - nx*nx) * e.open // almond: tall in the middle, pointed at the ends
			edge := 0.5 / ry             // the lid edge is about one row thick
			switch {
			case math.Abs(ny) > open+edge:
				continue
			case math.Abs(ny) > open:
				v.Set(x, y, '~', rampAt(lidCol, e.light)) // lid edge
				continue
			}
			// iris and pupil, circular on screen
			dx := (float64(x) + 0.5 - ix) / 2
			dy := float64(y) + 0.5 - iy
			d := math.Hypot(dx, dy) / ri
			switch {
			case d < pupil:
				v.Set(x, y, '@', 16)
			case d < pupil+0.07:
				v.Set(x, y, '#', rampAt(irisCol, e.light*0.6))
			case d < 1:
				rings := []byte("%*+*%o")
				v.Set(x, y, rings[int(d*12)%len(rings)], rampAt(irisCol, e.light*0.8+d*0.2))
			case d < 1.08:
				v.Set(x, y, 'O', rampAt(irisCol, e.light*0.5))
			default:
				v.Set(x, y, '.', rampAt(scleraCol, e.light)) // white of the eye
			}
		}
	}

	// The glint: none in the dark, growing with light. Drawn only where the
	// eye is open, so a closing lid hides it.
	if e.light < 0.08 {
		return
	}
	hx, hy := ix+ri*0.55, iy-ri*0.35
	size := e.light * ri * 0.35
	glint := []byte(".oO@")
	for y := int(hy - size); y <= int(hy+size); y++ {
		for x := int(hx - size*2); x <= int(hx+size*2); x++ {
			d := math.Hypot((float64(x)-hx)/2, float64(y)-hy)
			if d > math.Max(size, 0.5) {
				continue
			}
			nx := (float64(x) + 0.5 - cx) / rx
			ny := (float64(y) + 0.5 - cy) / ry
			if math.Abs(nx) > 1 || math.Abs(ny) > (1-nx*nx)*e.open {
				continue // under the lid
			}
			k := min(int(e.light*float64(len(glint))), len(glint)-1)
			if d > size*0.6 {
				k = max(k-1, 0)
			}
			v.Set(x, y, glint[k], rampAt(scleraCol, 0.6+e.light*0.4))
		}
	}
}

// drawClosed draws a shut eye: one curved lid line with lashes below it,
// the same at any size or orientation.
func (e *eye) drawClosed(v *View, cx, cy, rx float64) {
	col := rampAt(lidCol, e.light)
	for x := int(cx - rx); x <= int(cx+rx); x++ {
		nx := (float64(x) + 0.5 - cx) / rx
		if math.Abs(nx) > 1 {
			continue
		}
		y := int(cy + (1-nx*nx)*0.8) // a gentle downward curve
		c := byte('-')
		if math.Abs(nx) < 0.85 {
			c = '='
		}
		v.Set(x, y, c, col)
		if math.Abs(nx) < 0.8 && x%3 == 0 {
			lash := byte('|')
			if nx < -0.3 {
				lash = '/'
			} else if nx > 0.3 {
				lash = '\\'
			}
			v.Set(x, y+1, lash, col)
		}
	}
}
