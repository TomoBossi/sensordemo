package main

import (
	"math/rand"
	"testing"
)

// The wax cycles all the way up: over four minutes, blob after blob
// pinches off the pool, rises near the top of the bottle, sinks and melts
// back in, and no wax is lost.
func TestLavaCycles(t *testing.T) {
	l := &lavalamp{rng: rand.New(rand.NewSource(2))}
	l.fillWax()
	wax := func() float64 {
		w := l.pool
		for _, b := range l.blobs {
			w += b.r * b.r
		}
		return w
	}
	start := wax()
	span := lavaCapBot - lavaBaseTop
	seen := map[float64]bool{}
	high, melted := 0, 0
	for f := 0; f < 30*240; f++ {
		n := len(l.blobs)
		for s := 0; s < 3; s++ {
			l.step(1.0/90, Vec3{}, float64(f)/30)
		}
		if len(l.blobs) < n {
			melted += n - len(l.blobs)
		}
		for _, b := range l.blobs {
			if (b.y-lavaBaseTop)/span > 0.85 && !seen[b.r] {
				seen[b.r] = true
				high++
			}
		}
	}
	t.Logf("%d blobs reached the top, %d melted back in", high, melted)
	if high < 10 || melted < 10 {
		t.Errorf("in four minutes only %d reached the top and %d melted back", high, melted)
	}
	if end := wax(); end < start*0.999 || end > start*1.001 {
		t.Errorf("wax %.4f became %.4f", start, end)
	}
}
