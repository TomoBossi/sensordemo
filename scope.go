package main

func init() {
	register(entry{
		name: "scope",
		desc: "oscilloscope for any sensor: every value as a scrolling trace",
		new:  func(specs []string) Demo { return &scope{specs: specs} },
	})
}

// scope is the fallback for sensors without a dedicated demo. Placeholder
// until the real one lands.
type scope struct{ specs []string }

func (s *scope) Setup(ss *Streams) ([]*Gauge, error)      { return nil, nil }
func (s *scope) Draw(v *View, ss *Streams, t, dt float64) {}
func (s *scope) Key(k byte)                               {}
func (s *scope) Help() []string                           { return nil }
