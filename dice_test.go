package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// Thrown, every kind of die comes to rest lying flat on a face (or cocked
// against a wall), inside the tray and clear of the others, and reads a
// valid number.
func TestDiceSettle(t *testing.T) {
	for _, spec := range []string{"2d6", "1d4+1d8", "1d10+1d12+1d20", "6d6", "3d20"} {
		for seed := int64(1); seed <= 8; seed++ {
			kinds, err := parseDice(spec)
			if err != nil {
				t.Fatal(err)
			}
			d := &dice{kinds: kinds, rng: rand.New(rand.NewSource(seed))}
			d.layout(64, 40)
			frames := 0
			for ; frames < 30*12; frames++ {
				for s := 0; s < diceSteps; s++ {
					d.physics(1.0 / 30 / diceSteps)
				}
				all := true
				for _, dd := range d.dice {
					all = all && dd.asleep
				}
				if all {
					break
				}
			}
			for i, dd := range d.dice {
				cocked := math.Abs(dd.p[0])+dd.k.radius > d.W || math.Abs(dd.p[2])+dd.k.radius > d.D
				if !dd.asleep || !dd.flat() && !cocked {
					t.Errorf("%s seed %d: die %d not resting flat after %d frames (v %.1f w %.1f)", spec, seed, i, frames, dd.v.Len(), dd.w.Len())
				}
				if math.Abs(dd.p[0]) > d.W || math.Abs(dd.p[2]) > d.D || dd.p[1] < 0 {
					t.Errorf("%s seed %d: die %d outside the tray at %v", spec, seed, i, dd.p)
				}
				v := dd.value()
				if v < 1 || v > dd.k.sides {
					t.Errorf("%s seed %d: die %d reads %d", spec, seed, i, v)
				}
				for j := i + 1; j < len(d.dice); j++ {
					if gap := d.dice[j].p.Sub(dd.p).Len(); gap < 0.9*(dd.k.body+d.dice[j].k.body) {
						t.Errorf("%s seed %d: dice %d and %d overlap (%.2f apart)", spec, seed, i, j, gap)
					}
				}
			}
		}
	}
}

// Every face number appears once, and opposite faces add up.
func TestDiceLabels(t *testing.T) {
	for _, s := range []int{6, 8, 10, 12, 20} {
		k := newDieKind(s)
		seen := map[int]bool{}
		for i, l := range k.label {
			seen[l] = true
			for j, n := range k.n {
				if k.n[i].Dot(n) < -0.999 {
					want := s + 1
					if s == 10 {
						want = 9
					}
					if l+k.label[j] != want {
						t.Errorf("d%d: faces %d and %d opposite, labels %d and %d", s, i, j, l, k.label[j])
					}
				}
			}
		}
		if len(seen) != s || len(k.n) != s {
			t.Errorf("d%d: %d faces, %d distinct labels", s, len(k.n), len(seen))
		}
	}
	_ = fmt.Sprint
}

// A hard shake throws every die, every time: none stays where it lay.
func TestDiceShakeThrowsAll(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		ss := &Streams{byKey: map[string]*Stream{"linear_acceleration": {}}}
		kinds, _ := parseDice("2d6+1d20")
		d := &dice{kinds: kinds, rng: rand.New(rand.NewSource(seed)), light: diceLight}
		var f Frame
		f.Resize(64, 40)
		v := f.View(0, 0, 64, 40)
		now := 0.0
		frame := func(la [3]float64) {
			ss.Get("linear_acceleration").push(clientEvent(la[:]))
			now += 1.0 / 30
			d.Draw(v, ss, now, 1.0/30)
		}
		for i := 0; i < 30*25; i++ { // settle (the dice's time runs slow)
			frame([3]float64{})
		}
		type pose struct {
			p Vec3
			R Mat3
		}
		var before []pose
		for _, dd := range d.dice {
			if !dd.asleep {
				t.Fatalf("seed %d: not settled", seed)
			}
			before = append(before, pose{dd.p, dd.R})
		}
		for i := 0; i < 15; i++ { // half a second of shaking, up to 20 m/s^2
			a := 20 * math.Sin(float64(i)*1.3)
			frame([3]float64{a, a * 0.6, a * 0.3})
		}
		for i := 0; i < 30*3; i++ {
			frame([3]float64{})
		}
		for i, dd := range d.dice {
			moved := dd.p.Sub(before[i].p).Len()
			// How far it turned: the angle of the rotation between the poses.
			rel := dd.R.Mul(before[i].R.T())
			turned := math.Acos(math.Max(-1, math.Min(1, (rel[0][0]+rel[1][1]+rel[2][2]-1)/2)))
			if moved < 1.5 && turned < 1 {
				t.Errorf("seed %d: die %d barely moved (%.2f units, %.0f degrees)", seed, i, moved, turned*180/math.Pi)
			}
		}
	}
}
