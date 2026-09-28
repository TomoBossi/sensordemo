package core

import (
	"runtime"
	"sync"
)

// ParallelRows calls f for every row 0..h-1, spread over the CPU cores.
// Rows are interleaved so each core gets a fair mix of cheap and expensive
// ones. f must only write to its own row's cells.
func ParallelRows(h int, f func(y int)) {
	n := min(runtime.NumCPU(), h)
	if n <= 1 {
		for y := 0; y < h; y++ {
			f(y)
		}
		return
	}
	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for y := w; y < h; y += n {
				f(y)
			}
		}(w)
	}
	wg.Wait()
}
