package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	register(entry{
		name: "homing",
		desc: "a 3D arrow pointing to a saved place, with the distance (save one with: homing save)",
		uses: []string{},
		new:  func(specs []string) Demo { return &homing{} },
	})
}

// homing points a ray-marched 3D arrow at a saved place: the bearing from
// here (great circle) minus the phone's true heading. Places are saved on the
// phone only (~/.config/sensordemo/places.json):
//
//	sensordemo homing save        save here as "home"
//	sensordemo homing save:work   save here as "work"
//	sensordemo homing             point to home
//	sensordemo homing work        point to work
type homing struct {
	name   string
	save   bool
	saved  bool
	target latLon
	has    bool

	rel     float64 // smoothed arrow angle on screen, radians clockwise
	heading float64
	arrived float64
	err     string
}

type place struct {
	Lat, Lon float64
	Saved    time.Time
}

func placesPath() string {
	if p := os.Getenv("SENSORDEMO_PLACES"); p != "" {
		return p
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "sensordemo", "places.json")
}

func loadPlaces() map[string]place {
	m := map[string]place{}
	if b, err := os.ReadFile(placesPath()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (h *homing) Setup(ss *Streams) ([]*Gauge, error) {
	h.name = "home"
	switch arg := demoArg; {
	case arg == "save":
		h.save = true
	case strings.HasPrefix(arg, "save:"):
		h.save, h.name = true, strings.TrimPrefix(arg, "save:")
	case arg != "":
		h.name = arg
	}
	if !h.save {
		p, ok := loadPlaces()[h.name]
		if !ok {
			return nil, fmt.Errorf("no place %q saved yet: go there and run  sensordemo homing save%s", h.name,
				map[bool]string{true: "", false: ":" + h.name}[h.name == "home"])
		}
		h.target, h.has = latLon{p.Lat, p.Lon}, true
	}
	st, err := ss.Subscribe("location", 1)
	if err != nil {
		return nil, err
	}
	ss.Subscribe("rotation_vector", 30)
	return []*Gauge{
		{Spec: st.Spec, Index: 2, Label: "accuracy", Unit: "m", Scale: &Asymptotic{K: 20}},
	}, nil
}

func (h *homing) Help() []string {
	return []string{
		"The arrow points to the saved place; the number is the distance as the crow flies.",
		"Save a place where you stand: sensordemo homing save (as home) or homing save:NAME. Point to it: sensordemo homing or homing NAME. Places stay on the phone.",
		"Indoors the location is rough (tens of meters): the arrow is steadier outside.",
	}
}

func (h *homing) Key(k byte) {}

// bearing is the initial great-circle bearing from a to b, radians clockwise
// from true north; distance in meters (haversine).
func bearingDistance(a, b latLon) (float64, float64) {
	const R = 6371000.0
	la1, la2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dl := (b.Lon - a.Lon) * math.Pi / 180
	y := math.Sin(dl) * math.Cos(la2)
	x := math.Cos(la1)*math.Sin(la2) - math.Sin(la1)*math.Cos(la2)*math.Cos(dl)
	s := math.Sin((la2-la1)/2)*math.Sin((la2-la1)/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dl/2)*math.Sin(dl/2)
	return math.Atan2(y, x), 2 * R * math.Asin(math.Sqrt(s))
}

func formatDistance(m float64) string {
	switch {
	case m < 1000:
		return fmt.Sprintf("%.0f m", m)
	case m < 10000:
		return fmt.Sprintf("%.1f km", m/1000)
	}
	return fmt.Sprintf("%.0f km", m/1000)
}

func (h *homing) Draw(v *View, ss *Streams, t, dt float64) {
	if v.W < 16 || v.H < 10 {
		return
	}
	r := ss.Get("location").Read()
	if !r.OK || len(r.V) < 2 {
		msg := "waiting for a location fix... (outdoors is faster)"
		v.Text(max(0, (v.W-len(msg))/2), v.H/2, msg, 244)
		return
	}
	here := latLon{r.V[0], r.V[1]}
	acc := math.NaN()
	if len(r.V) > 2 {
		acc = r.V[2]
	}

	if h.save {
		if !h.saved {
			m := loadPlaces()
			m[h.name] = place{Lat: here.Lat, Lon: here.Lon, Saved: time.Now()}
			b, _ := json.MarshalIndent(m, "", "  ")
			os.MkdirAll(filepath.Dir(placesPath()), 0o700)
			if err := os.WriteFile(placesPath(), b, 0o600); err != nil {
				h.err = err.Error()
			}
			h.saved = true
		}
		msg := fmt.Sprintf("saved this spot as %q (accuracy %.0f m)", h.name, acc)
		if h.err != "" {
			msg = "could not save: " + h.err
		}
		text := "SAVED"
		s := max(1, fitScale(text, v.W-4, v.H/3))
		bw, bh := bannerSize(text, s)
		drawBanner(v, text, (v.W-bw)/2, (v.H-bh)/2-1, s, '#', 114)
		v.Text(max(0, (v.W-len(msg))/2), (v.H+bh)/2+1, msg, 250)
		hint := fmt.Sprintf("point to it later with: sensordemo homing%s", map[bool]string{true: "", false: " " + h.name}[h.name == "home"])
		v.Text(max(0, (v.W-len(hint))/2), (v.H+bh)/2+3, hint, 244)
		return
	}

	// Where the phone points (true north) and where the place is.
	if rv := ss.Get("rotation_vector"); rv != nil {
		if rr := rv.Read(); rr.OK {
			if R, ok := FromRotationVector(rr.V); ok {
				hd := Heading(R.Mul(screenFrame(ss)))
				if d, ok := declination(ss); ok {
					hd += d * math.Pi / 180
				}
				h.heading = hd
			}
		}
	}
	brg, dist := bearingDistance(here, h.target)
	want := brg - h.heading
	h.rel = math.Remainder(h.rel+math.Remainder(want-h.rel, 2*math.Pi)*math.Min(1, dt*5), 2*math.Pi)

	near := dist < 15 || (!math.IsNaN(acc) && dist < acc*0.6)
	area := v.H * 13 / 20
	h.renderArrow(v, area, t, near)

	// Distance and walking time.
	dtext := formatDistance(dist)
	if near {
		dtext = "HERE"
	}
	s := max(1, fitScale(strings.ToUpper(dtext), v.W-4, (v.H-area)*2/3))
	bw, bh := bannerSize(strings.ToUpper(dtext), s)
	drawBanner(v, strings.ToUpper(dtext), (v.W-bw)/2, area, s, '#', map[bool]uint8{true: 114, false: 214}[near])
	walk := time.Duration(dist / 1.3 * float64(time.Second)).Round(time.Minute)
	info := fmt.Sprintf("to %s  -  %s on foot", h.name, strings.TrimSuffix(walk.String(), "0s"))
	if near {
		info = fmt.Sprintf("you are at %s", h.name)
	}
	if !math.IsNaN(acc) && acc > dist*0.5 && !near {
		info += fmt.Sprintf("  (fix +-%.0f m)", acc)
	}
	v.Text(max(0, (v.W-len(info))/2), min(area+bh+1, v.H-1), info, 250)
}

// renderArrow ray-marches a solid arrow floating above a compass disc,
// turned to h.rel (clockwise, 0 = straight ahead, away from you).
func (h *homing) renderArrow(v *View, rows int, t float64, near bool) {
	cam := Vec3{0, 2.4, -3.0}
	look := Vec3{0, 0.1, 0.35}.Sub(cam).Norm()
	right := Vec3{0, 1, 0}.Cross(look).Norm() // +x: east when facing north
	up := look.Cross(right)
	lamp := Vec3{-0.4, 1, -0.5}.Norm()
	bob := 0.05 * math.Sin(t*2)
	spin := h.rel
	if near {
		spin = t * 1.5 // arrived: it spins in celebration
	}
	c, s := math.Cos(spin), math.Sin(spin)
	aspect := float64(2*rows) / float64(v.W)
	tanX := 0.62
	tanY := tanX * aspect
	if tanY < 0.45 { // wide: fit the height
		tanY = 0.45
		tanX = tanY / aspect
	}
	scene := func(p Vec3) (float64, int) {
		ground := p[1]
		// arrow in its own frame: x right, z forward, turned by spin
		q := Vec3{c*p[0] - s*p[2], p[1] - 0.35 - bob, s*p[0] + c*p[2]}
		shaft := math.Max(math.Abs(q[0])-0.13, math.Max(q[2]-0.25, -0.95-q[2]))
		hw := 0.55 * clamp01((1.05-q[2])/0.8) // head: widest at z=0.25, tip at 1.05
		head := math.Max(math.Abs(q[0])-hw, math.Max(q[2]-1.05, 0.25-q[2])) * 0.8
		a2 := math.Min(shaft, head)
		arrow := math.Max(a2, math.Abs(q[1])-0.09) - 0.02 // extruded, softly rounded
		if arrow < ground {
			return arrow, 1
		}
		return ground, 0
	}
	parallelRows(rows, func(y int) {
		for x := 0; x < v.W; x++ {
			u := (2*(float64(x)+0.5)/float64(v.W) - 1) * tanX
			w := (1 - 2*(float64(y)+0.5)/float64(rows)) * tanY
			dir := look.Add(right.Scale(u)).Add(up.Scale(w)).Norm()
			tt := 0.0
			for i := 0; i < 80 && tt < 12; i++ {
				p := cam.Add(dir.Scale(tt))
				d, mat := scene(p)
				if d < 0.002 {
					if mat == 0 {
						h.shadeGround(v, x, y, p, scene, lamp, spin)
					} else {
						e := 0.002
						f := func(q Vec3) float64 { v, _ := scene(q); return v }
						n := Vec3{f(p.Add(Vec3{e, 0, 0})) - f(p.Sub(Vec3{e, 0, 0})),
							f(p.Add(Vec3{0, e, 0})) - f(p.Sub(Vec3{0, e, 0})),
							f(p.Add(Vec3{0, 0, e})) - f(p.Sub(Vec3{0, 0, e}))}.Norm()
						lum := 0.2 + 0.8*math.Max(0, n.Dot(lamp))
						spec := math.Pow(math.Max(0, n.Dot(lamp.Sub(dir).Norm())), 30)
						lum = math.Min(1, lum+spec*0.5)
						k := min(int(lum*float64(len(shade))), len(shade)-1)
						cols := []uint8{94, 130, 166, 172, 208, 214, 220, 221, 227, 229}
						if near {
							cols = []uint8{22, 28, 34, 40, 76, 112, 118, 154, 156, 194}
						}
						v.Set(x, y, shade[k], cols[k])
					}
					break
				}
				tt += d
			}
		}
	})
}

// shadeGround draws the compass disc: a ring with N E S W turned to the
// phone's heading, the arrow's shadow, and nothing beyond the disc.
func (h *homing) shadeGround(v *View, x, y int, p Vec3, scene func(Vec3) (float64, int), lamp Vec3, spin float64) {
	r := math.Hypot(p[0], p[2])
	if r > 1.6 {
		return
	}
	// shadow: is the lamp blocked by the arrow?
	lit := true
	for tt := 0.02; tt < 3; {
		d, mat := scene(p.Add(lamp.Scale(tt)))
		if mat == 1 && d < 0.003 {
			lit = false
			break
		}
		tt += math.Max(d, 0.02)
	}
	c, col := byte('.'), uint8(238)
	if r > 1.45 {
		c, col = ':', 244 // the rim
		// cardinal letters around the rim, turned with the heading
		ang := math.Atan2(p[0], p[2]) + h.heading // world azimuth of this spot
		for i, name := range []string{"N", "E", "S", "W"} {
			if math.Abs(math.Remainder(ang-float64(i)*math.Pi/2, 2*math.Pi)) < 0.07 {
				c, col = name[0], map[bool]uint8{true: 196, false: 250}[i == 0]
			}
		}
	}
	if !lit {
		c, col = ' ', 0
		if r > 1.45 {
			c, col = ':', 238
		}
	}
	v.Set(x, y, c, col)
}
