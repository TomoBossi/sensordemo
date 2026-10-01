package battery

import "math"

// fluid is the charge's surface as shallow water: a height field over the
// tube's round floor, the water's velocity on the faces between its cells,
// and walls it can't cross. Shaking the tube pushes the water against the
// walls (in the tube's frame, its acceleration is a force the other way),
// so it piles up, sloshes back, and its waves cross and reflect; pushed up
// or down, its gravity grows or weakens for a moment. It keeps its volume.
//
// Units are the scene's: the tube's inner radius is innerR, x and z across
// it; h is how far the surface stands above its resting level.
type fluid struct {
	h    [fluidN * fluidN]float64
	u    [(fluidN + 1) * fluidN]float64 // x velocity, on the faces between cells in x
	w    [fluidN * (fluidN + 1)]float64 // z velocity, between cells in z
	wet  [fluidN * fluidN]bool          // inside the tube
	push [3]float64                     // the acceleration, smoothed: a phone's is jittery
	init bool
}

const (
	fluidN    = 30
	fluidDx   = 2 * innerR / fluidN
	fluidG    = 9.8  // gravity, in scene units: slopes against a push are a/g
	fluidH    = 0.35 // the depth the waves feel: their speed is sqrt(g H), about 1.9
	fluidDamp = 0.6  // how fast sloshing dies down, per second
	// Viscosity, in scene units squared per second: a wave of wavenumber k
	// dies at fluidVisc k² per second, so ripples a few cells long vanish
	// in a tenth of a second while the whole surface's slosh lingers.
	fluidVisc = 0.012
	fluidMaxH = 0.75 // the surface never rises or falls further than this
)

func (f *fluid) setup() {
	for j := 0; j < fluidN; j++ {
		for i := 0; i < fluidN; i++ {
			x, z := f.center(i, j)
			f.wet[j*fluidN+i] = math.Hypot(x, z) < innerR-fluidDx*0.3
		}
	}
	f.init = true
}

func (f *fluid) center(i, j int) (float64, float64) {
	return -innerR + (float64(i)+0.5)*fluidDx, -innerR + (float64(j)+0.5)*fluidDx
}

func (f *fluid) wetAt(i, j int) bool {
	return i >= 0 && j >= 0 && i < fluidN && j < fluidN && f.wet[j*fluidN+i]
}

// step advances the water by dt under the tube's acceleration a (scene
// frame, m/s²; y up).
func (f *fluid) step(a [3]float64, dt float64) {
	if !f.init {
		f.setup()
	}
	for k := range a {
		f.push[k] += (a[k] - f.push[k]) * math.Min(1, dt/0.07)
	}
	a = f.push
	// Stable steps: waves must not cross more than half a cell per step.
	c := math.Sqrt(fluidG * fluidH * 2)
	n := int(math.Ceil(dt / (0.4 * fluidDx / c)))
	h := dt / float64(n)
	g := fluidG * math.Max(0.2, math.Min(2.5, 1+a[1]/9.8)) // pushed up, it weighs more
	damp := math.Exp(-fluidDamp * h)
	for k := 0; k < n; k++ {
		// Velocities: the surface's slope and the push drive them; none
		// crosses a wall.
		for j := 0; j < fluidN; j++ {
			for i := 1; i < fluidN; i++ {
				idx := j*(fluidN+1) + i
				if !f.wetAt(i-1, j) || !f.wetAt(i, j) {
					f.u[idx] = 0
					continue
				}
				f.u[idx] = (f.u[idx] - h*(g*(f.h[j*fluidN+i]-f.h[j*fluidN+i-1])/fluidDx+a[0])) * damp
			}
		}
		for j := 1; j < fluidN; j++ {
			for i := 0; i < fluidN; i++ {
				idx := j*fluidN + i
				if !f.wetAt(i, j-1) || !f.wetAt(i, j) {
					f.w[idx] = 0
					continue
				}
				f.w[idx] = (f.w[idx] - h*(g*(f.h[j*fluidN+i]-f.h[(j-1)*fluidN+i])/fluidDx+a[2])) * damp
			}
		}
		f.smooth(f.u[:], fluidN+1, fluidN, h)
		f.smooth(f.w[:], fluidN, fluidN+1, h)
		f.walls()
		// Heights: what flows in raises the surface.
		for j := 0; j < fluidN; j++ {
			for i := 0; i < fluidN; i++ {
				if !f.wet[j*fluidN+i] {
					continue
				}
				div := (f.u[j*(fluidN+1)+i+1] - f.u[j*(fluidN+1)+i] + f.w[(j+1)*fluidN+i] - f.w[j*fluidN+i]) / fluidDx
				f.h[j*fluidN+i] -= h * fluidH * div
			}
		}
	}
	for i := range f.h {
		f.h[i] = math.Max(-fluidMaxH, math.Min(fluidMaxH, f.h[i]))
	}
}

// walls stops the water at the tube's wall: no velocity on a face with dry
// ground on either side.
func (f *fluid) walls() {
	for j := 0; j < fluidN; j++ {
		for i := 0; i <= fluidN; i++ {
			if !f.wetAt(i-1, j) || !f.wetAt(i, j) {
				f.u[j*(fluidN+1)+i] = 0
			}
		}
	}
	for j := 0; j <= fluidN; j++ {
		for i := 0; i < fluidN; i++ {
			if !f.wetAt(i, j-1) || !f.wetAt(i, j) {
				f.w[j*fluidN+i] = 0
			}
		}
	}
}

// smooth diffuses a velocity grid (w wide, n tall) by the viscosity over
// dt: each value moves toward its neighbors' mean, a missing neighbor
// counting as the value itself. The water drags along the walls.
func (f *fluid) smooth(g []float64, w, n int, dt float64) {
	k := fluidVisc * dt / (fluidDx * fluidDx)
	tmp := make([]float64, len(g))
	copy(tmp, g)
	for j := 0; j < n; j++ {
		for i := 0; i < w; i++ {
			c := tmp[j*w+i]
			sum := 0.0
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				ii, jj := i+d[0], j+d[1]
				if ii < 0 || jj < 0 || ii >= w || jj >= n {
					sum += c
					continue
				}
				sum += tmp[jj*w+ii]
			}
			g[j*w+i] = c + k*(sum-4*c)
		}
	}
}

// at is the surface's height above rest at x, z, blending the cells
// around it (a dry cell, past the wall, counts as its wet neighbor).
func (f *fluid) at(x, z float64) float64 {
	if !f.init {
		return 0
	}
	gx := (x+innerR)/fluidDx - 0.5
	gz := (z+innerR)/fluidDx - 0.5
	i0, j0 := int(math.Floor(gx)), int(math.Floor(gz))
	fx, fz := gx-float64(i0), gz-float64(j0)
	cell := func(i, j int) (float64, bool) {
		i = max(0, min(fluidN-1, i))
		j = max(0, min(fluidN-1, j))
		return f.h[j*fluidN+i], f.wet[j*fluidN+i]
	}
	sum, wt := 0.0, 0.0
	for _, c := range [4]struct {
		di, dj int
		k      float64
	}{{0, 0, (1 - fx) * (1 - fz)}, {1, 0, fx * (1 - fz)}, {0, 1, (1 - fx) * fz}, {1, 1, fx * fz}} {
		if v, ok := cell(i0+c.di, j0+c.dj); ok {
			sum += v * c.k
			wt += c.k
		}
	}
	if wt == 0 {
		return 0
	}
	return sum / wt
}

// slope is the surface's gradient at x, z.
func (f *fluid) slope(x, z float64) (float64, float64) {
	const e = fluidDx
	return (f.at(x+e, z) - f.at(x-e, z)) / (2 * e), (f.at(x, z+e) - f.at(x, z-e)) / (2 * e)
}

// energy is how much the water is moving: its kinetic and potential energy,
// for tests.
func (f *fluid) energy() float64 {
	e := 0.0
	for _, v := range f.u {
		e += v * v
	}
	for _, v := range f.w {
		e += v * v
	}
	for i, v := range f.h {
		if f.wet[i] {
			e += fluidG / fluidH * v * v
		}
	}
	return e
}
