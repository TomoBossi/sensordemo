package planetarium

import (
	"fmt"
	"math"
	"time"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

func init() {
	Register(Entry{
		Name: "planetarium",
		Desc: "point the phone at the sky: the real stars, constellations and planets in that direction, now",
		Uses: []string{"rotation_vector", "location"},
		New:  func(specs []string) Demo { return &planetarium{lines: true, names: true, speed: 1} },
	})
}

// planetarium shows the sky behind the phone, like a window: each star's
// position comes from its catalog coordinates, your location and the local
// sidereal time; the planets, Sun and Moon from approximate orbital elements.
// The rotation vector orients the view; the declination from sensord's
// location turns true north into the sensor's magnetic north.
type planetarium struct {
	lines, names bool
	speed        float64 // time-lapse factor
	simTime      time.Time
	last         time.Time
	pose         Mat3
	hasPose      bool
}

func (p *planetarium) Setup(ss *Streams) ([]*Gauge, error) {
	if _, err := ss.Subscribe("location", 0.1); err != nil {
		return nil, err
	}
	if _, err := ss.Subscribe("rotation_vector", 30); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *planetarium) Help() []string {
	return []string{
		"Hold the phone up to the sky: you see the stars, constellations and planets that are really in that direction right now, even through the ground or in daylight.",
		"Star colors follow their temperature (blue-white hot to orange-red cool); size follows brightness.",
		"+ -  time-lapse (1x, 60x, 600x, 3600x)    l  constellation lines    n  names",
	}
}

func (p *planetarium) Key(k byte) {
	i := 0
	for i < len(LapseSpeeds) && LapseSpeeds[i] < p.speed {
		i++
	}
	switch k {
	case '+', '=':
		p.speed = LapseSpeeds[min(i+1, len(LapseSpeeds)-1)]
	case '-', '_':
		p.speed = LapseSpeeds[max(i-1, 0)]
		if p.speed == 1 {
			p.simTime = Clock() // back to real time
		}
	case 'l':
		p.lines = !p.lines
	case 'n':
		p.names = !p.names
	}
}

// starColor maps a B-V color index to a terminal color: blue-white for hot
// stars through white and yellow to orange-red for cool ones.
func starColor(bv float32) uint8 {
	switch {
	case bv < -0.1:
		return 153
	case bv < 0.15:
		return 195
	case bv < 0.45:
		return 231
	case bv < 0.7:
		return 230
	case bv < 1.0:
		return 223
	case bv < 1.4:
		return 216
	}
	return 209
}

func (p *planetarium) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 8 {
		return
	}
	lr := ss.Get("location").Read()
	if !lr.OK || len(lr.V) < 2 {
		msg := "waiting for a location fix (needed to know your sky)..."
		v.Text(max(0, (v.W-len(msg))/2), v.H/2, msg, 244)
		return
	}
	lat, lon := lr.V[0], lr.V[1]
	decl := 0.0
	if d, ok := Declination(ss); ok {
		decl = d
	}

	// Simulated time: real time, or time-lapse.
	now := Clock()
	if p.simTime.IsZero() {
		p.simTime, p.last = now, now
	}
	p.simTime = p.simTime.Add(time.Duration(float64(now.Sub(p.last)) * p.speed))
	p.last = now
	jd := Julian(p.simTime)
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545), 360)
	lst := gmst + lon

	if rr := ss.Get("rotation_vector").Read(); rr.OK {
		if R, ok := FromRotationVector(rr.V); ok {
			R = R.Mul(ScreenFrame(ss))
			if !p.hasPose {
				p.pose, p.hasPose = R, true
			}
			p.pose = BlendRotation(p.pose, R, math.Min(1, dt*12))
		}
	}
	if !p.hasPose {
		msg := "waiting for the orientation..."
		v.Text(max(0, (v.W-len(msg))/2), v.H/2, msg, 244)
		return
	}
	// True-north sky vectors into the sensor's magnetic frame.
	toMag := RotZ(decl * math.Pi / 180)
	Rt := p.pose.T()
	fovX := math.Tan(34 * math.Pi / 180)
	aspect := float64(2*v.H) / float64(v.W)
	fovY := fovX * aspect
	project := func(d Vec3) (int, int, bool) {
		c := Rt.Apply(toMag.Apply(d))
		if c[2] > -0.05 {
			return 0, 0, false // behind the phone
		}
		x := (c[0]/(-c[2])/fovX + 1) / 2 * float64(v.W)
		y := (1 - c[1]/(-c[2])/fovY) / 2 * float64(v.H)
		return int(x), int(y), x > -1 && y > -1 && x < float64(v.W)+1 && y < float64(v.H)+1
	}

	// Ground: below the horizon the view is earth-toned and dim.
	ParallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * fovX
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * fovY
			d := p.pose.Apply(Vec3{u, w, -1}.Norm())
			if d[2] < 0 && CellNoise(x, y) < 0.05 {
				v.Set(x, y, '.', 236)
			}
		}
	})
	// Horizon line and cardinal points (true directions).
	for az := 0.0; az < 360; az += 0.5 {
		rad := az * math.Pi / 180
		if x, y, ok := project(Vec3{math.Sin(rad), math.Cos(rad), 0}); ok {
			v.Set(x, y, '-', 58)
		}
	}
	for i, n := range []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"} {
		rad := float64(i) * math.Pi / 4
		if x, y, ok := project(Vec3{math.Sin(rad), math.Cos(rad), 0}); ok {
			v.Text(x-len(n)/2, y, n, map[bool]uint8{true: 196, false: 144}[n == "N"])
		}
	}

	// Constellation stick figures.
	if p.lines {
		for _, strip := range catalogLines {
			for k := 0; k+3 < len(strip); k += 2 {
				x0, y0, ok0 := project(SkyVector(float64(strip[k]), float64(strip[k+1]), lat, lst))
				x1, y1, ok1 := project(SkyVector(float64(strip[k+2]), float64(strip[k+3]), lat, lst))
				if !ok0 || !ok1 {
					continue
				}
				n := max(Abs(x1-x0), Abs(y1-y0))
				if n > v.W { // wrapped around behind: skip
					continue
				}
				c := LineChar(float64(x1-x0), float64(y1-y0))
				for s := 1; s < n; s++ {
					f := float64(s) / float64(n)
					v.Set(x0+int(float64(x1-x0)*f+0.5), y0+int(float64(y1-y0)*f+0.5), c, 24)
				}
			}
		}
	}
	if p.names {
		for _, c := range catalogConstellations {
			if x, y, ok := project(SkyVector(float64(c.RA), float64(c.Dec), lat, lst)); ok {
				v.Text(x-len(c.Name)/2, y, c.Name, 60)
			}
		}
	}

	// Stars, brightest drawn last so they win.
	for i := len(catalogStars)/4 - 1; i >= 0; i-- {
		ra, dec, mag, bv := catalogStars[i*4], catalogStars[i*4+1], catalogStars[i*4+2], catalogStars[i*4+3]
		x, y, ok := project(SkyVector(float64(ra), float64(dec), lat, lst))
		if !ok {
			continue
		}
		c := byte('.')
		switch {
		case mag < 0.5:
			c = '@'
		case mag < 1.5:
			c = '*'
		case mag < 2.5:
			c = '+'
		case mag < 3.5:
			c = ':'
		}
		v.Set(x, y, c, starColor(bv))
	}
	if p.names {
		for _, s := range catalogNames {
			if x, y, ok := project(SkyVector(float64(s.RA), float64(s.Dec), lat, lst)); ok {
				v.Text(x+2, y, s.Name, 250)
			}
		}
	}

	// Planets, Sun and Moon.
	earth := Heliocentric(2, jd)
	for i, pl := range PlanetElements {
		if i == 2 {
			continue
		}
		ra, dec := EclipticToRADec(Heliocentric(i, jd).Sub(earth))
		if x, y, ok := project(SkyVector(ra, dec, lat, lst)); ok {
			v.Set(x, y, pl.C, pl.Col)
			if p.names {
				v.Text(x+2, y, pl.Name, pl.Col)
			}
		}
	}
	sra, sdec := EclipticToRADec(earth.Scale(-1))
	if x, y, ok := project(SkyVector(sra, sdec, lat, lst)); ok {
		v.Text(x-1, y, "(@)", 226)
		if p.names {
			v.Text(x+3, y, "Sun", 226)
		}
	}
	mra, mdec, lit := MoonPosition(jd)
	if x, y, ok := project(SkyVector(mra, mdec, lat, lst)); ok {
		c := "(" + string("  .oO@"[min(int(lit*6), 5)]) + ")"
		v.Text(x-1, y, c, 253)
		if p.names {
			v.Text(x+3, y, fmt.Sprintf("Moon %.0f%%", lit*100), 253)
		}
	}

	// Where the view points, and the time.
	center := p.pose.Apply(Vec3{0, 0, -1})
	c := RotZ(-decl * math.Pi / 180).Apply(center) // back to true north
	az := math.Mod(math.Atan2(c[0], c[1])*180/math.Pi+360, 360)
	alt := math.Asin(math.Max(-1, math.Min(1, c[2]))) * 180 / math.Pi
	status := fmt.Sprintf("az %03.0f  alt %+03.0f  %s", az, alt, p.simTime.Local().Format("15:04"))
	if p.speed > 1 {
		status += fmt.Sprintf("  x%.0f", p.speed)
	}
	v.Text(max(0, (v.W-len(status))/2), v.H-1, status, 244)
	v.Text(v.W/2, v.H/2, "+", 238)
}
