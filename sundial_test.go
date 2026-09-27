package main

import (
	"math"
	"testing"
	"time"
)

// The gnomon's shadow, cast by the Sun where it really is, falls along the
// hour line the dial has for the sun's time: north and south, through the
// year, morning and afternoon.
func TestSundialShadowOnHourLine(t *testing.T) {
	for _, lat := range []float64{-34.6, 51.5, 12, -60} {
		for _, day := range []int{0, 80, 172, 264, 355} {
			for hour := 6.0; hour <= 18; hour += 0.5 {
				tm := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day).Add(time.Duration((hour - 0.0) * float64(time.Hour)))
				sun, ha := sunAt(tm, lat, 0)
				if sun[1] < 0.05 {
					continue
				}
				s := &sundial{lat: lat}
				P := s.pole()
				// The style: from the root toward the pole, at the latitude's
				// angle. Its shadow's direction on the plate.
				a := math.Abs(lat) * math.Pi / 180
				d := P.Scale(math.Cos(a)).Add(Vec3{0, math.Sin(a), 0})
				sh := d.Sub(sun.Scale(d[1] / sun[1]))
				ge, gp := sh[0], sh.Dot(P)
				n := math.Hypot(ge, gp)
				ex, px := shadowDir(ha, lat)
				if math.Abs(ge/n-ex) > 0.01 || math.Abs(gp/n-px) > 0.01 {
					t.Fatalf("lat %v day %v %v h: shadow (%.3f, %.3f), hour line (%.3f, %.3f)", lat, day, hour, ge/n, gp/n, ex, px)
				}
			}
		}
	}
}

// The day's events come in order and noon is the sun's highest.
func TestSundialEvents(t *testing.T) {
	s := &sundial{lat: -34.60372, lon: -58.38159}
	loc := time.FixedZone("-03", -3*3600)
	s.events(time.Date(2026, 9, 27, 12, 0, 0, 0, loc))
	if !s.rise.Before(s.noon) || !s.noon.Before(s.set) {
		t.Fatalf("rise %v noon %v set %v", s.rise, s.noon, s.set)
	}
	// Near the equinox: about 12 hours of daylight, noon near 12:44 here.
	if l := s.set.Sub(s.rise).Hours(); l < 11.9 || l > 12.5 {
		t.Errorf("day %.2f h long", l)
	}
	if m := s.noon.Hour()*60 + s.noon.Minute(); m < 12*60+40 || m > 12*60+48 {
		t.Errorf("noon at %v", s.noon.Format("15:04"))
	}
	hi, _ := sunAt(s.noon, s.lat, s.lon)
	for _, dm := range []int{-30, 30} {
		o, _ := sunAt(s.noon.Add(time.Duration(dm)*time.Minute), s.lat, s.lon)
		if o[1] > hi[1] {
			t.Errorf("the sun is higher %d minutes from noon", dm)
		}
	}
}

func BenchmarkSundial(b *testing.B) {
	s := &sundial{speed: 1, lat: -34.60372, lon: -58.38159, az: 0.3, el: 0.9}
	s.sun, s.hourAng = sunAt(time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC), s.lat, s.lon)
	var f Frame
	for i := 0; i < b.N; i++ {
		f.Resize(88, 75)
		s.render(f.View(0, 0, 88, 75), 73)
	}
}

// A stream never subscribed reads as nothing yet, not a crash.
func TestUnsubscribedStreamReads(t *testing.T) {
	ss, _ := OpenMock("")
	if r := ss.Get("location").Read(); r.OK {
		t.Fatal("a reading from nowhere")
	}
	if h := ss.Get("location").History(0, 5); h != nil {
		t.Fatal("history from nowhere")
	}
	s := &sundial{speed: 1}
	var f Frame
	f.Resize(60, 30)
	s.Draw(f.View(0, 0, 60, 30), ss, 0, 1.0/30) // waits for a fix
}
