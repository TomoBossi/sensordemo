package main

import (
	"os"
	"testing"
	"time"
)

// TestFetch warms the cache for a public test point (OSM_FETCH=1).
func TestFetch(t *testing.T) {
	if os.Getenv("OSM_FETCH") == "" {
		t.Skip("network")
	}
	start := time.Now()
	ways, err := fetchWays(latLon{-34.6037, -58.3816}, 400)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[wayKind]int{}
	for _, w := range ways {
		counts[w.kind]++
	}
	t.Logf("%d ways in %v: %v", len(ways), time.Since(start), counts)
}
