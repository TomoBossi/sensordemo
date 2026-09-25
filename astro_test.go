package main

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestSunAndPlanets(t *testing.T) {
	jd := julian(time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC))
	earth := heliocentric(2, jd)
	ra, dec := eclipticToRADec(earth.Scale(-1))
	fmt.Printf("Sun      RA %5.2fh  Dec %+6.2f\n", ra/15, dec)
	if math.Abs(ra/15-12.17) > 0.15 || math.Abs(dec+1.0) > 0.6 {
		t.Errorf("Sun off: RA %.2fh Dec %.2f", ra/15, dec)
	}
	for i, p := range planetElements {
		if i == 2 {
			continue
		}
		ra, dec := eclipticToRADec(heliocentric(i, jd).Sub(earth))
		fmt.Printf("%-8s RA %5.2fh  Dec %+6.2f\n", p.name, ra/15, dec)
	}
	mra, mdec, lit := moonPosition(jd)
	fmt.Printf("Moon     RA %5.2fh  Dec %+6.2f  %.0f%% lit\n", mra/15, mdec, lit*100)
}
