package core

import (
	"math"
	"time"
)

var LapseSpeeds = []float64{1, 60, 600, 3600}

// Julian is the Julian date of t.
func Julian(t time.Time) float64 {
	return float64(t.UnixNano())/86400e9 + 2440587.5
}

// SkyVector turns equatorial coordinates (degrees) into a unit vector east, north,
// up for an observer at latitude lat, given the local sidereal time lst.
func SkyVector(raDeg, decDeg, lat, lst float64) Vec3 {
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
var PlanetElements = []struct {
	Name  string
	C     byte
	Col   uint8
	El    [6]float64
	Rates [6]float64
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

// Heliocentric ecliptic position (au) of planet i at Julian date jd.
func Heliocentric(i int, jd float64) Vec3 {
	T := (jd - 2451545) / 36525
	var e [6]float64
	for k := range e {
		e[k] = PlanetElements[i].El[k] + PlanetElements[i].Rates[k]*T
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

// EclipticToRADec converts an ecliptic vector to right ascension and
// Declination in degrees.
func EclipticToRADec(v Vec3) (float64, float64) {
	eps := 23.43928 * math.Pi / 180
	x, y, z := v[0], v[1]*math.Cos(eps)-v[2]*math.Sin(eps), v[1]*math.Sin(eps)+v[2]*math.Cos(eps)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360), math.Asin(z/v.Len()) * 180 / math.Pi
}

// MoonPosition is the Moon's approximate RA/Dec (about a degree) and its
// phase angle's illuminated fraction.
func MoonPosition(jd float64) (ra, dec, lit float64) {
	d := jd - 2451545
	rad := math.Pi / 180
	L := 218.316 + 13.176396*d
	M := (134.963 + 13.064993*d) * rad
	F := (93.272 + 13.229350*d) * rad
	lon := (L + 6.289*math.Sin(M)) * rad
	lat := 5.128 * math.Sin(F) * rad
	v := Vec3{math.Cos(lat) * math.Cos(lon), math.Cos(lat) * math.Sin(lon), math.Sin(lat)}
	ra, dec = EclipticToRADec(v)
	// elongation from the Sun sets the phase
	sun := Heliocentric(2, jd).Scale(-1)
	cosE := v.Dot(sun.Norm())
	return ra, dec, (1 - cosE) / 2
}
