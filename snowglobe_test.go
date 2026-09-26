package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func settledGlobe(t testing.TB) (*snowglobe, *Streams) {
	g := &snowglobe{}
	ss, _ := OpenMock("gravity=0,9.8,0;linear_acceleration=0,0,0")
	g.Setup(ss)
	g.build()
	g.layout(70, 40)
	for i := 0; i < 30*40; i++ { // 40 s
		g.sense(ss, 1.0/30)
		g.simulate(1.0/30, float64(i)/30)
	}
	return g, ss
}

// Everything settles when left alone, and a hard shake lifts most of it
// back up, spread through the whole globe.
func TestSnowglobeShake(t *testing.T) {
	for seed := int64(1); seed <= 5; seed++ {
		shakeOnce(t, seed)
	}
}

func shakeOnce(t *testing.T, seed int64) {
	g, ss := settledGlobe(t)
	g.rng = rand.New(rand.NewSource(seed))
	if n := len(g.flk); n > sgFlakes/20 {
		t.Fatalf("%d flakes still up after 40 s", n)
	}
	for i := 0; i < 45; i++ { // 1.5 s of shaking, then 1 s of swirling
		g.shake = Vec3{20 * math.Sin(float64(i)), 12 * math.Cos(float64(i)*1.3), 0}
		g.agitate(1.0 / 30)
		g.spawnFor(1.0 / 30)
		g.simulate(1.0/30, float64(i)/30)
	}
	for i := 0; i < 30; i++ {
		g.sense(ss, 1.0/30)
		g.simulate(1.0/30, float64(i)/30)
	}
	// Spread through the globe a second after the shake: no clump above
	// the lowest band, and plenty still up in the top half.
	up := len(g.flk)
	var grid [5][5]int
	top := 0
	for _, f := range g.flk {
		grid[min(4, int((1-f.p[1])*2.5))][min(4, int((f.p[0]+1)*2.5))]++
		top += boolIdx(f.p[1] > 0)
	}
	t.Logf("seed %d: %d of %d flakes up, %d in the top half; %v", seed, up, sgFlakes, top, grid)
	if up < sgFlakes*7/10 || top < up/5 {
		t.Errorf("seed %d: %d up, %d in the top half", seed, up, top)
	}
	for _, row := range grid[:3] { // the lowest band holds snow already settling back
		for _, n := range row {
			if n > up/5 {
				t.Errorf("seed %d: clumped: %v", seed, grid)
			}
		}
	}
	total := 0.0
	for _, f := range g.flk {
		total += f.m / sgMass
	}
	for _, d := range g.snow {
		total += d / sgMass
	}
	if math.Abs(total-sgFlakes) > 1 {
		t.Errorf("snow not conserved: %.1f", total)
	}
}

func BenchmarkSnowglobeFrame(b *testing.B) {
	g := &snowglobe{}
	ss, _ := OpenMock("gravity=0,9.8,0;linear_acceleration=0,0,0")
	g.Setup(ss)
	var f Frame
	f.Resize(150, 90)
	v := f.View(0, 0, 150, 90)
	g.Draw(v, ss, 0, 1.0/30)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Draw(v, ss, float64(i)/30, 1.0/30)
	}
}

// A frame while the phone turns: the picture is ray marched again.
func BenchmarkSnowglobeTurning(b *testing.B) {
	for _, size := range [][2]int{{70, 40}, {150, 90}} {
		b.Run(fmt.Sprint(size[0], "x", size[1]), func(b *testing.B) {
			g := &snowglobe{}
			ss, _ := OpenMock("gravity=0,9.8,0;linear_acceleration=0,0,0")
			g.Setup(ss)
			var f Frame
			f.Resize(size[0], size[1])
			v := f.View(0, 0, size[0], size[1])
			g.Draw(v, ss, 0, 1.0/30)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				g.drawn[0] = math.NaN() // as if the view moved
				g.Draw(v, ss, float64(i)/30, 1.0/30)
			}
		})
	}
}
