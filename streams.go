package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/TomoBossi/sensord/client"
	"github.com/TomoBossi/sensord/proto"
)

// histLen is how many past readings each stream keeps, for scopes and trails.
const histLen = 512

// Stream is the client side of one subscription: the latest reading, a short
// history, and the rate actually delivered.
type Stream struct {
	Spec   string // as requested: a type or exact name
	Sensor string // resolved name
	Info   proto.Sensor
	sub    *client.Subscription

	mu      sync.Mutex
	last    client.Event
	has     bool
	count   int       // events received
	arrived time.Time // wall time of the last event
	avgDt   float64   // smoothed interval between events, s
	hist    [histLen][]float64
	histN   int
}

// Reading is a snapshot of a stream for one frame.
type Reading struct {
	V       []float64
	T       int64
	OK      bool
	Count   int
	Hz      float64
	Arrived time.Time
}

func (s *Stream) Read() Reading {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := Reading{V: s.last.V, T: s.last.T, OK: s.has, Count: s.count, Arrived: s.arrived}
	if s.avgDt > 0 {
		r.Hz = 1 / s.avgDt
	}
	return r
}

// History returns up to n past readings of value index i, oldest first.
func (s *Stream) History(i, n int) []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	n = min(n, s.histN, histLen)
	out := make([]float64, 0, n)
	for k := s.histN - n; k < s.histN; k++ {
		v := s.hist[k%histLen]
		if i < len(v) {
			out = append(out, v[i])
		} else {
			out = append(out, 0)
		}
	}
	return out
}

func (s *Stream) push(ev client.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.has && ev.T > s.last.T {
		dt := float64(ev.T-s.last.T) / 1e9
		if s.avgDt == 0 {
			s.avgDt = dt
		} else {
			s.avgDt += (dt - s.avgDt) / 16
		}
	}
	s.last, s.has = ev, true
	s.count++
	s.arrived = time.Now()
	s.hist[s.histN%histLen] = ev.V
	s.histN++
}

// Streams holds every subscription of a run, keyed by the spec it was asked for.
type Streams struct {
	c       *client.Client
	sensors []proto.Sensor
	byKey   map[string]*Stream
	mock    map[string][]float64 // by sensor type; non-nil means mock mode
}

func OpenStreams() (*Streams, error) {
	c, err := client.Dial("")
	if err != nil {
		return nil, err
	}
	sensors, err := c.Sensors()
	if err != nil {
		c.Close()
		return nil, err
	}
	return &Streams{c: c, sensors: sensors, byKey: map[string]*Stream{}}, nil
}

func (ss *Streams) Close() {
	if ss.c != nil {
		ss.c.Close()
	}
}

// Lookup finds a sensor by exact name or by type (the default of that type),
// the same way sensord resolves it.
func (ss *Streams) Lookup(spec string) (proto.Sensor, bool) {
	for _, s := range ss.sensors {
		if equalFold(s.Name, spec) {
			return s, true
		}
	}
	for _, s := range ss.sensors {
		if s.Default && equalFold(s.Type, spec) {
			return s, true
		}
	}
	return proto.Sensor{}, false
}

// Subscribe starts a stream at hz (0 = every event). Subscribing the same spec
// twice returns the existing stream.
func (ss *Streams) Subscribe(spec string, hz float64) (*Stream, error) {
	if s := ss.byKey[spec]; s != nil {
		return s, nil
	}
	if ss.mock != nil {
		return ss.subscribeMock(spec)
	}
	sub, err := ss.c.Subscribe(spec, hz)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", spec, err)
	}
	info, _ := ss.Lookup(sub.Sensor)
	s := &Stream{Spec: spec, Sensor: sub.Sensor, Info: info, sub: sub}
	ss.byKey[spec] = s
	go func() {
		for ev := range sub.C {
			s.push(ev)
		}
	}()
	return s, nil
}

// Unsubscribe stops the stream under spec, so sensord can power the sensor
// down if nobody else reads it. Get returns nil for it afterwards.
func (ss *Streams) Unsubscribe(spec string) {
	s := ss.byKey[spec]
	if s == nil {
		return
	}
	delete(ss.byKey, spec)
	if s.sub != nil {
		s.sub.Close()
	}
}

// Get returns the stream subscribed under spec, or nil.
func (ss *Streams) Get(spec string) *Stream { return ss.byKey[spec] }

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
