package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// q, c and ? belong to the app (quit, colors, help): no demo may bind
// them, since the app takes them first.
func TestNoDemoTakesGlobalKeys(t *testing.T) {
	files, _ := filepath.Glob("../demos/*/*.go")
	if len(files) == 0 {
		t.Fatal("no demo sources found")
	}
	re := regexp.MustCompile(`case[^:\n]*'[qc?]'`)
	for _, f := range files {
		if regexp.MustCompile(`_test\.go$`).MatchString(f) {
			continue
		}
		src, _ := os.ReadFile(f)
		if m := re.Find(src); m != nil {
			t.Errorf("%s binds a key the app takes: %s", f, m)
		}
	}
}
