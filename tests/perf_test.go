package tests

import (
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/TomoBossi/sensordemo/internal/core"
)

// TestFrameTimes renders each demo at a normal and a small-font size.
// Run with PERF=1 go test -run TestFrameTimes -v ./tests/
func TestFrameTimes(t *testing.T) {
	if os.Getenv("PERF") == "" {
		t.Skip("set PERF=1")
	}
	mock := "accelerometer=1,9,2;orient=20,80,10;proximity=5;light=300;magnetic_field=10,10,-15,3;step_detector=1;location=-34.6037,-58.3816,15,20,0,0,-10,23"
	for _, name := range []string{"donut", "space", "eye", "navball", "fluid", "compass", "sky", "scope", "gestures"} {
		for _, sz := range [][2]int{{70, 40}, {150, 90}} {
			ss, _ := OpenMock(mock)
			d := Find(name).New(nil)
			d.Setup(ss)
			var f Frame
			f.Resize(sz[0], sz[1])
			v := f.View(0, 0, sz[0], sz[1])
			time.Sleep(50 * time.Millisecond)
			d.Draw(v, ss, 0, 1.0/30) // warm up, after mock readings arrive
			start := time.Now()
			const n = 10
			for i := 0; i < n; i++ {
				d.Draw(v, ss, float64(i)/30, 1.0/30)
			}
			ms := float64(time.Since(start).Microseconds()) / n / 1000
			fmt.Printf("%-9s %3dx%-3d %6.1f ms/frame  (%.0f fps max)\n", name, sz[0], sz[1], ms, 1000/ms)
		}
	}
}
