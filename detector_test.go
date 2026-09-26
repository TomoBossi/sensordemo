package main

import (
	"math"
	"testing"
)

// drive runs the detector on a field of the given strength for n frames.
func drive(d *detector, ss *Streams, v *View, x float64, n int, t *float64) {
	ss.Get("magnetic_field").push(clientEvent([]float64{x, 0, 0}))
	for i := 0; i < n; i++ {
		*t += 1.0 / 30
		d.Draw(v, ss, *t, 1.0/30)
	}
}

// Zeroing is a tare: zeroed next to steel (a strong field), a small change
// still moves the needle; bigger readings widen the scale, and zeroing
// again resets it.
func TestDetectorTare(t *testing.T) {
	ss := &Streams{byKey: map[string]*Stream{"magnetic_field": {}}}
	d := &detector{zeroing: zeroTime}
	var f Frame
	f.Resize(60, 30)
	v := f.View(0, 0, 60, 30)
	now := 0.0
	drive(d, ss, v, 180, 30, &now) // zeroes at 180 uT
	if !d.hasBase || math.Abs(d.base-180) > 0.01 {
		t.Fatalf("zero %.2f, want 180", d.base)
	}
	small := d.scale
	drive(d, ss, v, 181.5, 30, &now)
	if d.needle < 0.2 {
		t.Errorf("1.5 uT over a 180 uT zero: needle %.2f on a %.0f uT scale", d.needle, d.scale)
	}
	drive(d, ss, v, 260, 30, &now)
	if d.scale <= small || d.needle > 1 {
		t.Errorf("scale %.0f after a big reading (was %.0f)", d.scale, small)
	}
	d.Key('z')
	drive(d, ss, v, 260, 30, &now)
	if math.Abs(d.base-260) > 0.01 || d.scale != small || d.needle > 0.05 {
		t.Errorf("after zeroing again: zero %.1f scale %.0f needle %.2f", d.base, d.scale, d.needle)
	}
}
