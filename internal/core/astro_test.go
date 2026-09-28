package core

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestSunAndPlanets(t *testing.T) {
	jd := Julian(time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC))
	earth := Heliocentric(2, jd)
	ra, dec := EclipticToRADec(earth.Scale(-1))
	fmt.Printf("Sun      RA %5.2fh  Dec %+6.2f\n", ra/15, dec)
	if math.Abs(ra/15-12.17) > 0.15 || math.Abs(dec+1.0) > 0.6 {
		t.Errorf("Sun off: RA %.2fh Dec %.2f", ra/15, dec)
	}
	for i, p := range PlanetElements {
		if i == 2 {
			continue
		}
		ra, dec := EclipticToRADec(Heliocentric(i, jd).Sub(earth))
		fmt.Printf("%-8s RA %5.2fh  Dec %+6.2f\n", p.Name, ra/15, dec)
	}
	mra, mdec, lit := MoonPosition(jd)
	fmt.Printf("Moon     RA %5.2fh  Dec %+6.2f  %.0f%% lit\n", mra/15, mdec, lit*100)
}
