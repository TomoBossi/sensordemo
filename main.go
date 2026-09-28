// Command sensordemo shows phone sensors as full-terminal ASCII demos, fed by
// the sensord app. Run "sensordemo list" for the demos.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/TomoBossi/sensordemo/demos"
	. "github.com/TomoBossi/sensordemo/internal/core"
)

func usage() {
	width := termWidth()
	fmt.Println("usage:")
	fmt.Print(columns([][2]string{
		{"  sensordemo [--gray] DEMO [ARG]", "a demo by name (some take an argument)"},
		{"  sensordemo [--gray] SENSOR[,SENSOR...]", "the demo that uses these sensors"},
		{"  sensordemo list", "the demos and the sensors they use"},
	}, width))
	fmt.Println("\ndemos:")
	sort.Slice(Registry, func(i, j int) bool { return Registry[i].Name < Registry[j].Name })
	var rows [][2]string
	for _, e := range Registry {
		rows = append(rows, [2]string{"  " + e.Name, e.Desc})
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
	sort.Slice(Registry, func(i, j int) bool { return Registry[i].Name < Registry[j].Name })
	width := termWidth()
	for _, e := range Registry {
		uses := strings.Join(e.Uses, ", ")
		if uses == "" {
			uses = "any sensor"
		}
		fmt.Println(e.Name)
		fmt.Println(strings.Join(wrapWords(e.Desc, width, 4, 4), "\n"))
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
	if len(args) == 2 {
		DemoArg = args[1] // e.g. sensordemo hourglass 5m
		args = args[:1]
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

func run(arg string, gray bool) error {
	ss, err := OpenStreams()
	if err != nil {
		return err
	}
	defer ss.Close()
	e, specs, err := PickDemo(arg, ss)
	if err != nil {
		return err
	}
	demo := e.New(specs)
	gauges, err := demo.Setup(ss)
	if err != nil {
		return err
	}
	gauges = append(gauges, ScreenGauge(ss)...)

	t, err := OpenTerminal()
	if err != nil {
		return err
	}
	if o, ok := demo.(interface {
		SetOut(interface{ Write([]byte) (int, error) })
	}); ok {
		o.SetOut(t.out)
	}
	defer t.Restore()
	defer func() { // leave the terminal usable even if a demo panics
		if p := recover(); p != nil {
			t.Restore()
			panic(p)
		}
	}()

	title := e.Name
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
	fps := 0.0
	tick := time.NewTicker(time.Second / 30)
	defer tick.Stop()
	for {
		select {
		case k := <-t.Keys:
			switch k {
			case 'q', 3, 4: // q, Ctrl-C, Ctrl-D
				return nil
			case 'c':
				pal = (pal + 1) % len(Palettes)
			case '?':
				help = !help
			default:
				demo.Key(k)
			}
			continue
		case now := <-tick.C:
			w, h := t.Size()
			f.Resize(w, h)
			// Frames actually drawn per second: when rendering is slower
			// than the ticker, ticks are skipped and this drops.
			if d := now.Sub(last).Seconds(); d > 0 {
				if fps == 0 {
					fps = 1 / d
				}
				fps += (1/d - fps) * 0.1
			}
			hudH := DrawHUD(f.View(0, 0, w, h), title, Palettes[pal].Name, fps, gauges, ss)
			demo.Draw(f.View(0, hudH, w, h-hudH), ss, now.Sub(start).Seconds(), now.Sub(last).Seconds())
			if help {
				drawHelp(f.View(0, 0, w, h), e, demo.Help())
			}
			last = now
			f.Flush(t.out, &Palettes[pal])
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
	e, specs, err := PickDemo(arg, ss)
	if err != nil {
		return err
	}
	demo := e.New(specs)
	gauges, err := demo.Setup(ss)
	if err != nil {
		return err
	}
	gauges = append(gauges, ScreenGauge(ss)...)
	var f Frame
	start := time.Now()
	for i := 0; i < frames; i++ {
		time.Sleep(time.Second / 30)
		f.Resize(w, h)
		hudH := DrawHUD(f.View(0, 0, w, h), e.Name, "native", 0, gauges, ss)
		demo.Draw(f.View(0, hudH, w, h-hudH), ss, time.Since(start).Seconds(), 1.0/30)
	}
	for y := 0; y < f.H; y++ {
		fmt.Println(f.Row(y))
	}
	return nil
}

func drawHelp(v *View, e *Entry, lines []string) {
	lines = append([]string{e.Name + ": " + e.Desc, ""}, lines...)
	lines = append(lines, "", "q quit   c colors   ? close this")
	inner := max(10, min(v.W-4, 56))
	var wrapped []string
	for _, l := range lines {
		wrapped = append(wrapped, Wrap(l, inner)...)
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
