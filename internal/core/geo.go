package core

import (
	"encoding/json"
	"fmt"
	"math"
)

type LatLon struct{ Lat, Lon float64 }

// bearing is the initial great-circle bearing from a to b, radians clockwise
// from true north; distance in meters (haversine).
func BearingDistance(a, b LatLon) (float64, float64) {
	const R = 6371000.0
	la1, la2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dl := (b.Lon - a.Lon) * math.Pi / 180
	y := math.Sin(dl) * math.Cos(la2)
	x := math.Cos(la1)*math.Sin(la2) - math.Sin(la1)*math.Cos(la2)*math.Cos(dl)
	s := math.Sin((la2-la1)/2)*math.Sin((la2-la1)/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dl/2)*math.Sin(dl/2)
	return math.Atan2(y, x), 2 * R * math.Asin(math.Sqrt(s))
}

func FormatDistance(m float64) string {
	switch {
	case m < 1000:
		return fmt.Sprintf("%.0f m", m)
	case m < 10000:
		return fmt.Sprintf("%.1f km", m/1000)
	}
	return fmt.Sprintf("%.0f km", m/1000)
}

func MetersBetween(a, b LatLon) float64 {
	dy := (a.Lat - b.Lat) * 110540
	dx := (a.Lon - b.Lon) * 111320 * math.Cos(a.Lat*math.Pi/180)
	return math.Hypot(dx, dy)
}

var CompassPoints = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

func (l *LatLon) UnmarshalJSON(b []byte) error {
	var v struct{ Lat, Lon float64 }
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	l.Lat, l.Lon = v.Lat, v.Lon
	return nil
}
