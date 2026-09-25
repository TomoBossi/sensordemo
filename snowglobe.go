package main

import (
	"math"
	"math/rand"
)

func init() {
	register(entry{
		name: "snowglobe",
		desc: "a snow globe: shake the phone for a flurry that settles on a little winter scene",
		uses: []string{"linear_acceleration"},
		new:  func(specs []string) Demo { return &snowglobe{} },
	})
}

// snowglobe is a glass dome on a wooden base with a small scene inside. Its
// flakes sink slowly through the "water" (strong drag), drift with the
// phone's tilt, and settle on the scene where they land; shaking the phone
// (acceleration beyond gravity) knocks the settled snow loose and stirs it.
type snowglobe struct {
	w, h   int
	cx, cy float64 // dome center, cells
	R      float64 // dome radius in columns (rows are twice as tall)
	solid  []bool  // scene and ground: where snow can rest
	scene  []byte  // scene characters
	sceneC []uint8
	settle []int // settled flakes per cell
	flakes []flake
	rng    *rand.Rand
	gx, gy float64 // smoothed gravity on screen
	stir   float64 // recent shaking, decays
}

type flake struct {
	x, y, vx, vy float64 // y in rows
	size         int     // 0 small .. 2 large (drawn nearer)
	stuck        bool
}

func (g *snowglobe) Setup(ss *Streams) ([]*Gauge, error) {
	g.rng = rand.New(rand.NewSource(9))
	if _, err := ss.Subscribe("accelerometer", 50); err != nil {
		return nil, err
	}
	lin, err := ss.Subscribe("linear_acceleration", 50)
	if err != nil {
		return nil, err
	}
	return gaugesFor(lin, 3), nil
}

func (g *snowglobe) Help() []string {
	return []string{
		"Shake the phone: the settled snow flies up and swirls, then sinks and settles on the roof, the tree, the snowman and the ground.",
		"Tilt it and the falling snow drifts that way.",
		"space  a small shake    r  start over",
	}
}

func (g *snowglobe) Key(k byte) {
	switch k {
	case ' ':
		g.stir = 6
	case 'r':
		g.w = 0
	}
}

// in reports whether (x, y) is inside the glass.
func (g *snowglobe) in(x, y float64) bool {
	dx, dy := (x-g.cx)/g.R, (y-g.cy)*2/g.R
	return dx*dx+dy*dy < 0.97
}

func (g *snowglobe) put(x, y int, s string, col uint8, solid bool) {
	for i := 0; i < len(s); i++ {
		xx := x + i
		if xx < 0 || y < 0 || xx >= g.w || y >= g.h || s[i] == ' ' {
			continue
		}
		g.scene[y*g.w+xx] = s[i]
		g.sceneC[y*g.w+xx] = col
		if solid {
			g.solid[y*g.w+xx] = true
		}
	}
}

func (g *snowglobe) layout(w, h int) {
	g.w, g.h = w, h
	g.R = math.Min(float64(w)/2-2, float64(h-5)) // leave room for the base
	g.cx, g.cy = float64(w)/2, g.R/2+1
	n := w * h
	g.solid, g.scene, g.sceneC, g.settle = make([]bool, n), make([]byte, n), make([]uint8, n), make([]int, n)

	// Ground: a snowy hill across the bottom of the dome.
	for x := 0; x < w; x++ {
		dx := (float64(x) - g.cx) / g.R
		if math.Abs(dx) >= 1 {
			continue
		}
		hill := g.cy + g.R/2*(0.52-0.12*math.Cos(dx*3.1)+0.08*math.Sin(dx*7))
		for y := int(hill); y < h; y++ {
			if g.in(float64(x)+0.5, float64(y)+0.5) {
				c := byte('.')
				if y > int(hill) {
					c = ':'
				}
				g.put(x, y, string(c), 255, true)
			}
		}
	}
	ground := func(x int) int { // first ground row in column x
		for y := 0; y < h; y++ {
			if g.solid[y*w+x] {
				return y
			}
		}
		return h
	}

	// Cabin, left of center.
	hx := int(g.cx - g.R*0.64)
	base := ground(hx+5) - 1
	cabin := []string{
		"   ||      ",
		"   ||____  ",
		"  /      \\ ",
		" /________\\",
		" | [#] [] |",
		" |  _   _ |",
		" |_| |____|",
	}
	for i, row := range cabin {
		col := uint8(137)
		switch {
		case i <= 1:
			col = 95 // chimney
		case i <= 3:
			col = 124 // roof
		}
		g.put(hx, base-len(cabin)+1+i, row, col, true)
	}
	// the lit window
	for i := 0; i < len(cabin[4]); i++ {
		if cabin[4][i] == '#' {
			g.sceneC[(base-2)*w+hx+i] = 220
		}
	}

	// Pine tree, right of center.
	tx := int(g.cx + g.R*0.42)
	tb := ground(tx) - 1
	tree := []string{"    ^    ", "   /^\\   ", "  //^\\\\  ", "   /^\\   ", "  //^\\\\  ", " ///^\\\\\\ ", "    |    "}
	for i, row := range tree {
		col := uint8(28 + 6*uint8(i%3))
		if i == len(tree)-1 {
			col = 94
		}
		g.put(tx-4, tb-len(tree)+1+i, row, col, true)
	}

	// Snowman in the middle.
	sx := int(g.cx) - 1
	sb := ground(sx+2) - 1
	snowman := []string{"  _  ", " (\") ", "( : )", "(   )"}
	for i, row := range snowman {
		g.put(sx, sb-len(snowman)+1+i, row, 255, true)
	}
	g.sceneC[(sb-2)*w+sx+2] = 208 // carrot-ish

	// Flakes: scattered through the dome, some already resting.
	g.flakes = g.flakes[:0]
	count := int(g.R * g.R * 0.9)
	for len(g.flakes) < count {
		x := g.cx + (g.rng.Float64()*2-1)*g.R
		y := g.cy + (g.rng.Float64()*2-1)*g.R/2
		if !g.in(x, y) || g.solid[int(y)*w+int(x)] {
			continue
		}
		g.flakes = append(g.flakes, flake{x: x, y: y, size: g.rng.Intn(3)})
	}
	g.stir = 3
}

func (g *snowglobe) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 20 || v.H < 12 {
		return
	}
	if g.w != v.W || g.h != v.H {
		g.layout(v.W, v.H)
	}
	if r := ss.Get("accelerometer").Read(); r.OK && len(r.V) >= 2 {
		a := toScreen(ss, r.V)
		g.gx += (-a[0] - g.gx) * math.Min(1, dt*4)
		g.gy += (a[1] - g.gy) * math.Min(1, dt*4)
	}
	if r := ss.Get("linear_acceleration").Read(); r.OK && len(r.V) >= 3 {
		shake := math.Sqrt(r.V[0]*r.V[0] + r.V[1]*r.V[1] + r.V[2]*r.V[2])
		if shake > 3 {
			g.stir = math.Min(g.stir+shake*dt*3, 10)
		}
	}
	g.stir *= math.Exp(-dt * 0.7)

	// A good shake knocks the settled snow loose.
	if g.stir > 2.5 {
		for i := range g.flakes {
			if g.flakes[i].stuck && g.rng.Float64() < g.stir*dt {
				g.flakes[i].stuck = false
				g.flakes[i].vy = -g.rng.Float64() * g.stir * 0.6
			}
		}
	}
	for i := range g.settle {
		g.settle[i] = 0
	}

	// Sinking speed: slow, like snow in water; tilt drifts it sideways.
	gl := math.Max(math.Hypot(g.gx, g.gy), 1)
	ux, uy := g.gx/gl, g.gy/gl
	for i := range g.flakes {
		f := &g.flakes[i]
		if f.stuck {
			g.settle[int(f.y)*g.w+int(f.x)]++
			continue
		}
		sink := 1.2 + 0.4*float64(f.size) // rows per second; big flakes sink faster
		swirl := g.stir * 1.5
		// Falling follows gravity on the screen: tilt the phone and the snow
		// slants that way (x in columns, which are half a row wide).
		f.vx += (ux*sink*6-f.vx)*dt*1.5 + (g.rng.Float64()-0.5)*swirl*dt*6 + math.Sin(t*0.9+f.y)*dt*0.4
		f.vy += (uy*sink-f.vy)*dt*1.5 + (g.rng.Float64()-0.5)*swirl*dt*3
		nx, ny := f.x+f.vx*dt, f.y+f.vy*dt
		if !g.in(nx, ny) {
			// against the glass: when calm, snow rests there too;
			// otherwise it bounces off
			if g.stir < 1.5 && f.vx*ux+f.vy*uy > 0 {
				f.stuck = true
			} else {
				f.vx, f.vy = -f.vx*0.4, -f.vy*0.4
			}
			continue
		}
		xi, yi := int(nx), int(ny)
		if g.solid[yi*g.w+xi] || g.settle[yi*g.w+xi] >= 2 {
			// landed on the scene or on settled snow: rest just above it
			if g.stir < 1.5 {
				f.stuck = true
			} else {
				f.vx, f.vy = -f.vx*0.3, -math.Abs(f.vy)*0.5
			}
			continue
		}
		f.x, f.y = nx, ny
	}

	// Glass and base.
	for a := 0.0; a < 2*math.Pi; a += 0.3 / g.R {
		x := g.cx + g.R*math.Cos(a)
		y := g.cy + g.R/2*math.Sin(a)
		if y < g.cy+g.R/2*0.75 {
			c := byte('.')
			if deg := math.Mod(a*180/math.Pi, 360); deg > 205 && deg < 228 {
				c = '\'' // a highlight on the upper left of the glass
			}
			v.Set(int(x), int(y), c, 117)
		}
	}
	by := int(g.cy + g.R/2*0.8)
	for row := 0; row < 4 && by+row < v.H; row++ {
		half := int(g.R*0.8) + row*2
		for x := int(g.cx) - half; x <= int(g.cx)+half; x++ {
			c, col := byte('#'), uint8(94)
			switch {
			case x == int(g.cx)-half:
				c = '/'
			case x == int(g.cx)+half:
				c = '\\'
			case row == 1:
				c, col = '=', 136
			}
			v.Set(x, by+row, c, col)
		}
	}

	// Scene, with snow cover where flakes rest.
	for y := 0; y < g.h && y < by; y++ {
		for x := 0; x < g.w; x++ {
			i := y*g.w + x
			if g.scene[i] != 0 {
				v.Set(x, y, g.scene[i], g.sceneC[i])
			}
			if g.settle[i] > 0 {
				c := byte('.')
				if g.settle[i] > 2 {
					c = ':'
				}
				v.Set(x, y, c, 255)
			}
		}
	}
	// Smoke from the chimney, drifting with the tilt.
	for k := 0; k < 5; k++ {
		sx := g.cx - g.R*0.64 + 3.5 + float64(k)*0.7*ux + math.Sin(t*1.3+float64(k))*0.8
		sy := float64(int(g.cy+g.R/2*0.52)) - 9 - float64(k)*1.1
		if g.in(sx, sy) {
			v.Set(int(sx), int(sy), "~.~. "[k], 245)
		}
	}
	// Falling flakes, nearer ones bigger.
	for _, f := range g.flakes {
		if !f.stuck && int(f.y) < by {
			v.Set(int(f.x), int(f.y), ".+*"[f.size], []uint8{250, 253, 231}[f.size])
		}
	}
}
