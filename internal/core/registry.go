package core

import (
	"fmt"
	"strings"
)

// Demo is one visualization. Setup subscribes to what it needs and returns the
// gauges for the data strip; Draw renders a frame into the space below it.
type Demo interface {
	Setup(ss *Streams) ([]*Gauge, error)
	Draw(v *View, ss *Streams, t, dt float64)
	Key(k byte)
	Help() []string
}

// Entry is a demo as registered: its name, a line about it, the sensor
// types that pick it, and how to make one.
type Entry struct {
	Name string
	Desc string
	Uses []string // sensor types that map to this demo
	New  func(specs []string) Demo
}

var Registry []Entry

// DemoArg is an optional second argument for the demo (such as the
// hourglass's duration).
var DemoArg string

func Register(e Entry) { Registry = append(Registry, e) }

func Find(name string) *Entry {
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

// PickDemo maps the command-line argument to a demo: a demo name, or one or more
// comma-separated sensors (names or types). For sensors, it picks the demo
// whose sensors cover all of them with the fewest extras, and falls back to
// the scope, which works for any sensor.
func PickDemo(arg string, ss *Streams) (*Entry, []string, error) {
	if e := Find(arg); e != nil {
		return e, nil, nil
	}
	specs := strings.Split(arg, ",")
	types := make([]string, len(specs))
	for i, sp := range specs {
		s, ok := ss.Lookup(sp)
		if !ok {
			return nil, nil, fmt.Errorf("%q is neither a demo nor a sensor (see: sensordemo list, sensord list)", sp)
		}
		types[i] = s.Type
	}
	var best *Entry
	for i := range Registry {
		e := &Registry[i]
		if len(e.Uses) == 0 || !covers(e.Uses, types) {
			continue
		}
		if best == nil || len(e.Uses) < len(best.Uses) {
			best = e
		}
	}
	if best == nil {
		best = Find("scope")
	}
	return best, specs, nil
}

func covers(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ScreenGauge subscribes to the display rotation, which every demo that
// uses orientation needs. It is not shown: it is plain from the screen
// itself. Older sensord versions don't have it; then the screen is assumed
// upright.
func ScreenGauge(ss *Streams) []*Gauge {
	ss.Subscribe("display_rotation", 0)
	return nil
}
