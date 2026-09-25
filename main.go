// Command sensordemo shows phone sensors as full-terminal ASCII demos, fed by
// the sensord app. Run "sensordemo list" for the demos.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
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
	width := termWidth()
	fmt.Println("usage:")
	fmt.Print(columns([][2]string{
		{"  sensordemo [--gray] DEMO", "a demo by name"},
		{"  sensordemo [--gray] SENSOR[,SENSOR...]", "the demo that uses these sensors"},
		{"  sensordemo list", "the demos and the sensors they use"},
	}, width))
	fmt.Println("\ndemos:")
	sort.Slice(registry, func(i, j int) bool { return registry[i].name < registry[j].name })
	var rows [][2]string
	for _, e := range registry {
		rows = append(rows, [2]string{"  " + e.name, e.desc})
	}
	fmt.Print(columns(rows, width))
	fmt.Println()
	fmt.Println(reflow(`Full-terminal ASCII demos of the phone's sensors, fed by sensord. Sensors are names or types from "sensord list"; they map to the demo that uses them, or to the scope, which shows any sensor.

Keys:
  q   quit
  c   cycle colors: native (the demo's own), gray, amber, green, ice, fire, violet
  ?   help for the current demo

--gray starts in grayscale.`, width))
}

func list() {
	sort.Slice(registry, func(i, j int) bool { return registry[i].name < registry[j].name })
	width := termWidth()
	for _, e := range registry {
		uses := strings.Join(e.uses, ", ")
		if uses == "" {
			uses = "any sensor"
		}
		fmt.Println(e.name)
		fmt.Println(strings.Join(wrapWords(e.desc, width, 4, 4), "\n"))
		fmt.Println(strings.Join(wrapWords("sensors: "+uses, width, 4, 13), "\n"))
	}
}

func main() {
	gray := false
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
		case "--gray", "-g":
			gray = true
		case "--color", "-c": // the default now; kept for old habits
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
	if err := run(args[0], gray); err != nil {
		fmt.Fprintln(os.Stderr, "sensordemo:", err)
		os.Exit(1)
	}
}

// screenGauge subscribes to the display rotation, which every demo that
// uses orientation needs, and shows it in the data strip. Older sensord
// versions don't have it; then the screen is assumed upright.
func screenGauge(ss *Streams) []*Gauge {
	if _, err := ss.Subscribe("display_rotation", 0); err != nil {
		return nil
	}
	return []*Gauge{{Spec: "display_rotation", Label: "screen", Unit: "deg", Scale: &Linear{Min: 0, Max: 270}}}
}

func run(arg string, gray bool) error {
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
	gauges = append(gauges, screenGauge(ss)...)

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
	pal := 0 // the demo's own colors
	if gray {
		pal = 1
	}
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
				pal = (pal + 1) % len(palettes)
			case '?':
				help = !help
			default:
				demo.Key(k)
			}
			continue
		case now := <-tick.C:
			w, h := t.Size()
			f.Resize(w, h)
			hudH := DrawHUD(f.View(0, 0, w, h), title, palettes[pal].name, gauges, ss)
			demo.Draw(f.View(0, hudH, w, h-hudH), ss, now.Sub(start).Seconds(), now.Sub(last).Seconds())
			if help {
				drawHelp(f.View(0, 0, w, h), e, demo.Help())
			}
			last = now
			f.Flush(t.out, &palettes[pal])
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
	gauges = append(gauges, screenGauge(ss)...)
	var f Frame
	start := time.Now()
	for i := 0; i < frames; i++ {
		time.Sleep(time.Second / 30)
		f.Resize(w, h)
		hudH := DrawHUD(f.View(0, 0, w, h), e.name, "native", gauges, ss)
		demo.Draw(f.View(0, hudH, w, h-hudH), ss, time.Since(start).Seconds(), 1.0/30)
	}
	for y := 0; y < f.H; y++ {
		fmt.Println(string(f.chars[y*f.W : (y+1)*f.W]))
	}
	return nil
}

func drawHelp(v *View, e *entry, lines []string) {
	lines = append([]string{e.name + ": " + e.desc, ""}, lines...)
	lines = append(lines, "", "q quit   c colors   ? close this")
	inner := max(10, min(v.W-4, 56))
	var wrapped []string
	for _, l := range lines {
		wrapped = append(wrapped, wrap(l, inner)...)
	}
	bw := 0
	for _, l := range wrapped {
		bw = max(bw, len(l))
	}
	bw = min(bw+4, v.W)
	bh := min(len(wrapped)+2, v.H)
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
	for i, l := range wrapped {
		if i+1 < bh-1 {
			v.Text(x0+2, y0+1+i, l, 252)
		}
	}
}

// wrap splits s into lines of at most n characters at spaces, keeping any
// leading indentation on continuation lines.
func wrap(s string, n int) []string {
	if len(s) <= n {
		return []string{s}
	}
	indent := len(s) - len(strings.TrimLeft(s, " "))
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = strings.Repeat(" ", indent) + w
		case len(line)+1+len(w) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = strings.Repeat(" ", indent) + w
		}
		for len(line) > n { // a single word longer than the line
			out = append(out, line[:n])
			line = line[n:]
		}
	}
	return append(out, line)
}
