// Command sensordemo shows phone sensors as full-terminal ASCII demos, fed by
// the sensord app. Run "sensordemo list" for the demos.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Demo is one visualization. Setup subscribes to what it needs and returns the
// gauges for the data strip; Draw renders a frame into the space below it.
type Demo interface {
	Setup(ss *Streams) ([]*Gauge, error)
	Draw(v *View, ss *Streams, t, dt float64)
	Key(k byte)
	Help() []string
}

type entry struct {
	name string
	desc string
	uses []string // sensor types that map to this demo
	new  func(specs []string) Demo
}

var registry []entry

func register(e entry) { registry = append(registry, e) }

func find(name string) *entry {
	for i := range registry {
		if registry[i].name == name {
			return &registry[i]
		}
	}
	return nil
}

// pick maps the command-line argument to a demo: a demo name, or one or more
// comma-separated sensors (names or types). For sensors, it picks the demo
// whose sensors cover all of them with the fewest extras, and falls back to
// the scope, which works for any sensor.
func pick(arg string, ss *Streams) (*entry, []string, error) {
	if e := find(arg); e != nil {
		return e, nil, nil
	}
	specs := strings.Split(arg, ",")
	types := make([]string, len(specs))
	for i, sp := range specs {
		s, ok := ss.Lookup(sp)
		if !ok {
			return nil, nil, fmt.Errorf("%q is neither a demo nor a sensor (see: sensordemo list, sensord list)", sp)
		}
		types[i] = s.Type
	}
	var best *entry
	for i := range registry {
		e := &registry[i]
		if len(e.uses) == 0 || !covers(e.uses, types) {
			continue
		}
		if best == nil || len(e.uses) < len(best.uses) {
			best = e
		}
	}
	if best == nil {
		best = find("scope")
	}
	return best, specs, nil
}

func covers(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func usage() {
	fmt.Print(`usage: sensordemo [--color] DEMO | SENSOR[,SENSOR...]
       sensordemo list

Full-terminal ASCII demos of the phone's sensors, fed by sensord. Give a demo
name, or one or more sensors (names or types from "sensord list"): sensors map
to the demo that uses them, or to the scope, which shows any sensor.

Keys: q quit, c toggle color, ? help for the current demo.
`)
}

func list() {
	sort.Slice(registry, func(i, j int) bool { return registry[i].name < registry[j].name })
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DEMO\tSENSORS\tWHAT IT SHOWS")
	for _, e := range registry {
		uses := strings.Join(e.uses, ",")
		if uses == "" {
			uses = "(any)"
		}
		if len(uses) > 30 {
			uses = uses[:27] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.name, uses, e.desc)
	}
	w.Flush()
}

func main() {
	color := false
	snapshot, mock, frames := "", "", 30
	var args []string
	for i := 1; i < len(os.Args); i++ {
		switch a := os.Args[i]; a {
		case "--snapshot": // hidden: render ~1 s off-screen and print the last frame
			if i+1 < len(os.Args) {
				snapshot = os.Args[i+1]
				i++
			}
		case "--mock": // hidden: fixed readings instead of sensord (see mock.go)
			if i+1 < len(os.Args) {
				mock = os.Args[i+1]
				i++
			}
		case "--frames": // hidden: how many frames a snapshot renders (30 = ~1 s)
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &frames)
				i++
			}
		case "--color", "-c":
			color = true
		case "-h", "--help", "help":
			usage()
			return
		default:
			args = append(args, a)
		}
	}
	if len(args) == 1 && args[0] == "list" {
		list()
		return
	}
	if len(args) != 1 {
		usage()
		os.Exit(2)
	}
	if snapshot != "" {
		var w, h int
		fmt.Sscanf(snapshot, "%dx%d", &w, &h)
		if err := snap(args[0], w, h, mock, frames); err != nil {
			fmt.Fprintln(os.Stderr, "sensordemo:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(args[0], color); err != nil {
		fmt.Fprintln(os.Stderr, "sensordemo:", err)
		os.Exit(1)
	}
}

func run(arg string, color bool) error {
	ss, err := OpenStreams()
	if err != nil {
		return err
	}
	defer ss.Close()
	e, specs, err := pick(arg, ss)
	if err != nil {
		return err
	}
	demo := e.new(specs)
	gauges, err := demo.Setup(ss)
	if err != nil {
		return err
	}

	t, err := OpenTerminal()
	if err != nil {
		return err
	}
	defer t.Restore()
	defer func() { // leave the terminal usable even if a demo panics
		if p := recover(); p != nil {
			t.Restore()
			panic(p)
		}
	}()

	title := e.name
	if specs != nil {
		title += " (" + strings.Join(specs, ",") + ")"
	}
	var f Frame
	help := false
	start, last := time.Now(), time.Now()
	tick := time.NewTicker(time.Second / 30)
	defer tick.Stop()
	for {
		select {
		case k := <-t.Keys:
			switch k {
			case 'q', 3, 4: // q, Ctrl-C, Ctrl-D
				return nil
			case 'c':
				color = !color
			case '?':
				help = !help
			default:
				demo.Key(k)
			}
			continue
		case now := <-tick.C:
			w, h := t.Size()
			f.Resize(w, h)
			hudH := DrawHUD(f.View(0, 0, w, h), title, gauges, ss)
			demo.Draw(f.View(0, hudH, w, h-hudH), ss, now.Sub(start).Seconds(), now.Sub(last).Seconds())
			if help {
				drawHelp(f.View(0, 0, w, h), e, demo.Help())
			}
			last = now
			f.Flush(t.out, color)
		}
	}
}

// snap renders about a second of frames at w x h without a terminal and prints
// the last one, for checking demos from scripts.
func snap(arg string, w, h int, mock string, frames int) error {
	var ss *Streams
	var err error
	if mock != "" {
		ss, err = OpenMock(mock)
	} else {
		ss, err = OpenStreams()
	}
	if err != nil {
		return err
	}
	defer ss.Close()
	e, specs, err := pick(arg, ss)
	if err != nil {
		return err
	}
	demo := e.new(specs)
	gauges, err := demo.Setup(ss)
	if err != nil {
		return err
	}
	var f Frame
	start := time.Now()
	for i := 0; i < frames; i++ {
		time.Sleep(time.Second / 30)
		f.Resize(w, h)
		hudH := DrawHUD(f.View(0, 0, w, h), e.name, gauges, ss)
		demo.Draw(f.View(0, hudH, w, h-hudH), ss, time.Since(start).Seconds(), 1.0/30)
	}
	for y := 0; y < f.H; y++ {
		fmt.Println(string(f.chars[y*f.W : (y+1)*f.W]))
	}
	return nil
}

func drawHelp(v *View, e *entry, lines []string) {
	lines = append([]string{e.name + ": " + e.desc, ""}, lines...)
	lines = append(lines, "", "q quit   c color   ? close this")
	bw := 0
	for _, l := range lines {
		bw = max(bw, len(l))
	}
	bw = min(bw+4, v.W)
	bh := min(len(lines)+2, v.H)
	x0, y0 := (v.W-bw)/2, (v.H-bh)/2
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			c := byte(' ')
			switch {
			case (y == 0 || y == bh-1) && (x == 0 || x == bw-1):
				c = '+'
			case y == 0 || y == bh-1:
				c = '-'
			case x == 0 || x == bw-1:
				c = '|'
			}
			v.Set(x0+x, y0+y, c, 250)
		}
	}
	for i, l := range lines {
		if i+1 < bh-1 {
			v.Text(x0+2, y0+1+i, l[:min(len(l), bw-4)], 252)
		}
	}
}
