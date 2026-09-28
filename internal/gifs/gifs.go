package gifs

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/TomoBossi/sensordemo/internal/core"

	"github.com/TomoBossi/sensord/client"
)

// Package gifs makes the README's GIFs: a demo run offline on simulated
// time, fed a script of sensor readings, drawn with the app's own header
// and gauges, in the phone's font and colors, and encoded by ffmpeg. Each
// demo's script is its folder's gif_test.go, which calls Run; the GIF is
// written next to it:
//
//	GIFS=all go test -run TestGIF -timeout 60m ./demos/...
//	GIFS=candle,eye GIFSIZE=97x94 go test -run TestGIF ./demos/...
//
// Each glyph is rendered once from the phone's monospace font at 12x24
// pixels (a phone's cell), and shrunk to 4x8 by averaging in linear light,
// as the eye does small text.

const (
	gifFont = "/system/fonts/DroidSansMono.ttf" // Termux's default: Android's monospace
	gifHiW  = 12                                // the atlas's cell
	gifHiH  = 24
	gifFPS  = 30
)

var (
	gifBoost           = 2.0
	gifLevels          = 3    // coverage levels (0: smooth): a few compress far better and look alike at this size
	gifColors          = 128  // the GIF's palette
	gifCellW, gifCellH = 4, 8 // the GIF's cell: the atlas's, shrunk
)

// gifScene is one GIF: a demo, how long to run it before recording and how
// long to record, and what the sensors read at each moment.
type Scene struct {
	Demo   string
	Arg    string   // the demo's argument, if any
	Specs  []string // sensors, for scope
	Start  time.Time
	Warm   float64 // seconds run before recording
	Secs   float64 // seconds recorded
	Inputs func(t float64) map[string][]float64
	Keys   map[float64]byte             // keys pressed, by time
	Setup  func(t *testing.T, d Demo)   // after Setup, before the first frame
	Every  func(d Demo, t float64)      // each frame, before drawing
	Hold   float64                      // seconds the last frame is held, for scenes that don't loop
	Fade   float64                      // seconds the end crossfades into the start, for scenes that can't loop exactly
	Speed  float64                      // the demo's time runs this much faster than the GIF's (1 if 0)
	ClockX float64                      // the time of day runs this much faster (1 if 0)
	Turn   func(t float64) float64      // the phone turned about its screen, degrees: the GIF shows it
	Until  func(d Demo, t float64) bool // stop recording once this holds
}

// Run renders the GIF of the scene that scene makes (fresh each call:
// scenes keep state in their closures) into the current directory, the
// demo's own, if GIFS names it (or is "all").
func Run(t *testing.T, scene func() Scene) {
	sc := scene()
	want := os.Getenv("GIFS")
	if want == "" {
		t.Skip("set GIFS=all or GIFS=demo,demo")
	}
	if want != "all" && !strings.Contains(","+want+",", ","+sc.Demo+",") {
		t.Skip("not in GIFS")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("needs ffmpeg")
	}
	cols, rows := 97, 94 // the phone's terminal, font as set, keyboard hidden
	if c := os.Getenv("GIFCELL"); c != "" {
		fmt.Sscanf(c, "%dx%d", &gifCellW, &gifCellH)
	}
	if l := os.Getenv("GIFLEVELS"); l != "" {
		fmt.Sscanf(l, "%d", &gifLevels)
	}
	if c := os.Getenv("GIFCOLORS"); c != "" {
		fmt.Sscanf(c, "%d", &gifColors)
	}
	if b := os.Getenv("GIFBOOST"); b != "" {
		fmt.Sscanf(b, "%g", &gifBoost)
	}
	if s := os.Getenv("GIFSIZE"); s != "" {
		fmt.Sscanf(s, "%dx%d", &cols, &rows)
	}
	out := os.Getenv("GIFOUT") // elsewhere than the demo's folder
	if out == "" {
		out = "."
	}
	os.MkdirAll(out, 0o755)
	// Demos that save things (calibration, places, map tiles) keep them
	// away from the real ones.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	cache := filepath.Join(tmp, "cache")
	if c := os.Getenv("GIFCACHE"); c != "" { // keep map data between runs
		cache = c
	}
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	// A calibration that matches the scripts' magnetometer, and a saved
	// place a kilometre from the scripts' square.
	cal := filepath.Join(tmp, "magcal.json")
	os.WriteFile(cal, []byte(`{"bias":[12,-30,45],"radius":22.98}`), 0o644)
	t.Setenv("SENSORDEMO_MAGCAL", cal)
	places := filepath.Join(tmp, "places.json")
	os.WriteFile(places, []byte(`{"home":{"Lat":-34.60835,"Lon":-58.37205,"Saved":"2026-09-01T12:00:00Z"}}`), 0o644)
	t.Setenv("SENSORDEMO_PLACES", places)
	// The scripts' places are in Buenos Aires: so is the time of day the
	// demos show, wherever the GIFs are made.
	local := time.Local
	time.Local = time.FixedZone("-03", -3*3600)
	defer func() { time.Local = local }()
	atlas := gifAtlas(t)
	path := filepath.Join(out, sc.Demo+".gif")
	if err := renderGIF(t, sc, scene, cols, rows, atlas, path); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err == nil {
		t.Logf("%s: %.1f MB", sc.Demo+".gif", float64(fi.Size())/1e6)
	}
}

// gifAtlas renders the printable ASCII characters in the phone's font and
// shrinks each to a gifCellW x gifCellH coverage map.
func gifAtlas(t *testing.T) [][]float64 {
	dir := t.TempDir()
	chars := make([]byte, 0, 95)
	for c := 32; c < 127; c++ {
		chars = append(chars, byte(c))
	}
	txt := filepath.Join(dir, "chars.txt")
	os.WriteFile(txt, chars, 0o644)
	w := len(chars) * gifHiW
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", fmt.Sprintf("color=c=black:s=%dx%d", w, gifHiH), "-frames:v", "1",
		"-vf", fmt.Sprintf("drawtext=fontfile=%s:textfile=%s:expansion=none:fontsize=20:fontcolor=white:x=0:y=4", gifFont, txt),
		"-f", "rawvideo", "-pix_fmt", "gray", "-")
	raw, err := cmd.Output()
	if err != nil || len(raw) != w*gifHiH {
		t.Fatalf("rendering the font: %v (%d bytes)", err, len(raw))
	}
	sx, sy := gifHiW/gifCellW, gifHiH/gifCellH
	atlas := make([][]float64, 128)
	for i, c := range chars {
		cov := make([]float64, gifCellW*gifCellH)
		for y := 0; y < gifCellH; y++ {
			for x := 0; x < gifCellW; x++ {
				sum := 0.0
				for dy := 0; dy < sy; dy++ {
					for dx := 0; dx < sx; dx++ {
						sum += float64(raw[(y*sy+dy)*w+i*gifHiW+x*sx+dx]) / 255
					}
				}
				// Strokes keep their full brightness, as a font's hinting
				// keeps them crisp at small sizes: coverage is boosted.
				c := math.Min(1, sum/float64(sx*sy)*gifBoost)
				if gifLevels > 1 { // a few levels of coverage compress far better
					c = math.Round(c*float64(gifLevels-1)) / float64(gifLevels-1)
				}
				cov[y*gifCellW+x] = c
			}
		}
		atlas[c] = cov
	}
	return atlas
}

// termuxRGB is the color Termux shows for a 256-color index (0 is its
// default foreground).
func termuxRGB(c uint8) [3]float64 {
	var r, g, b uint8
	switch {
	case c == 0:
		r, g, b = 255, 255, 255
	case c < 16:
		base := [16][3]uint8{{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {100, 149, 237}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
			{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255}}[c]
		r, g, b = base[0], base[1], base[2]
	case c >= 232:
		v := uint8(8 + 10*int(c-232))
		r, g, b = v, v, v
	default:
		i := int(c) - 16
		r, g, b = uint8(CubeLevels[i/36]), uint8(CubeLevels[i/6%6]), uint8(CubeLevels[i%6])
	}
	return [3]float64{srgbToLinear(r), srgbToLinear(g), srgbToLinear(b)}
}

func srgbToLinear(v uint8) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func linearToSRGB(c float64) uint8 {
	c = math.Max(0, math.Min(1, c))
	if c <= 0.0031308 {
		c *= 12.92
	} else {
		c = 1.055*math.Pow(c, 1/2.4) - 0.055
	}
	return uint8(c*255 + 0.5)
}

// rasterize paints a frame into rgb (gifCellW x gifCellH pixels a cell).
func rasterize(f *Frame, atlas [][]float64, rgb []byte, lut map[uint8][]byte) {
	W := f.W * gifCellW
	for cy := 0; cy < f.H; cy++ {
		for cx := 0; cx < f.W; cx++ {
			ch, fg := f.Cell(cx, cy)
			if ch < 32 || ch > 126 {
				ch = '?'
			}
			cov := atlas[ch]
			ramp, ok := lut[fg]
			if !ok {
				col := termuxRGB(fg)
				ramp = make([]byte, 3*256)
				for k := 0; k < 256; k++ {
					a := float64(k) / 255
					ramp[3*k], ramp[3*k+1], ramp[3*k+2] = linearToSRGB(col[0]*a), linearToSRGB(col[1]*a), linearToSRGB(col[2]*a)
				}
				lut[fg] = ramp
			}
			for y := 0; y < gifCellH; y++ {
				row := ((cy*gifCellH+y)*W + cx*gifCellW) * 3
				for x := 0; x < gifCellW; x++ {
					k := int(cov[y*gifCellW+x]*255 + 0.5)
					copy(rgb[row+3*x:row+3*x+3], ramp[3*k:3*k+3])
				}
			}
		}
	}
}

// renderGIF runs a scene and pipes its recorded frames to ffmpeg.
func renderGIF(t *testing.T, sc Scene, scene func() Scene, cols, rows int, atlas [][]float64, path string) error {
	// A scene that ends when something happens, and fades back into its
	// start: run it once to find when, then record just that long.
	if sc.Until != nil && sc.Fade > 0 {
		stop, err := runGIF(t, scene(), cols, rows, nil, "", true)
		if err != nil {
			return err
		}
		sc = scene()
		sc.Until = nil
		sc.Secs = float64(stop+1)/gifFPS - sc.Fade
	}
	_, err := runGIF(t, sc, cols, rows, atlas, path, false)
	return err
}

// runGIF runs a scene and pipes its recorded frames to ffmpeg; dry, it
// only runs it, and returns the recorded frame at which until held.
func runGIF(t *testing.T, sc Scene, cols, rows int, atlas [][]float64, path string, dry bool) (int, error) {
	ss, _ := OpenMock("") // subscriptions stay silent: the script feeds them
	defer ss.Close()
	start := sc.Start
	if start.IsZero() {
		start = time.Date(2026, 9, 26, 21, 0, 0, 0, time.FixedZone("-03", -3*3600))
	}
	simNow := start
	Clock = func() time.Time { return simNow }
	speed, clockX := sc.Speed, sc.ClockX
	if speed == 0 {
		speed = 1
	}
	if clockX == 0 {
		clockX = 1
	}
	defer func() { Clock = time.Now }()
	DemoArg = sc.Arg
	defer func() { DemoArg = "" }()
	e := Find(sc.Demo)
	if e == nil {
		return 0, fmt.Errorf("no demo %q", sc.Demo)
	}
	d := e.New(sc.Specs)
	// The first readings, so Setup finds the sensors reading.
	feed := newGIFFeed(ss)
	gauges, err := d.Setup(ss)
	if err != nil {
		return 0, err
	}
	gauges = append(gauges, ScreenGauge(ss)...)
	if sc.Setup != nil {
		sc.Setup(t, d)
	}
	title := e.Name
	if sc.Specs != nil {
		title += " (" + strings.Join(sc.Specs, ",") + ")"
	}

	W, H := cols*gifCellW, rows*gifCellH
	vf := fmt.Sprintf("split[a][b];[a]palettegen=max_colors=%d:stats_mode=full[p];[b][p]paletteuse=dither=none:diff_mode=rectangle", gifColors)
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "rawvideo", "-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", W, H), "-r", fmt.Sprint(gifFPS), "-i", "-", "-filter_complex", vf, "-loop", "0", path}
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr
	var pipe interface {
		Write([]byte) (int, error)
		Close() error
	}
	var bw *bufio.Writer
	if !dry {
		p, err := cmd.StdinPipe()
		if err != nil {
			return 0, err
		}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		pipe = p
		bw = bufio.NewWriterSize(pipe, 1<<20)
	}
	rgb := make([]byte, W*H*3)
	rot := make([]byte, W*H*3)
	lut := map[uint8][]byte{}
	var f Frame
	const dt = 1.0 / gifFPS
	N := int(math.Round(sc.Secs * gifFPS))
	F := int(math.Round(sc.Fade * gifFPS))
	n := int(math.Round(sc.Warm*gifFPS)) + N + F
	rec := int(math.Round(sc.Warm * gifFPS))
	var first [][]byte // the first frames, to crossfade the last ones into
	pressed := map[float64]bool{}
	for i := 0; i < n; i++ {
		tt := float64(i) * dt
		simNow = start.Add(time.Duration(tt * clockX * float64(time.Second)))
		// Two readings a frame: sensors report at 60 Hz.
		feed.push(sc.Inputs, tt)
		feed.push(sc.Inputs, tt+dt/2)
		for at, k := range sc.Keys {
			if !pressed[at] && tt >= at {
				pressed[at] = true
				d.Key(k)
			}
		}
		if sc.Every != nil {
			sc.Every(d, tt)
		}
		f.Resize(cols, rows)
		hudH := DrawHUD(f.View(0, 0, cols, rows), title, Palettes[0].Name, gifFPS, gauges, ss)
		d.Draw(f.View(0, hudH, cols, rows-hudH), ss, tt*speed, dt*speed)
		if i < rec {
			continue
		}
		if dry {
			if sc.Until != nil && sc.Until(d, tt) {
				return i - rec, nil
			}
			continue
		}
		rasterize(&f, atlas, rgb, lut)
		if sc.Turn != nil {
			turnFrame(rgb, rot, W, H, sc.Turn(tt))
			copy(rgb, rot)
		}
		if sc.Until != nil && sc.Until(d, tt) {
			bw.Write(rgb)
			break
		}
		switch r := i - rec; {
		case r < F:
			first = append(first, append([]byte(nil), rgb...))
		case r < N:
			bw.Write(rgb)
		default: // the end, fading into the start
			k := r - N
			a := (float64(k) + 0.5) / float64(F)
			for j := range rgb {
				rgb[j] = byte(float64(rgb[j])*(1-a) + float64(first[k][j])*a + 0.5)
			}
			bw.Write(rgb)
		}
	}
	if dry {
		return 0, fmt.Errorf("%s: what should end it never happened", sc.Demo)
	}
	for i := 0; i < int(sc.Hold*gifFPS); i++ {
		bw.Write(rgb)
	}
	bw.Flush()
	pipe.Close()
	return 0, cmd.Wait()
}

// gifFeed pushes a script's readings into every subscribed stream of the
// sensor's type: continuous ones each time, the rest when they change.
type gifFeed struct {
	ss    *Streams
	last  map[*Stream][]float64
	lastT map[*Stream]float64
}

func newGIFFeed(ss *Streams) *gifFeed {
	return &gifFeed{ss: ss, last: map[*Stream][]float64{}, lastT: map[*Stream]float64{}}
}

func (g *gifFeed) push(inputs func(float64) map[string][]float64, t float64) {
	vals := inputs(t)
	for _, s := range g.ss.All() {
		v, ok := vals[s.Info.Type]
		if !ok {
			continue
		}
		if s.Info.Mode == "continuous" && s.Info.MaxHz > 0 && s.Info.MaxHz < 60 {
			if lt, seen := g.lastT[s]; seen && t-lt < 1/s.Info.MaxHz-1e-6 {
				continue // a slow sensor, such as location, at its own rate
			}
			g.lastT[s] = t
		}
		if s.Info.Mode != "continuous" {
			if old, seen := g.last[s]; seen && equalVals(old, v) {
				continue
			}
			g.last[s] = append([]float64(nil), v...)
		}
		s.Push(client.Event{T: int64(t*1e9) + 1, V: append([]float64(nil), v...)})
	}
}

func equalVals(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A phone in the hand, for scripts: its pose (yaw about up, pitch about
// its x, roll about its y, in degrees, as the mock's "orient"), and its
// acceleration beyond gravity (screen axes, m/s^2). motion fills in every
// motion sensor consistently: the rotation vectors, gravity, the
// accelerometer, linear acceleration, the gyroscope, the magnetometer.
type Pose struct{ Yaw, Pitch, Roll float64 }

var Field = Vec3{0, 18.6, 13.5} // Earth's field, µT: magnetic north, and up (as in the southern hemisphere)

func Motion(pose func(t float64) Pose, lin func(t float64) Vec3, t float64) map[string][]float64 {
	at := func(t float64) (Mat3, []float64) {
		p := pose(t)
		q := QuatFromEuler([]float64{p.Yaw, p.Pitch, p.Roll})
		R, _ := FromRotationVector(q)
		return R, q
	}
	R, q := at(t)
	const h = 1e-3
	R2, _ := at(t + h)
	O := R.T().Mul(R2) // the turn over h, in the phone's frame
	gyro := Vec3{(O[2][1] - O[1][2]) / (2 * h), (O[0][2] - O[2][0]) / (2 * h), (O[1][0] - O[0][1]) / (2 * h)}
	g := R.T().Apply(Vec3{0, 0, 9.81})
	var a Vec3
	if lin != nil {
		a = lin(t)
	}
	m := R.T().Apply(Field)
	acc := g.Add(a)
	return map[string][]float64{
		"rotation_vector":             append(q, 0),
		"game_rotation_vector":        q,
		"geomagnetic_rotation_vector": append(q, 0),
		"gravity":                     g[:],
		"accelerometer":               acc[:],
		"linear_acceleration":         a[:],
		"gyroscope":                   gyro[:],
		"magnetic_field":              {m[0], m[1], m[2], 3},
		"magnetic_field_uncalibrated": {m[0] + 12, m[1] - 30, m[2] + 45, 12, -30, 45},
		"display_rotation":            {0},
	}
}

// merge adds more readings to a script's.
func Merge(a map[string][]float64, more map[string][]float64) map[string][]float64 {
	for k, v := range more {
		a[k] = v
	}
	return a
}

// smooth steps from 0 to 1 between a and b, easing both ends.
func Ease(a, b, t float64) float64 { return Smoothstep(a, b, t) }

// The place scripts are at: a public square, not anyone's home.
var Place = []float64{-34.60372, -58.38159, 6, math.NaN(), math.NaN(), math.NaN(), -9.3, 22.8}

// wave is a sum of sines that repeats every T: a hand's small unsteady
// movement, for scripts that must loop.
func Wave(t, T float64, amps ...float64) float64 {
	v := 0.0
	for i, a := range amps {
		k := float64(i + 1)
		v += a * math.Sin(2*math.Pi*k*t/T+k*1.7)
	}
	return v
}

// cycle is t's progress through a loop of length T after warm-up, 0..1.
func Cycle(t, warm, T float64) float64 { return math.Max(0, t-warm) / T }

// turnFrame draws src into dst turned by deg about its center, shrunk to
// fit: how the phone looks as it's turned over.
func turnFrame(src, dst []byte, W, H int, deg float64) {
	a := deg * math.Pi / 180
	c, sn := math.Cos(a), math.Sin(a)
	fw, fh := float64(W), float64(H)
	k := math.Min(fw/(fw*math.Abs(c)+fh*math.Abs(sn)), fh/(fw*math.Abs(sn)+fh*math.Abs(c)))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			u, v := (float64(x)+0.5-fw/2)/k, (float64(y)+0.5-fh/2)/k
			sx, sy := c*u+sn*v+fw/2-0.5, -sn*u+c*v+fh/2-0.5
			o := (y*W + x) * 3
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			if x0 < 0 || y0 < 0 || x0+1 >= W || y0+1 >= H {
				dst[o], dst[o+1], dst[o+2] = 0, 0, 0
				continue
			}
			fx, fy := sx-float64(x0), sy-float64(y0)
			for ch := 0; ch < 3; ch++ {
				p := func(xx, yy int) float64 { return float64(src[(yy*W+xx)*3+ch]) }
				val := (p(x0, y0)*(1-fx)+p(x0+1, y0)*fx)*(1-fy) + (p(x0, y0+1)*(1-fx)+p(x0+1, y0+1)*fx)*fy
				dst[o+ch] = byte(val + 0.5)
			}
		}
	}
}
