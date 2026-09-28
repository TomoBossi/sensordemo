package kaleidoscope

import (
	"math"
	"strings"
	"testing"

	"github.com/TomoBossi/sensord/client"
	. "github.com/TomoBossi/sensordemo/internal/core"
)

func kalFrame(k *kaleidoscope, ss *Streams, f *Frame, gz float64, frames int, now *float64) string {
	for i := 0; i < frames; i++ {
		// Held still, a little sensor noise; turning, a clean turn (so a
		// turn and its reverse are exactly opposite).
		noise := 0.0
		if gz == 0 {
			noise = 0.02 * math.Sin(float64(i)*2.7)
		}
		ss.Get("gyroscope").Push(clientEvent([]float64{noise, -noise, gz + noise}))
		*now += 1.0 / 30
		f.Resize(64, 40)
		k.Draw(f.View(0, 0, 64, 40), ss, *now, 1.0/30)
	}
	var b strings.Builder
	for y := 0; y < f.H; y++ {
		b.WriteString(f.Row(y))
	}
	return b.String()
}

// Held still (only sensor noise), it doesn't move; a turn changes the
// pattern, and turning back brings it back. And it's mirror symmetric.
func TestKaleidoscopeStillAndReversible(t *testing.T) {
	ss := FakeStreams("gyroscope")
	k := &kaleidoscope{seed: 3}
	k.fill()
	var f Frame
	now := 0.0
	a := kalFrame(k, ss, &f, 0, 60, &now)
	b := kalFrame(k, ss, &f, 0, 90, &now)
	if a != b {
		t.Error("the pattern moved while the phone was still")
	}
	c := kalFrame(k, ss, &f, 1.0, 30, &now) // a quarter-ish turn
	c = kalFrame(k, ss, &f, 0, 120, &now)
	if c == a {
		t.Error("turning didn't change the pattern")
	}
	d := kalFrame(k, ss, &f, -1.0, 30, &now) // and back
	d = kalFrame(k, ss, &f, 0, 120, &now)
	if d != a {
		t.Error("turning back didn't bring the pattern back")
	}
	// Left-right and top-bottom mirror symmetry.
	at := func(x, y int) byte { c, _ := f.Cell(x, y); return c }
	for y := 0; y < 40; y++ {
		for x := 0; x < 64; x++ {
			if at(x, y) != at(63-x, y) || at(x, y) != at(x, 39-y) {
				t.Fatalf("not symmetric at %d,%d", x, y)
			}
		}
	}
}

func BenchmarkKaleidoscope(b *testing.B) {
	for _, turning := range []bool{true, false} {
		b.Run(map[bool]string{true: "turning", false: "still"}[turning], func(b *testing.B) {
			ss := FakeStreams("gyroscope")
			k := &kaleidoscope{seed: 3}
			k.fill()
			var f Frame
			f.Resize(150, 90)
			gz := 0.0
			if turning {
				gz = 0.8
			}
			ss.Get("gyroscope").Push(clientEvent([]float64{0, 0, gz}))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f.Resize(150, 90)
				k.Draw(f.View(0, 0, 150, 90), ss, float64(i)/30, 1.0/30)
			}
		})
	}
}

func clientEvent(v []float64) client.Event { return client.Event{T: 1, V: v} }
