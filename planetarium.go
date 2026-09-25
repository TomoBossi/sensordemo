package main

import (
	"fmt"
	"math"
	"time"
)

func init() {
	register(entry{
		name: "planetarium",
		desc: "point the phone at the sky: the real stars, constellations and planets in that direction, now",
		uses: []string{"rotation_vector", "location"},
		new:  func(specs []string) Demo { return &planetarium{lines: true, names: true, speed: 1} },
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

var lapseSpeeds = []float64{1, 60, 600, 3600}

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
	for i < len(lapseSpeeds) && lapseSpeeds[i] < p.speed {
		i++
	}
	switch k {
	case '+', '=':
		p.speed = lapseSpeeds[min(i+1, len(lapseSpeeds)-1)]
	case '-', '_':
		p.speed = lapseSpeeds[max(i-1, 0)]
		if p.speed == 1 {
			p.simTime = time.Now() // back to real time
		}
	case 'l':
		p.lines = !p.lines
	case 'n':
		p.names = !p.names
	}
}

// julian is the Julian date of t.
func julian(t time.Time) float64 {
	return float64(t.UnixNano())/86400e9 + 2440587.5
}

// skyVector turns equatorial coordinates (degrees) into a unit vector east, north,
// up for an observer at latitude lat, given the local sidereal time lst.
func skyVector(raDeg, decDeg, lat, lst float64) Vec3 {
	rad := math.Pi / 180
	h := (lst - raDeg) * rad
	d, phi := decDeg*rad, lat*rad
	return Vec3{
		-math.Cos(d) * math.Sin(h),
		math.Cos(phi)*math.Sin(d) - math.Sin(phi)*math.Cos(d)*math.Cos(h),
		math.Sin(phi)*math.Sin(d) + math.Cos(phi)*math.Cos(d)*math.Cos(h),
	}
}

// Keplerian elements for approximate planet positions (JPL, valid
// 1800-2050): a (au), e, I, L, long. of perihelion, long. of node (deg), and
// their rates per Julian century.
var planetElements = []struct {
	name  string
	c     byte
	col   uint8
	el    [6]float64
	rates [6]float64
}{
	{"Mercury", 'o', 250, [6]float64{0.38709927, 0.20563593, 7.00497902, 252.25032350, 77.45779628, 48.33076593},
		[6]float64{0.00000037, 0.00001906, -0.00594749, 149472.67411175, 0.16047689, -0.12534081}},
	{"Venus", '@', 230, [6]float64{0.72333566, 0.00677672, 3.39467605, 181.97909950, 131.60246718, 76.67984255},
		[6]float64{0.00000390, -0.00004107, -0.00078890, 58517.81538729, 0.00268329, -0.27769418}},
	{"Earth", 0, 0, [6]float64{1.00000261, 0.01671123, -0.00001531, 100.46457166, 102.93768193, 0},
		[6]float64{0.00000562, -0.00004392, -0.01294668, 35999.37244981, 0.32327364, 0}},
	{"Mars", 'o', 203, [6]float64{1.52371034, 0.09339410, 1.84969142, -4.55343205, -23.94362959, 49.55953891},
		[6]float64{0.00001847, 0.00007882, -0.00813131, 19140.30268499, 0.44441088, -0.29257343}},
	{"Jupiter", 'O', 223, [6]float64{5.20288700, 0.04838624, 1.30439695, 34.39644051, 14.72847983, 100.47390909},
		[6]float64{-0.00011607, -0.00013253, -0.00183714, 3034.74612775, 0.21252668, 0.20469106}},
	{"Saturn", 'O', 186, [6]float64{9.53667594, 0.05386179, 2.48599187, 49.95424423, 92.59887831, 113.66242448},
		[6]float64{-0.00125060, -0.00050991, 0.00193609, 1222.49362201, -0.41897216, -0.28867794}},
}

// heliocentric ecliptic position (au) of planet i at Julian date jd.
func heliocentric(i int, jd float64) Vec3 {
	T := (jd - 2451545) / 36525
	var e [6]float64
	for k := range e {
		e[k] = planetElements[i].el[k] + planetElements[i].rates[k]*T
	}
	rad := math.Pi / 180
	a, ecc, I := e[0], e[1], e[2]*rad
	w := (e[4] - e[5]) * rad
	node := e[5] * rad
	M := math.Remainder(e[3]-e[4], 360) * rad
	E := M
	for k := 0; k < 8; k++ {
		E -= (E - ecc*math.Sin(E) - M) / (1 - ecc*math.Cos(E))
	}
	xp, yp := a*(math.Cos(E)-ecc), a*math.Sqrt(1-ecc*ecc)*math.Sin(E)
	cw, sw, cn, sn, ci, si := math.Cos(w), math.Sin(w), math.Cos(node), math.Sin(node), math.Cos(I), math.Sin(I)
	return Vec3{
		(cw*cn-sw*sn*ci)*xp + (-sw*cn-cw*sn*ci)*yp,
		(cw*sn+sw*cn*ci)*xp + (-sw*sn+cw*cn*ci)*yp,
		sw*si*xp + cw*si*yp,
	}
}

// eclipticToRADec converts an ecliptic vector to right ascension and
// declination in degrees.
func eclipticToRADec(v Vec3) (float64, float64) {
	eps := 23.43928 * math.Pi / 180
	x, y, z := v[0], v[1]*math.Cos(eps)-v[2]*math.Sin(eps), v[1]*math.Sin(eps)+v[2]*math.Cos(eps)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360), math.Asin(z/v.Len()) * 180 / math.Pi
}

// moonPosition is the Moon's approximate RA/Dec (about a degree) and its
// phase angle's illuminated fraction.
func moonPosition(jd float64) (ra, dec, lit float64) {
	d := jd - 2451545
	rad := math.Pi / 180
	L := 218.316 + 13.176396*d
	M := (134.963 + 13.064993*d) * rad
	F := (93.272 + 13.229350*d) * rad
	lon := (L + 6.289*math.Sin(M)) * rad
	lat := 5.128 * math.Sin(F) * rad
	v := Vec3{math.Cos(lat) * math.Cos(lon), math.Cos(lat) * math.Sin(lon), math.Sin(lat)}
	ra, dec = eclipticToRADec(v)
	// elongation from the Sun sets the phase
	sun := heliocentric(2, jd).Scale(-1)
	cosE := v.Dot(sun.Norm())
	return ra, dec, (1 - cosE) / 2
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
	if d, ok := declination(ss); ok {
		decl = d
	}

	// Simulated time: real time, or time-lapse.
	now := time.Now()
	if p.simTime.IsZero() {
		p.simTime, p.last = now, now
	}
	p.simTime = p.simTime.Add(time.Duration(float64(now.Sub(p.last)) * p.speed))
	p.last = now
	jd := julian(p.simTime)
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545), 360)
	lst := gmst + lon

	if rr := ss.Get("rotation_vector").Read(); rr.OK {
		if R, ok := FromRotationVector(rr.V); ok {
			R = R.Mul(screenFrame(ss))
			if !p.hasPose {
				p.pose, p.hasPose = R, true
			}
			p.pose = blendRotation(p.pose, R, math.Min(1, dt*12))
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
	parallelRows(v.H, func(y int) {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * fovX
			w := (1 - 2*(float64(y)+0.5)/float64(v.H)) * fovY
			d := p.pose.Apply(Vec3{u, w, -1}.Norm())
			if d[2] < 0 && cellNoise(x, y) < 0.05 {
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
				x0, y0, ok0 := project(skyVector(float64(strip[k]), float64(strip[k+1]), lat, lst))
				x1, y1, ok1 := project(skyVector(float64(strip[k+2]), float64(strip[k+3]), lat, lst))
				if !ok0 || !ok1 {
					continue
				}
				n := max(abs(x1-x0), abs(y1-y0))
				if n > v.W { // wrapped around behind: skip
					continue
				}
				c := lineChar(float64(x1-x0), float64(y1-y0))
				for s := 1; s < n; s++ {
					f := float64(s) / float64(n)
					v.Set(x0+int(float64(x1-x0)*f+0.5), y0+int(float64(y1-y0)*f+0.5), c, 24)
				}
			}
		}
	}
	if p.names {
		for _, c := range catalogConstellations {
			if x, y, ok := project(skyVector(float64(c.RA), float64(c.Dec), lat, lst)); ok {
				v.Text(x-len(c.Name)/2, y, c.Name, 60)
			}
		}
	}

	// Stars, brightest drawn last so they win.
	for i := len(catalogStars)/4 - 1; i >= 0; i-- {
		ra, dec, mag, bv := catalogStars[i*4], catalogStars[i*4+1], catalogStars[i*4+2], catalogStars[i*4+3]
		x, y, ok := project(skyVector(float64(ra), float64(dec), lat, lst))
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
			if x, y, ok := project(skyVector(float64(s.RA), float64(s.Dec), lat, lst)); ok {
				v.Text(x+2, y, s.Name, 250)
			}
		}
	}

	// Planets, Sun and Moon.
	earth := heliocentric(2, jd)
	for i, pl := range planetElements {
		if i == 2 {
			continue
		}
		ra, dec := eclipticToRADec(heliocentric(i, jd).Sub(earth))
		if x, y, ok := project(skyVector(ra, dec, lat, lst)); ok {
			v.Set(x, y, pl.c, pl.col)
			if p.names {
				v.Text(x+2, y, pl.name, pl.col)
			}
		}
	}
	sra, sdec := eclipticToRADec(earth.Scale(-1))
	if x, y, ok := project(skyVector(sra, sdec, lat, lst)); ok {
		v.Text(x-1, y, "(@)", 226)
		if p.names {
			v.Text(x+3, y, "Sun", 226)
		}
	}
	mra, mdec, lit := moonPosition(jd)
	if x, y, ok := project(skyVector(mra, mdec, lat, lst)); ok {
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
