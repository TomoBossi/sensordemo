package core

import "math"

// Palettes recolor a frame at output time. Demos draw with their own 256-color
// choices ("native"); every other palette keeps only each color's brightness
// and maps it onto its own ramp, so any demo can be shown in any palette.
type palette struct {
	Name string
	ramp []uint8 // dark to bright; nil = native colors
}

var Palettes = []palette{
	{"native", nil},
	{"gray", []uint8{236, 238, 240, 242, 244, 246, 248, 250, 252, 254, 255}},
	{"amber", []uint8{52, 94, 130, 136, 172, 178, 214, 220, 221, 222, 223, 230}},
	{"green", []uint8{22, 28, 34, 40, 46, 82, 118, 119, 120, 157, 194}},
	{"ice", []uint8{17, 18, 19, 20, 26, 32, 38, 44, 45, 81, 117, 159, 195}},
	{"fire", []uint8{52, 88, 124, 160, 196, 202, 208, 214, 220, 226, 227, 229, 231}},
	{"violet", []uint8{53, 54, 55, 91, 92, 93, 129, 135, 141, 177, 183, 189, 225}},
}

// Map returns the color to draw for a demo's color c (0 = terminal default,
// left alone).
func (p *palette) Map(c uint8) uint8 {
	if c == 0 || p.ramp == nil {
		return c
	}
	l := Luminance(c)
	return p.ramp[min(int(l*float64(len(p.ramp))), len(p.ramp)-1)]
}

var CubeLevels = [6]float64{0, 95, 135, 175, 215, 255}

// Luminance of an xterm 256-color index, 0..1, with a gamma that keeps dark
// colors dark, so demos' shading survives the remap.
func Luminance(c uint8) float64 {
	var r, g, b float64
	switch {
	case c < 16:
		base := [16][3]float64{{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0}, {0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
			{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255}}[c]
		r, g, b = base[0], base[1], base[2]
	case c < 232:
		i := int(c) - 16
		r, g, b = CubeLevels[i/36], CubeLevels[i/6%6], CubeLevels[i%6]
	default:
		v := float64(8 + 10*(int(c)-232))
		r, g, b = v, v, v
	}
	l := (0.2126*r + 0.7152*g + 0.0722*b) / 255
	return math.Pow(l, 1.4)
}
