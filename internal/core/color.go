package core

import "math"

func Pick(ramp []uint8, i float64) uint8 {
	return ramp[int(Clamp01(i)*float64(len(ramp)-1)+0.5)]
}

// RGB256 is the terminal color nearest an RGB color (0..1): from the 6x6x6
// cube, or the gray ramp for grays.
func RGB256(c [3]float64) uint8 {
	levels := [6]float64{0, 95, 135, 175, 215, 255}
	var idx [3]int
	var cube [3]float64
	for i := 0; i < 3; i++ {
		v := Clamp01(c[i]) * 255
		best := 0
		for j := 1; j < 6; j++ {
			if math.Abs(levels[j]-v) < math.Abs(levels[best]-v) {
				best = j
			}
		}
		idx[i], cube[i] = best, levels[best]
	}
	col := uint8(16 + 36*idx[0] + 6*idx[1] + idx[2])
	// A gray may be nearer.
	avg := (Clamp01(c[0]) + Clamp01(c[1]) + Clamp01(c[2])) / 3 * 255
	g := int(math.Round((avg - 8) / 10))
	g = max(0, min(23, g))
	gv := float64(8 + 10*g)
	dist := func(a [3]float64) float64 {
		s := 0.0
		for i := 0; i < 3; i++ {
			d := a[i] - Clamp01(c[i])*255
			s += d * d
		}
		return s
	}
	if dist([3]float64{gv, gv, gv}) < dist(cube) {
		return uint8(232 + g)
	}
	return col
}

// ArtCell is a cell of art drawn once and kept: its character and color,
// and whether it stands in front of what moves (the hourglass's frame in
// front of its sand).
type ArtCell struct {
	Ch    byte
	Col   uint8
	Front bool
}

// Color ramps, dark to bright, each one hue throughout: warm wood (the
// hourglass's frame, the dice tray), its dark ends, and brass.
var (
	WoodRamp  = []uint8{232, 52, 88, 94, 130, 137, 173, 180, 223}
	EndRamp   = []uint8{232, 233, 52, 88, 94, 130, 137}
	BrassRamp = []uint8{94, 130, 136, 172, 178, 214, 220, 221, 222, 229, 230, 231}
)

// LineGlyph is the character for a line running dx across, dy down (in
// cells, which are twice as tall as wide).
func LineGlyph(dx, dy float64) byte {
	a := math.Atan2(dy*2, dx) // in square units
	if a < 0 {
		a += math.Pi
	}
	switch {
	case a < math.Pi/8 || a > 7*math.Pi/8:
		return '-'
	case a < 3*math.Pi/8:
		return '\\'
	case a < 5*math.Pi/8:
		return '|'
	}
	return '/'
}

// LineChar picks the character that looks like a segment of this direction.
func LineChar(dx, dy float64) byte {
	a := math.Mod(math.Atan2(-dy*2, dx)*180/math.Pi+180, 180) // screen-true angle
	switch {
	case a < 22.5 || a >= 157.5:
		return '-'
	case a < 67.5:
		return '/'
	case a < 112.5:
		return '|'
	default:
		return '\\'
	}
}

func BoolIdx(b bool) int {
	if b {
		return 1
	}
	return 0
}
