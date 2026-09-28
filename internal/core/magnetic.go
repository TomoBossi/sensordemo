package core

import (
	"fmt"
	"math"
)

// Compass corrections and diagnostics shared by compass and navball.
//
// The rotation vector's heading is magnetic. With a location fix, sensord
// adds the local declination (true north = magnetic + declination) and the
// field strength Earth should have here; comparing that with what the
// magnetometer measures reveals nearby magnets and steel, the usual cause of
// big compass errors indoors.

// SubscribeMagnetics asks for location rarely (declination barely changes)
// and the magnetometer. Location is optional: without it, headings stay
// magnetic.
func SubscribeMagnetics(ss *Streams) (*Stream, error) {
	ss.Subscribe("location", 0.1)
	return ss.Subscribe("magnetic_field", 20)
}

// Declination returns the local declination in degrees, if known.
func Declination(ss *Streams) (float64, bool) {
	if s := ss.Get("location"); s != nil {
		if r := s.Read(); r.OK && len(r.V) >= 7 && !math.IsNaN(r.V[6]) {
			return r.V[6], true
		}
	}
	return 0, false
}

// MagStatus is a one-line diagnosis of the compass that fits width: true or
// magnetic north, the field measured vs expected, and the calibration
// status. warn is set when the reading is probably off.
func MagStatus(ss *Streams, width int) (line string, warn bool) {
	north, decl := "magnetic N", ""
	if d, ok := Declination(ss); ok {
		north, decl = "true N", fmt.Sprintf(" (decl %+.0f)", d)
	}
	var field, problem string
	r := ss.Get("magnetic_field").Read()
	if r.OK && len(r.V) >= 3 {
		f := math.Sqrt(r.V[0]*r.V[0] + r.V[1]*r.V[1] + r.V[2]*r.V[2])
		field = fmt.Sprintf("  %.0fuT", f)
		if s := ss.Get("location"); s != nil {
			if lr := s.Read(); lr.OK && len(lr.V) >= 8 && lr.V[7] > 0 {
				field = fmt.Sprintf("  %.0f/%.0fuT", f, lr.V[7])
				if f > lr.V[7]*1.35 || f < lr.V[7]*0.65 {
					problem, warn = "  DISTURBED: magnet/metal?", true
				}
			}
		}
		if len(r.V) >= 4 && r.V[3] < 2 {
			problem += "  calibrate: figure 8"
			warn = true
		}
	}
	// Drop details, least important first, until it fits.
	for _, l := range []string{north + decl + field + problem, north + field + problem, north + problem, problem} {
		if len(l) <= width {
			return l, warn
		}
	}
	return (north + problem)[:width], warn
}
