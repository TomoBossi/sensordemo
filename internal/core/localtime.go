package core

import (
	"os"
	"os/exec"
	"strings"
	"time"
	_ "time/tzdata" // Android keeps its zone database where Go doesn't look
)

// On Android, Go's time.Local stays UTC: the system's tzdata lives under
// /apex, which Go doesn't read, and TZ alone can't be resolved without a
// database. With the embedded one, set the local zone from TZ or the
// phone's own setting.
func init() {
	if time.Local.String() != "UTC" {
		return
	}
	name := os.Getenv("TZ")
	if name == "" {
		if out, err := exec.Command("getprop", "persist.sys.timezone").Output(); err == nil {
			name = strings.TrimSpace(string(out))
		}
	}
	if loc, err := time.LoadLocation(name); err == nil && name != "" {
		time.Local = loc
	}
}
