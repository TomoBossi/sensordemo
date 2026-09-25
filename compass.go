package main

import (
	"fmt"
	"math"
)

func init() {
	register(entry{
		name: "compass",
		desc: "a compass rose that points to real north (rotation vector + magnetometer)",
		uses: []string{"magnetic_field"},
		new:  func(specs []string) Demo { return &compass{} },
	})
}

// compass draws a rose that turns so N points at magnetic north, with a fixed
// needle marking where the top of the phone points. Heading comes from the
// rotation vector (magnetometer + gyro + accelerometer fused), the same
// signal Google Maps uses for its direction cone.
type compass struct {
	has     bool
	heading float64 // smoothed, radians clockwise from north
}

func (c *compass) Setup(ss *Streams) ([]*Gauge, error) {
	if _, err := ss.Subscribe("rotation_vector", 30); err != nil {
		return nil, err
	}
	mag, err := ss.Subscribe("magnetic_field", 20)
	if err != nil {
		return nil, err
	}
	return gaugesFor(mag, 3), nil
}

func (c *compass) Help() []string {
	return []string{
		"The rose turns so N points north; the heading is where the phone's long axis points, however you roll or tilt it.",
		"Held upright, the long axis points at the sky, so the heading follows the back of the phone instead, like a camera.",
		"If it drifts, wave the phone in a figure 8 to recalibrate the magnetometer; magnets and metal nearby bend it too.",
	}
}

func (c *compass) Key(k byte) {}

var points = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

func (c *compass) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 12 || v.H < 8 {
		return
	}
	r := ss.Get("rotation_vector").Read()
	if R, ok := FromRotationVector(r.V); ok && r.OK {
		// The top of the phone (device +y) in world coordinates (x east,
		// y north); its angle from north is the heading.
		h := Heading(R.Mul(screenFrame(ss)))
		if !c.has {
			c.heading, c.has = h, true
		}
		d := math.Remainder(h-c.heading, 2*math.Pi) // shortest way round
		// Keep the smoothed angle in (-pi, pi]: it would otherwise
		// accumulate full turns and go negative.
		c.heading = math.Remainder(c.heading+d*math.Min(1, dt*8), 2*math.Pi)
	}
	cx, cy := float64(v.W)/2, float64(v.H)/2
	rad := math.Min(float64(v.W)/2-2, float64(v.H)-2) // in columns; rows are x2
	at := func(ang, rr float64) (int, int) {
		// ang: clockwise from screen-up, radians
		return int(cx + rr*math.Sin(ang) + 0.5), int(cy - rr*math.Cos(ang)/2 + 0.5)
	}

	// Rim: ticks every 5 degrees, longer every 45.
	for deg := 0; deg < 360; deg += 5 {
		a := float64(deg)*math.Pi/180 - c.heading
		ch, col := byte('.'), uint8(240)
		if deg%45 == 0 {
			ch, col = '+', 250
		} else if deg%15 == 0 {
			ch = ':'
		}
		x, y := at(a, rad)
		v.Set(x, y, ch, col)
	}
	// Cardinal and intercardinal labels just inside the rim.
	for i, p := range points {
		a := float64(i)*math.Pi/4 - c.heading
		x, y := at(a, rad*0.82)
		col := uint8(250)
		if p == "N" {
			col = 196
		}
		v.Text(x-len(p)/2, y, p, col)
	}
	// The north arm of the rose, kept out of the middle where the digits go.
	for k := 0.48; k < 0.7; k += 0.02 {
		x, y := at(-c.heading, rad*k)
		v.Set(x, y, '^', 196)
		x, y = at(math.Pi-c.heading, rad*k)
		v.Set(x, y, '=', 244)
	}
	// Fixed marker just outside the rim: where the phone points.
	x, y := at(0, rad*1.0)
	v.Set(x, y-1, 'V', 226)
	v.Set(x, y, '|', 226)

	// Heading in big digits in the middle.
	deg := math.Mod(math.Mod(math.Round(c.heading*180/math.Pi), 360)+360, 360) // 0..359
	text := fmt.Sprintf("%03.0f", deg)
	name := points[int(math.Mod(deg+22.5, 360)/45)%len(points)]
	s := max(1, fitScale(text, int(rad*0.8), int(rad*0.3)))
	bw, bh := bannerSize(text, s)
	drawBanner(v, text, int(cx)-bw/2, int(cy)-bh/2, s, '#', 226)
	v.Text(int(cx)-len(name)/2, int(cy)+bh/2+1, name, 250)
	if !c.has {
		v.Text(1, v.H-1, "waiting for rotation vector...", 244)
	}
}
