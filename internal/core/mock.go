package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/TomoBossi/sensord/client"
	"github.com/TomoBossi/sensord/proto"
)

// Mock readings, for rendering demos without the phone or sensord:
//
//	sensordemo --snapshot 70x30 --mock 'accelerometer=3,9,1' fluid
//	sensordemo --snapshot 70x30 --mock 'orient=30,-20,0' donut
//	sensordemo --snapshot 70x30 --mock 'CHOP_CHOP=1' gestures
//
// "SENSOR=v1,v2,..." gives a constant reading; continuous sensors repeat it at
// 60 Hz, event sensors deliver it once. "orient=yaw,pitch,roll" (degrees) sets
// every rotation vector to that orientation.

// mockSensors mirrors the moto g17 power's sensor list, closely enough for
// demos: type names, modes and rates.
var mockSensors = []proto.Sensor{
	{Name: "bmi3xy_acc", Type: "accelerometer", MaxHz: 400, Mode: "continuous", Default: true},
	{Name: "qmc6308", Type: "magnetic_field", MaxHz: 50, Mode: "continuous", Default: true},
	{Name: "UNCALI_MAG", Type: "magnetic_field_uncalibrated", MaxHz: 50, Mode: "continuous", Default: true},
	{Name: "bmi3xy_gyro", Type: "gyroscope", MaxHz: 400, Mode: "continuous", Default: true},
	{Name: "ltr569_l", Type: "light", Mode: "on-change", Default: true},
	{Name: "ltr569_p", Type: "proximity", Mode: "on-change", Default: true, Wakeup: true},
	{Name: "GRAVITY", Type: "gravity", MaxHz: 200, Mode: "continuous", Default: true},
	{Name: "LINEARACCEL", Type: "linear_acceleration", MaxHz: 200, Mode: "continuous", Default: true},
	{Name: "ROTATION_VECTOR", Type: "rotation_vector", MaxHz: 200, Mode: "continuous", Default: true},
	{Name: "GAME_ROTATION_VECTOR", Type: "game_rotation_vector", MaxHz: 200, Mode: "continuous", Default: true},
	{Name: "GEOMAGNETIC_ROTATION_VECTOR", Type: "geomagnetic_rotation_vector", MaxHz: 200, Mode: "continuous", Default: true},
	{Name: "STEP_COUNTER", Type: "step_counter", Mode: "on-change", Default: true},
	{Name: "STEP_DETECTOR", Type: "step_detector", Mode: "special", Default: true},
	{Name: "SIGNIFICANT_MOTION", Type: "significant_motion", Mode: "one-shot", Default: true, Wakeup: true},
	{Name: "TILT_DETECTOR", Type: "tilt_detector", Mode: "special", Default: true, Wakeup: true},
	{Name: "WAKE_GESTURE", Type: "wake_gesture", Mode: "one-shot", Default: true, Wakeup: true},
	{Name: "CHOP_CHOP", Type: "chop_chop", Mode: "one-shot", Wakeup: true},
	{Name: "FLIP_TWIST", Type: "flip_twist", Mode: "one-shot", Wakeup: true},
	{Name: "FLIP", Type: "flip", Mode: "on-change", Wakeup: true},
	{Name: "SIGNIFICANT_MOVE", Type: "significant_move", Mode: "one-shot", Wakeup: true},
	{Name: "location", Type: "location", MaxHz: 1, Mode: "continuous", Default: true, Wakeup: true},
	{Name: "gps", Type: "gps", MaxHz: 1, Mode: "continuous", Default: true, Wakeup: true},
	{Name: "display_rotation", Type: "display_rotation", Mode: "on-change", Default: true, Wakeup: true},
	{Name: "REAR_ALS", Type: "rearals", Mode: "on-change", Default: true, Wakeup: true},
	{Name: "REAR_FLK", Type: "rearflk", Mode: "on-change", Wakeup: true},
	{Name: "battery", Type: "battery", MaxHz: 4, Mode: "continuous", Default: true},
	{Name: "thermal", Type: "thermal", MaxHz: 1, Mode: "continuous", Default: true},
	{Name: "flashlight", Type: "flashlight", Mode: "on-change", Default: true, Wakeup: true},
	{Name: "screen", Type: "screen", Mode: "on-change", Default: true, Wakeup: true},
	{Name: "wifi", Type: "wifi", MaxHz: 1, Mode: "continuous", Default: true},
	{Name: "cell", Type: "cell", Mode: "on-change", Default: true, Wakeup: true},
}

// OpenMock returns Streams that serve fixed readings instead of sensord.
func OpenMock(spec string) (*Streams, error) {
	ss := &Streams{sensors: mockSensors, byKey: map[string]*Stream{}, mock: map[string][]float64{}}
	for _, part := range strings.Split(spec, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, vals, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("mock %q: want SENSOR=v1,v2,...", part)
		}
		var v []float64
		for _, f := range strings.Split(vals, ",") {
			x, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
			if err != nil {
				return nil, fmt.Errorf("mock %q: %v", part, err)
			}
			v = append(v, x)
		}
		if name == "orient" {
			q := QuatFromEuler(v)
			for _, t := range []string{"rotation_vector", "game_rotation_vector", "geomagnetic_rotation_vector"} {
				ss.mock[t] = q
			}
			continue
		}
		info, ok := ss.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("mock: unknown sensor %q", name)
		}
		ss.mock[info.Type] = v
	}
	return ss, nil
}

// subscribeMock feeds a stream from the mock readings.
func (ss *Streams) subscribeMock(spec string) (*Stream, error) {
	info, ok := ss.Lookup(spec)
	if !ok {
		return nil, fmt.Errorf("%s: unknown sensor", spec)
	}
	s := &Stream{Spec: spec, Sensor: info.Name, Info: info}
	ss.byKey[spec] = s
	v, ok := ss.mock[info.Type]
	if !ok {
		return s, nil // subscribed, but silent
	}
	go func() {
		start := time.Now()
		for {
			s.Push(client.Event{T: time.Since(start).Nanoseconds() + 1, V: v})
			if info.Mode != "continuous" {
				return
			}
			time.Sleep(time.Second / 60)
		}
	}()
	return s, nil
}

// QuatFromEuler builds a rotation-vector reading (x, y, z, w) from yaw
// (about world up), pitch (about device x) and roll (about device y), degrees.
func QuatFromEuler(v []float64) []float64 {
	for len(v) < 3 {
		v = append(v, 0)
	}
	rad := math.Pi / 180
	m := RotZ(v[0] * rad).Mul(RotX(v[1] * rad)).Mul(RotY(v[2] * rad))
	// Matrix to quaternion (m is well conditioned for these angles).
	w := math.Sqrt(math.Max(0, 1+m[0][0]+m[1][1]+m[2][2])) / 2
	if w < 1e-6 {
		return []float64{1, 0, 0, 0}
	}
	return []float64{(m[2][1] - m[1][2]) / (4 * w), (m[0][2] - m[2][0]) / (4 * w), (m[1][0] - m[0][1]) / (4 * w), w}
}
