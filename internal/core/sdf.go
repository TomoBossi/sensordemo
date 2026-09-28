package core

import "math"

// Smin blends two distances smoothly over k (polynomial smooth minimum).
func Smin(a, b, k float64) float64 {
	h := Clamp01(0.5 + 0.5*(b-a)/k)
	return b + (a-b)*h - k*h*(1-h)
}

func Smax(a, b, k float64) float64 { return -Smin(-a, -b, k) }

func SkyDir(azDeg, elDeg float64) Vec3 {
	az, el := azDeg*math.Pi/180, elDeg*math.Pi/180
	return Vec3{math.Cos(el) * math.Sin(az), math.Cos(el) * math.Cos(az), math.Sin(el)}
}

func SdBox(q, b Vec3) float64 {
	dx, dy, dz := math.Abs(q[0])-b[0], math.Abs(q[1])-b[1], math.Abs(q[2])-b[2]
	out := math.Hypot(math.Hypot(math.Max(dx, 0), math.Max(dy, 0)), math.Max(dz, 0))
	return out + math.Min(math.Max(dx, math.Max(dy, dz)), 0)
}

func SdSphere(q Vec3, z, r float64) float64 {
	return math.Sqrt(q[0]*q[0]+q[1]*q[1]+(q[2]-z)*(q[2]-z)) - r
}

// CylSpan is the stretch of a ray inside a vertical cylinder (axis at x,
// z; radius r; heights y0..y1), as distances along it.
func CylSpan(o, d Vec3, x, z, r, y0, y1 float64) (float64, float64, bool) {
	ox, oz := o[0]-x, o[2]-z
	a := d[0]*d[0] + d[2]*d[2]
	b := 2 * (ox*d[0] + oz*d[2])
	cc := ox*ox + oz*oz - r*r
	tin, tout := math.Inf(-1), math.Inf(1)
	if a > 1e-12 {
		disc := b*b - 4*a*cc
		if disc < 0 {
			return 0, 0, false
		}
		sq := math.Sqrt(disc)
		tin, tout = (-b-sq)/(2*a), (-b+sq)/(2*a)
	} else if cc > 0 {
		return 0, 0, false
	}
	if d[1] != 0 {
		ta, tb := (y0-o[1])/d[1], (y1-o[1])/d[1]
		if ta > tb {
			ta, tb = tb, ta
		}
		tin, tout = math.Max(tin, ta), math.Min(tout, tb)
	} else if o[1] < y0 || o[1] > y1 {
		return 0, 0, false
	}
	tin = math.Max(tin, 0)
	return tin, tout, tin < tout
}

// RotAxis is the rotation by angle about the unit axis a (Rodrigues).
func RotAxis(a Vec3, angle float64) Mat3 {
	c, s := math.Cos(angle), math.Sin(angle)
	k := 1 - c
	return Mat3{
		{c + a[0]*a[0]*k, a[0]*a[1]*k - a[2]*s, a[0]*a[2]*k + a[1]*s},
		{a[1]*a[0]*k + a[2]*s, c + a[1]*a[1]*k, a[1]*a[2]*k - a[0]*s},
		{a[2]*a[0]*k - a[1]*s, a[2]*a[1]*k + a[0]*s, c + a[2]*a[2]*k},
	}
}
