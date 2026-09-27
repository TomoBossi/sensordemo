package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// q, c and ? belong to the app (quit, colors, help): no demo may bind
// them, since the app takes them first.
func TestNoDemoTakesGlobalKeys(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`case[^:\n]*'[qc?]'`)
	for _, f := range files {
		if f == "main.go" || filepath.Ext(f) != ".go" || regexp.MustCompile(`_test\.go$`).MatchString(f) {
			continue
		}
		src, _ := os.ReadFile(f)
		if m := re.Find(src); m != nil {
			t.Errorf("%s binds a key the app takes: %s", f, m)
		}
	}
}
