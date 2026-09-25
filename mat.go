package main

import "math"

type Vec3 [3]float64

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a Vec3) Scale(k float64) Vec3 { return Vec3{a[0] * k, a[1] * k, a[2] * k} }
func (a Vec3) Dot(b Vec3) float64   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a Vec3) Len() float64         { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func (a Vec3) Norm() Vec3 {
	if l := a.Len(); l > 0 {
		return a.Scale(1 / l)
	}
	return a
}

// Mat3 is a 3x3 matrix, row-major.
type Mat3 [3]Vec3

func Identity() Mat3 { return Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} }

func (m Mat3) Apply(v Vec3) Vec3 { return Vec3{m[0].Dot(v), m[1].Dot(v), m[2].Dot(v)} }

func (m Mat3) T() Mat3 {
	return Mat3{
		{m[0][0], m[1][0], m[2][0]},
		{m[0][1], m[1][1], m[2][1]},
		{m[0][2], m[1][2], m[2][2]},
	}
}

func (m Mat3) Mul(n Mat3) Mat3 {
	t := n.T()
	var r Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			r[i][j] = m[i].Dot(t[j])
		}
	}
	return r
}

func RotX(a float64) Mat3 {
	c, s := math.Cos(a), math.Sin(a)
	return Mat3{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

func RotY(a float64) Mat3 {
	c, s := math.Cos(a), math.Sin(a)
	return Mat3{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
}

func RotZ(a float64) Mat3 {
	c, s := math.Cos(a), math.Sin(a)
	return Mat3{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
}

// Heading is where the phone points, in radians clockwise from north, given
// its device-to-world matrix. Flat on a table that is its top edge (device
// +y); held upright it is the back of the phone (device -z), which is where
// the top edge's horizontal direction becomes meaningless. The two are blended
// by how horizontal each one is, so the switch is smooth.
func Heading(R Mat3) float64 {
	top := R.Apply(Vec3{0, 1, 0})
	back := R.Apply(Vec3{0, 0, -1})
	wt := math.Hypot(top[0], top[1])
	wb := math.Hypot(back[0], back[1])
	e := top[0]*wt + back[0]*wb
	n := top[1]*wt + back[1]*wb
	return math.Atan2(e, n)
}

// FromRotationVector turns an Android rotation-vector reading (x, y, z[, w])
// into the matrix that takes device coordinates to world coordinates (x east,
// y north, z up), like SensorManager.getRotationMatrixFromVector.
func FromRotationVector(v []float64) (Mat3, bool) {
	if len(v) < 3 {
		return Identity(), false
	}
	x, y, z := v[0], v[1], v[2]
	var w float64
	if len(v) >= 4 {
		w = v[3]
	} else {
		w = math.Sqrt(math.Max(0, 1-x*x-y*y-z*z))
	}
	return Mat3{
		{1 - 2*y*y - 2*z*z, 2*x*y - 2*z*w, 2*x*z + 2*y*w},
		{2*x*y + 2*z*w, 1 - 2*x*x - 2*z*z, 2*y*z - 2*x*w},
		{2*x*z - 2*y*w, 2*y*z + 2*x*w, 1 - 2*x*x - 2*y*y},
	}, true
}
