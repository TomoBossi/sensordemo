package main

import "fmt"

// axisInfo is how a sensor type's values are labeled and scaled.
type axisInfo struct {
	labels []string
	unit   string
	scale  func() Scale
}

func sym(max float64) func() Scale { return func() Scale { return &Symmetric{Max: max} } }
func fixed(max float64) func() Scale {
	return func() Scale { return &Symmetric{Max: max, Fixed: true} }
}
func lin(lo, hi float64) func() Scale { return func() Scale { return &Linear{Min: lo, Max: hi} } }

var xyz = []string{"x", "y", "z"}

// axes knows the common Android sensor types; anything else gets generic
// labels and an auto-ranging symmetric scale.
var axes = map[string]axisInfo{
	"accelerometer":               {xyz, "m/s2", fixed(15)},
	"accelerometer_uncalibrated":  {[]string{"x", "y", "z", "bias x", "bias y", "bias z"}, "m/s2", fixed(15)},
	"linear_acceleration":         {xyz, "m/s2", sym(3)},
	"gravity":                     {xyz, "m/s2", fixed(10)},
	"gyroscope":                   {xyz, "rad/s", sym(3)},
	"gyroscope_uncalibrated":      {[]string{"x", "y", "z", "drift x", "drift y", "drift z"}, "rad/s", sym(3)},
	"magnetic_field":              {xyz, "uT", sym(60)},
	"magnetic_field_uncalibrated": {[]string{"x", "y", "z", "bias x", "bias y", "bias z"}, "uT", sym(60)},
	"orientation":                 {[]string{"azimuth", "pitch", "roll"}, "deg", sym(180)},
	"rotation_vector":             {[]string{"x", "y", "z", "w", "acc"}, "", sym(1)},
	"game_rotation_vector":        {[]string{"x", "y", "z", "w"}, "", sym(1)},
	"geomagnetic_rotation_vector": {[]string{"x", "y", "z", "w", "acc"}, "", sym(1)},
	"light":                       {[]string{"light"}, "lux", func() Scale { return &Asymptotic{K: 300} }},
	"proximity":                   {[]string{"distance"}, "cm", lin(0, 5)},
	"step_counter":                {[]string{"steps"}, "", func() Scale { return &Relative{} }},
	"step_detector":               {[]string{"step"}, "", lin(0, 1)},
	"device_orientation":          {[]string{"orient"}, "", lin(0, 3)},
}

// short names for the data strip
var shortType = map[string]string{
	"accelerometer": "acc", "linear_acceleration": "lin", "gravity": "grav",
	"gyroscope": "gyro", "magnetic_field": "mag", "rotation_vector": "rot",
	"game_rotation_vector": "grot", "geomagnetic_rotation_vector": "georot",
}

func axisFor(typ string, i int) (label, unit string, scale Scale) {
	a, ok := axes[typ]
	if !ok {
		return fmt.Sprintf("v%d", i), "", &Symmetric{Max: 1}
	}
	label = fmt.Sprintf("v%d", i)
	if i < len(a.labels) {
		label = a.labels[i]
	}
	return label, a.unit, a.scale()
}

// gaugesFor makes one gauge per value of a stream, up to limit (0 = the
// type's known value count, or 3).
func gaugesFor(s *Stream, limit int) []*Gauge {
	n := limit
	if n == 0 {
		n = 3
		if a, ok := axes[s.Info.Type]; ok {
			n = len(a.labels)
		}
	}
	prefix := shortType[s.Info.Type]
	if prefix == "" {
		prefix = s.Info.Type
	}
	var out []*Gauge
	for i := 0; i < n; i++ {
		label, unit, scale := axisFor(s.Info.Type, i)
		if n > 1 || label == "v0" {
			label = prefix + " " + label
		}
		out = append(out, &Gauge{Spec: s.Spec, Index: i, Label: label, Unit: unit, Scale: scale})
	}
	return out
}
