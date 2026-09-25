package main

import "math"

// Sensors report in the phone's body frame (x right, y up, z out of the
// screen, in portrait). When Termux rotates, "up" on screen is a different
// body axis. The screen frame is the body frame turned about z by the display
// rotation that sensord reports.

// screenFrame is the matrix whose columns are the screen's x, y and z axes in
// body coordinates, for the current display rotation. At 90 degrees (phone
// turned counterclockwise, top to the left) screen x is body -y and screen y
// is body +x, as in Android's AccelerometerPlay sample.
func screenFrame(ss *Streams) Mat3 {
	return RotZ(-screenDegrees(ss) * math.Pi / 180)
}

func screenDegrees(ss *Streams) float64 {
	if s := ss.Get("display_rotation"); s != nil {
		if r := s.Read(); r.OK && len(r.V) > 0 {
			return r.V[0]
		}
	}
	return 0
}

// toScreen expresses a body-frame vector (such as an accelerometer reading)
// in screen coordinates.
func toScreen(ss *Streams, v []float64) Vec3 {
	var b Vec3
	copy(b[:], v)
	return screenFrame(ss).T().Apply(b)
}
