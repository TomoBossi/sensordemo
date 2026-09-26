package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

func testMaze(level int, seed int64) *maze {
	m := &maze{level: level, seed: seed}
	m.generate(70, 36)
	return m
}

// The ball never ends up inside a wall, however it is thrown around.
func TestMazeNoTunneling(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		m := testMaze(1, seed)
		rng := rand.New(rand.NewSource(seed))
		ax, ay := 0.0, 0.0
		for i := 0; i < 3000; i++ {
			if i%40 == 0 {
				ax, ay = rng.Float64()*8-4, rng.Float64()*8-4
			}
			m.step(ax, ay, 1.0/30, float64(i))
			for _, b := range m.walls {
				cx, cy := math.Max(b.x0, math.Min(m.x, b.x1)), math.Max(b.y0, math.Min(m.y, b.y1))
				if d := math.Hypot(m.x-cx, m.y-cy); d < ballR-1e-6 {
					t.Fatalf("seed %d step %d: ball %.3f,%.3f is %.3f into wall %+v", seed, i, m.x, m.y, ballR-d, b)
				}
			}
			if m.check(float64(i)); m.state != rolling {
				m.reset()
			}
		}
	}
}

// Rolled into a wall, the ball comes to rest against it.
func TestMazeRestsAgainstWall(t *testing.T) {
	m := testMaze(1, 3)
	m.cells[0].open = [4]bool{} // box the start in (walls only matter by list)
	for i := 0; i < 300; i++ {
		m.step(-3, 0, 1.0/30, 0) // tilted left: the border wall
	}
	if want := wallT/2 + ballR; math.Abs(m.x-want) > 0.01 || math.Hypot(m.vx, m.vy) > 0.05 {
		t.Fatalf("ball at x=%.3f v=%.3f,%.3f; want at rest at x=%.3f", m.x, m.vx, m.vy, want)
	}
}

// Rolling sideways off a bridge drops the ball.
func TestMazeFallsOffBridge(t *testing.T) {
	m := testMaze(3, 5)
	bi := -1
	for i, c := range m.cells {
		if c.bridge {
			bi = i
			break
		}
	}
	if bi < 0 {
		t.Fatal("no bridge")
	}
	m.x, m.y = float64(bi%m.cols)+0.5, float64(bi/m.cols)+0.5
	if !m.floorAt(m.x, m.y) {
		t.Fatal("bridge center is not floor")
	}
	c := m.cells[bi]
	ax, ay := 0.0, 3.0 // across a horizontal strip
	if c.open[0] || c.open[1] {
		if c.open[2] || c.open[3] { // a corner: go out a closed side
			if !c.open[2] {
				ax, ay = 0, 3
			} else {
				ax, ay = 0, -3
			}
		}
	} else {
		ax, ay = 3, 0
	}
	for i := 0; i < 90 && m.state == rolling; i++ {
		m.step(ax, ay, 1.0/30, float64(i))
	}
	if m.state != falling {
		t.Fatalf("still %d at %.2f,%.2f", m.state, m.x, m.y)
	}
}

// MAZE_SHOW=level[,seed] prints boards for looking at.
func TestMazeShow(t *testing.T) {
	spec := os.Getenv("MAZE_SHOW")
	if spec == "" {
		t.Skip("set MAZE_SHOW=level")
	}
	var level, seed int
	fmt.Sscanf(spec, "%d,%d", &level, &seed)
	m := testMaze(level, int64(seed))
	tilt, _ := strconv.ParseFloat(os.Getenv("MAZE_TILT"), 64)
	m.up = Vec3{tilt, tilt * 0.6, 1}.Norm()
	var f Frame
	f.Resize(70, 37)
	m.render(f.View(0, 0, 70, 37), 36, 0)
	for y := 0; y < f.H; y++ {
		fmt.Println(string(f.chars[y*f.W : (y+1)*f.W]))
	}
}

func BenchmarkMazeRender(b *testing.B) {
	m := testMaze(4, 2)
	m.up = Vec3{0.3, 0.2, 1}.Norm()
	var f Frame
	f.Resize(150, 90)
	for i := 0; i < b.N; i++ {
		m.render(f.View(0, 0, 150, 90), 89, 0)
	}
}
