package main

import (
	"math/rand"
	"testing"
)

// The wax cycles: over four minutes, blob after blob pinches off the pool,
// rises high in the bottle, sinks and melts back in, and the wax is
// conserved.
func TestLavaCycles(t *testing.T) {
	l := &lavalamp{rng: rand.New(rand.NewSource(2)), up: [2]float64{0, 1}}
	l.layout(64, 44)
	l.fillWax()
	wax := func() float64 {
		w := l.pool
		for _, b := range l.blobs {
			w += b.r * b.r
		}
		return w
	}
	start := wax()
	span := l.y1 - l.y0
	highest := map[*blob]bool{}
	rose, melted := 0, 0
	seen := map[float64]bool{} // blobs by radius, to count each once
	for f := 0; f < 30*240; f++ {
		n := len(l.blobs)
		for s := 0; s < 3; s++ {
			l.step(1.0/90, [2]float64{}, float64(f)/30)
		}
		if len(l.blobs) < n {
			melted += n - len(l.blobs)
		}
		for i := range l.blobs {
			b := &l.blobs[i]
			if (b.y-l.y0)/span > 0.7 && !seen[b.r] {
				seen[b.r] = true
				rose++
			}
		}
	}
	_ = highest
	t.Logf("%d blobs rose high, %d melted back in", rose, melted)
	if rose < 10 || melted < 10 {
		t.Errorf("only %d rose high and %d melted back in four minutes", rose, melted)
	}
	if end := wax(); end < start*0.999 || end > start*1.001 {
		t.Errorf("wax %.1f became %.1f", start, end)
	}
}
