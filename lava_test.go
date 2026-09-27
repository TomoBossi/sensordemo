package main

import (
	"math"
	"math/rand"
	"testing"
)

func lavaWax(l *lavalamp) float64 {
	w := l.pool
	for _, b := range l.blobs {
		w += b.vol
	}
	return w
}

// The wax cycles all the way up without lingering there: over four
// minutes, blob after blob grows out of the pool, rises near the top,
// spends only a few seconds up there, sinks and melts back in; no wax is
// lost, and the bottle is never short of blobs.
func TestLavaCycles(t *testing.T) {
	l := &lavalamp{rng: rand.New(rand.NewSource(2))}
	l.fillWax()
	start := lavaWax(l)
	span := lavaCapBot - lavaBaseTop
	reached := map[int]bool{}
	upTime := map[int]float64{}
	melted, crowd := 0, 0
	const frames = 30 * 240
	for f := 0; f < frames; f++ {
		for s := 0; s < 3; s++ {
			l.step(1.0/90, float64(f)/30)
		}
		free := 0
		for _, b := range l.blobs {
			if b.melting {
				melted++ // counted once per frame while melting; normalized below
			}
			if b.target == 0 && !b.melting {
				free++
				if (b.y-lavaBaseTop)/span > 0.85 {
					reached[b.id] = true
					upTime[b.id] += 1.0 / 30
				}
			}
		}
		crowd += free
	}
	total := 0.0
	for _, s := range upTime {
		total += s
	}
	avgUp := total / math.Max(1, float64(len(upTime)))
	t.Logf("%d blobs reached the top, %.1f s up there on average, %.1f free blobs on average", len(reached), avgUp, float64(crowd)/frames)
	if len(reached) < 15 {
		t.Errorf("only %d blobs reached the top in four minutes", len(reached))
	}
	if avgUp > 8 {
		t.Errorf("blobs linger at the top %.1f s on average", avgUp)
	}
	if avg := float64(crowd) / frames; avg < 5 {
		t.Errorf("only %.1f blobs in the bottle on average", avg)
	}
	if melted == 0 {
		t.Error("nothing melted back")
	}
	if end := lavaWax(l); math.Abs(end-start) > start*0.001 {
		t.Errorf("wax %.5f became %.5f", start, end)
	}
}

// A shake splits the wax into more, smaller blobs, which later merge
// again; no wax is lost either way.
func TestLavaSplitAndMerge(t *testing.T) {
	l := &lavalamp{rng: rand.New(rand.NewSource(5))}
	l.fillWax()
	for f := 0; f < 30*40; f++ {
		l.step(1.0/30, float64(f)/30)
	}
	start := lavaWax(l)
	before := len(l.blobs)
	l.split(Vec3{0, 1, 0}, 40)
	after := len(l.blobs)
	if after <= before {
		t.Fatalf("a shake left %d blobs (was %d)", after, before)
	}
	least := after
	for f := 0; f < 30*30; f++ {
		l.step(1.0/30, 40+float64(f)/30)
		least = min(least, len(l.blobs))
	}
	t.Logf("%d blobs, %d after the shake, down to %d as they merged and melted", before, after, least)
	if least >= after {
		t.Error("the pieces never merged or melted back")
	}
	if end := lavaWax(l); math.Abs(end-start) > start*0.001 {
		t.Errorf("wax %.5f became %.5f", start, end)
	}
}
