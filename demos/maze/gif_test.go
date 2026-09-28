package maze

import (
	"math"
	"testing"

	. "github.com/TomoBossi/sensordemo/internal/core"
	"github.com/TomoBossi/sensordemo/internal/gifs"
)

// TestGIF renders this demo's GIF for the README (GIFS=all or GIFS=maze).
func TestGIF(t *testing.T) { gifs.Run(t, gifScene) }

// The maze: a hand tilting the phone rolls the ball along the way to the
// goal, across the bridges; a moment after the ball drops in, the board
// fades back to the start. The board is large at this size, so it runs at
// 1.6 times the speed.
func gifScene() gifs.Scene {
	const speed = 1.6
	var m *maze
	var gx, gy, last float64
	var solvedAt float64 = -1
	return gifs.Scene{Demo: "maze", Warm: 1, Secs: 60, Speed: speed, Fade: 0.7,
		Setup: func(t *testing.T, d Demo) { m = d.(*maze) },
		Until: func(d Demo, t float64) bool {
			if m.state == solved && solvedAt < 0 {
				solvedAt = t
			}
			// The banner a moment, well before the next board (the demo
			// waits winDur of its own time, which runs faster here).
			return solvedAt >= 0 && (t-solvedAt)*speed > winDur-1
		},
		Inputs: func(t float64) map[string][]float64 {
			ax, ay := 0.0, 0.0
			if m != nil && m.cells != nil && m.state == rolling {
				ax, ay = mazePilot(m)
			}
			k := math.Min(1, (t-last)*speed/0.15) // a hand, not a servo (in the demo's time)
			last = t
			gx += (ax - gx) * k
			gy += (ay - gy) * k
			// The board's pull is (-x, +y) of gravity on the screen.
			sx, sy := -gx/tiltK, gy/tiltK
			g := Vec3{sx, sy, math.Sqrt(math.Max(0, 9.81*9.81-sx*sx-sy*sy))}
			return map[string][]float64{"gravity": g[:], "accelerometer": g[:]}
		}}
}

// mazePilot is the pull (cells/s^2) that rolls the ball along the way to
// the goal: down the middle of each passage, slowing for the turns.
func mazePilot(m *maze) (float64, float64) {
	cur := m.cellAt(m.x, m.y)
	goal := m.cellAt(m.goal[0], m.goal[1])
	// Which way from each cell leads to the goal.
	next := make([]int, len(m.cells))
	for i := range next {
		next[i] = -1
	}
	next[goal] = goal
	queue := []int{goal}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for d, dv := range dirs {
			if !m.cells[c].open[d] {
				continue
			}
			n := (c/m.cols+dv[1])*m.cols + c%m.cols + dv[0]
			if next[n] < 0 && !m.cells[n].hole {
				next[n] = c
				queue = append(queue, n)
			}
		}
	}
	center := func(i int) [2]float64 { return [2]float64{float64(i%m.cols) + 0.5, float64(i/m.cols) + 0.5} }
	pull := func(want, v float64) float64 { return math.Max(-4, math.Min(4, 2.8*(want-v))) }
	if cur == goal || next[cur] < 0 {
		g := m.goal
		return pull(1.5*(g[0]-m.x), m.vx), pull(1.5*(g[1]-m.y), m.vy)
	}
	// The straight run ahead, to the next turn.
	c := center(cur)
	n1 := next[cur]
	dx, dy := float64(n1%m.cols-cur%m.cols), float64(n1/m.cols-cur/m.cols)
	end, run := n1, 1
	for end != goal && run < 12 {
		n2 := next[end]
		if float64(n2%m.cols-end%m.cols) != dx || float64(n2/m.cols-end/m.cols) != dy {
			break
		}
		end, run = n2, run+1
	}
	e := center(end)
	// Where the ball will be by the time a tilt takes effect.
	px, py := m.x+m.vx*0.25, m.y+m.vy*0.25
	dist := (e[0]-px)*dx + (e[1]-py)*dy // along the run, to its last cell's middle
	vmax, brake := 3.0, 1.5
	for c2 := cur; ; c2 = next[c2] {
		if m.cells[c2].bridge {
			vmax, brake = 1.3, 1.0 // careful over a bridge: no walls to lean on
		}
		if c2 == end {
			break
		}
	}
	along := math.Min(vmax, math.Sqrt(2*brake*math.Max(0, dist))+0.15)
	// Across: back to the passage's middle line.
	crossX, crossY := (c[0]-m.x)*math.Abs(dy), (c[1]-m.y)*math.Abs(dx)
	wantX := dx*along + 3*crossX
	wantY := dy*along + 3*crossY
	return pull(wantX, m.vx), pull(wantY, m.vy)
}
