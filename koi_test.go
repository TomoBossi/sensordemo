package main

import (
	"math"
	"math/rand"
	"testing"
)

func testPond(t testing.TB, mock string, seed int64) (*pond, *Streams) {
	ss, _ := OpenMock(mock)
	p := &pond{}
	p.Setup(ss)
	p.rng = rand.New(rand.NewSource(seed))
	p.build(88, 75)
	return p, ss
}

func (p *pond) tick(ss *Streams, t, dt float64) {
	p.sense(ss, dt)
	p.ripple(dt / 2)
	p.ripple(dt / 2)
	p.swim(dt, t)
	p.drift(dt)
}

// The koi keep to the water, and so do the pads.
func TestKoiStayInPond(t *testing.T) {
	for seed := int64(1); seed <= 4; seed++ {
		p, ss := testPond(t, "linear_acceleration=0,0,0;proximity=5", seed)
		for i := 0; i < 30*120; i++ {
			p.tick(ss, float64(i)/30, 1.0/30)
			for fi, f := range p.fish {
				if e := p.edgeAt(f.pts[0][0], f.pts[0][1]); e > 0 {
					t.Fatalf("seed %d, %.1f s: koi %d's head is %.1f onto the bank", seed, float64(i)/30, fi, e)
				}
			}
		}
		for _, pd := range p.pads {
			if e := p.edgeAt(pd.x, pd.y); e > -pd.r*0.3 {
				t.Errorf("seed %d: a pad drifted onto the bank (%.1f)", seed, e)
			}
		}
	}
}

// Covering the phone sends them away from the middle and down.
func TestKoiScare(t *testing.T) {
	p, ss := testPond(t, "linear_acceleration=0,0,0;proximity=0", 3)
	for i := 0; i < 30*6; i++ {
		p.tick(ss, float64(i)/30, 1.0/30)
	}
	cx, cy := float64(p.W)/2, float64(p.H)
	var near, depth float64
	for _, f := range p.fish {
		near += math.Hypot((f.pts[0][0]-cx)/cx, (f.pts[0][1]-cy)/cy)
		depth += f.depth
	}
	near /= float64(len(p.fish))
	depth /= float64(len(p.fish))
	t.Logf("scared: mean distance out %.2f, depth %.2f", near, depth)
	if near < 0.45 || depth < 0.8 {
		t.Errorf("scared koi at %.2f out, %.2f deep", near, depth)
	}
}

// Food scattered on the water gets eaten.
func TestKoiEat(t *testing.T) {
	p, ss := testPond(t, "linear_acceleration=0,0,0;proximity=5", 5)
	p.feed()
	n := len(p.food)
	for i := 0; i < 30*40 && len(p.food) > 0; i++ {
		p.tick(ss, float64(i)/30, 1.0/30)
	}
	if len(p.food) > 0 {
		t.Errorf("%d of %d pellets left after 40 s", len(p.food), n)
	}
}

func BenchmarkKoi(b *testing.B) {
	ss, _ := OpenMock("linear_acceleration=0,0,0;light=200")
	p := &pond{}
	p.Setup(ss)
	var f Frame
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Resize(88, 75)
		p.Draw(f.View(0, 0, 88, 75), ss, float64(i)/30, 1.0/30)
	}
}
