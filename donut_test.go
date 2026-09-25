package main

import (
	"fmt"
	"testing"
)

// TestDonutNeutral prints the donut with no orientation input (go test -v).
func TestDonutNeutral(t *testing.T) {
	ss := &Streams{byKey: map[string]*Stream{"game_rotation_vector": {}}}
	d := &donut{source: "game_rotation_vector", zoom: 1}
	var f Frame
	f.Resize(70, 30)
	d.Draw(f.View(0, 0, 70, 30), ss, 0, 0)
	for y := 0; y < f.H; y++ {
		fmt.Println(string(f.chars[y*f.W : (y+1)*f.W]))
	}
}
