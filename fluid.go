package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "fluid",
		desc: "an ASCII liquid that pours and sloshes with the accelerometer",
		uses: []string{"accelerometer", "linear_acceleration", "gravity"},
		new:  func(specs []string) Demo { return &fluid{fill: 0.35} },
	})
}

// fluid is a 2D particle liquid using double density relaxation (Clavet et
// al., "Particle-based viscoelastic fluid simulation", 2005): each particle
// pushes its neighbors apart when the local density is above rest density,
// which gives incompressible-looking water with a free surface.
//
// Units: one column is 1 unit wide and one row 2 units tall (terminal cells
// are about twice as tall as wide), so the simulation is not stretched.
// Gravity is the accelerometer's in-screen component, so tilting pours,
// shaking makes waves, and lying flat is zero-g.
type fluid struct {
	fill float64 // fraction of the box filled with liquid

	w, h   int     // terminal cells the particles were laid out for
	W, H   float64 // box in simulation units
	px, py []float64
	ox, oy []float64 // positions before the step (velocity = p - o)
	grid   map[int][]int32
	count  []int
	speed  []float64
	dens   []float64
	kick   float64
	rng    *rand.Rand
}

const (
	fluidRadius  = 2.2    // interaction radius
	fluidRest    = 3.0    // rest density
	fluidStiff   = 0.08   // pressure stiffness
	fluidNear    = 0.35   // near-pressure stiffness: keeps particles apart
	fluidSteps   = 3      // substeps per frame
	fluidGravity = 0.0011 // units/step^2 per m/s^2
	fluidSpacing = 0.8    // particle spacing at rest density
)

var waterRamp = []byte(" .:-=+*#%@")

func (f *fluid) Setup(ss *Streams) ([]*Gauge, error) {
	st, err := ss.Subscribe("accelerometer", 60)
	if err != nil {
		return nil, err
	}
	f.rng = rand.New(rand.NewSource(1))
	return gaugesFor(st, 3), nil
}

func (f *fluid) Help() []string {
	return []string{
		"Tilt the phone to pour, shake it to make waves.",
		"Lay it flat for zero gravity.",
		"",
		"s  splash      r  refill",
		"+ -  more / less liquid",
	}
}

func (f *fluid) Key(k byte) {
	switch k {
	case 'r':
		f.w = 0 // re-lay out on the next frame
	case 's':
		f.kick = 1
	case '+', '=':
		f.fill = math.Min(f.fill+0.05, 0.7)
		f.w = 0
	case '-', '_':
		f.fill = math.Max(f.fill-0.05, 0.05)
		f.w = 0
	}
}

// layout fills the bottom of a w x h box with particles on a jittered grid,
// or rescales existing particles when only the size changed.
func (f *fluid) layout(w, h int) {
	W, H := float64(w), float64(2*h)
	if f.w != 0 && len(f.px) > 0 {
		sx, sy := W/f.W, H/f.H
		for i := range f.px {
			f.px[i] *= sx
			f.py[i] *= sy
			f.ox[i], f.oy[i] = f.px[i], f.py[i]
		}
		f.w, f.h, f.W, f.H = w, h, W, H
		return
	}
	f.w, f.h, f.W, f.H = w, h, W, H
	f.px, f.py, f.ox, f.oy = nil, nil, nil, nil
	top := H * (1 - f.fill)
	for y := H - fluidSpacing/2; y > top; y -= fluidSpacing {
		for x := fluidSpacing / 2; x < W; x += fluidSpacing {
			jx := x + (f.rng.Float64()-0.5)*0.1
			jy := y + (f.rng.Float64()-0.5)*0.1
			f.px, f.py = append(f.px, jx), append(f.py, jy)
		}
	}
	f.ox = append([]float64(nil), f.px...)
	f.oy = append([]float64(nil), f.py...)
}

func (f *fluid) step(gx, gy float64) {
	n := len(f.px)
	// Integrate: verlet-style, velocity is the last displacement.
	for i := 0; i < n; i++ {
		vx := (f.px[i] - f.ox[i]) * 0.998
		vy := (f.py[i] - f.oy[i]) * 0.998
		f.ox[i], f.oy[i] = f.px[i], f.py[i]
		f.px[i] += vx + gx
		f.py[i] += vy + gy
	}

	// Spatial hash with cells the size of the interaction radius.
	if f.grid == nil {
		f.grid = map[int][]int32{}
	}
	for k := range f.grid {
		f.grid[k] = f.grid[k][:0]
	}
	cell := func(x, y float64) (int, int) { return int(x / fluidRadius), int(y / fluidRadius) }
	key := func(cx, cy int) int { return cy*4096 + cx }
	for i := 0; i < n; i++ {
		cx, cy := cell(f.px[i], f.py[i])
		f.grid[key(cx, cy)] = append(f.grid[key(cx, cy)], int32(i))
	}

	// Double density relaxation.
	var nb []int32
	var nq []float64
	for i := 0; i < n; i++ {
		nb, nq = nb[:0], nq[:0]
		rho, rhoN := 0.0, 0.0
		cx, cy := cell(f.px[i], f.py[i])
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				for _, j := range f.grid[key(cx+dx, cy+dy)] {
					if int(j) == i {
						continue
					}
					rx, ry := f.px[j]-f.px[i], f.py[j]-f.py[i]
					r2 := rx*rx + ry*ry
					if r2 >= fluidRadius*fluidRadius {
						continue
					}
					q := 1 - math.Sqrt(r2)/fluidRadius
					rho += q * q
					rhoN += q * q * q
					nb, nq = append(nb, j), append(nq, q)
				}
			}
		}
		P := fluidStiff * (rho - fluidRest)
		PN := fluidNear * rhoN
		dxi, dyi := 0.0, 0.0
		for k, j := range nb {
			q := nq[k]
			rx, ry := f.px[j]-f.px[i], f.py[j]-f.py[i]
			r := math.Sqrt(rx*rx + ry*ry)
			if r < 1e-9 {
				rx, ry, r = f.rng.Float64()-0.5, f.rng.Float64()-0.5, 0.5
			}
			D := 0.5 * (P*q + PN*q*q)
			ux, uy := rx/r*D, ry/r*D
			f.px[j] += ux
			f.py[j] += uy
			dxi -= ux
			dyi -= uy
		}
		f.px[i] += dxi
		f.py[i] += dyi
	}

	// Walls.
	const m = 0.3
	for i := 0; i < n; i++ {
		f.px[i] = math.Max(m, math.Min(f.W-m, f.px[i]))
		f.py[i] = math.Max(m, math.Min(f.H-m, f.py[i]))
	}
}

func (f *fluid) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 4 || v.H < 4 {
		return
	}
	// Simulate on a grid no bigger than a normal screen's (70x40 cells) and
	// draw it scaled up: a small font then gives a smoother picture of the
	// same liquid instead of many more particles and a slower frame.
	scale := math.Max(1, math.Sqrt(float64(v.W*v.H)/(70*40)))
	gw, gh := int(math.Ceil(float64(v.W)/scale)), int(math.Ceil(float64(v.H)/scale))
	if f.w != gw || f.h != gh {
		f.layout(gw, gh)
	}

	// Screen x is device x; screen y runs down while device y runs up. The
	// accelerometer reads the reaction to gravity, so gravity is -a.
	gx, gy := 0.0, 0.0
	if r := ss.Get("accelerometer").Read(); r.OK && len(r.V) >= 2 {
		a := toScreen(ss, r.V)
		gx, gy = -a[0]*fluidGravity, a[1]*fluidGravity
	}
	if f.kick > 0 { // splash: throw everything up and sideways
		for i := range f.px {
			f.oy[i] = f.py[i] + 1.5 + f.rng.Float64()
			f.ox[i] = f.px[i] + (f.rng.Float64()-0.5)*1.5
		}
		f.kick = 0
	}
	for s := 0; s < fluidSteps; s++ {
		f.step(gx, gy)
	}

	// Render: particle count and speed per simulation cell, smoothed over
	// neighbors so the body reads as a surface rather than noise, then
	// sampled for every screen cell. Density picks the character; speed
	// picks the color (calm = deep blue, fast = foam).
	n := gw * gh
	if len(f.count) != n {
		f.count = make([]int, n)
		f.speed = make([]float64, n)
		f.dens = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		f.count[i], f.speed[i] = 0, 0
	}
	for i := range f.px {
		x, y := int(f.px[i]), int(f.py[i]/2)
		if x >= 0 && y >= 0 && x < gw && y < gh {
			c := y*gw + x
			f.count[c]++
			f.speed[c] += math.Hypot(f.px[i]-f.ox[i], f.py[i]-f.oy[i])
		}
	}
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= gw || y >= gh {
			return 0
		}
		return float64(f.count[y*gw+x])
	}
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			f.dens[y*gw+x] = (4*at(x, y) + 2*(at(x-1, y)+at(x+1, y)+at(x, y-1)+at(x, y+1)) +
				at(x-1, y-1) + at(x+1, y-1) + at(x-1, y+1) + at(x+1, y+1)) / 16
		}
	}
	dens := func(x, y int) float64 {
		x, y = max(0, min(x, gw-1)), max(0, min(y, gh-1))
		return f.dens[y*gw+x]
	}
	for y := 0; y < v.H; y++ {
		for x := 0; x < v.W; x++ {
			// bilinear sample of the simulation grid
			sx, sy := (float64(x)+0.5)/scale-0.5, (float64(y)+0.5)/scale-0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			d := (dens(x0, y0)*(1-fx)+dens(x0+1, y0)*fx)*(1-fy) +
				(dens(x0, y0+1)*(1-fx)+dens(x0+1, y0+1)*fx)*fy
			if d < 0.25 {
				continue
			}
			k := min(int(d*3.2), len(waterRamp)-1)
			k = max(k, 1)
			col := uint8(153) // spray between cells
			c := max(0, min(int(sy+0.5), gh-1))*gw + max(0, min(int(sx+0.5), gw-1))
			if f.count[c] > 0 {
				col = 27 // deep blue
				sp := f.speed[c] / float64(f.count[c])
				switch {
				case sp > 0.35:
					col = 231 // foam
				case sp > 0.2:
					col = 159
				case sp > 0.1:
					col = 45
				case sp > 0.05:
					col = 33
				}
			}
			v.Set(x, y, waterRamp[k], col)
		}
	}
}
