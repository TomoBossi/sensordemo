package main

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TomoBossi/sensord/client"
)

// The README's GIFs: every demo run offline on simulated time, fed scripted
// sensor readings, drawn with the app's own header and gauges, in the
// phone's font and colors, and encoded by ffmpeg.
//
//	GIFS=all go test -run TestGIFs -timeout 2h .
//	GIFS=candle,eye GIFSIZE=97x94 GIFOUT=docs/gifs go test -run TestGIFs .
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
type gifScene struct {
	demo   string
	arg    string   // the demo's argument, if any
	specs  []string // sensors, for scope
	start  time.Time
	warm   float64 // seconds run before recording
	secs   float64 // seconds recorded
	inputs func(t float64) map[string][]float64
	keys   map[float64]byte             // keys pressed, by time
	setup  func(t *testing.T, d Demo)   // after Setup, before the first frame
	every  func(d Demo, t float64)      // each frame, before drawing
	hold   float64                      // seconds the last frame is held, for scenes that don't loop
	fade   float64                      // seconds the end crossfades into the start, for scenes that can't loop exactly
	speed  float64                      // the demo's time runs this much faster than the GIF's (1 if 0)
	clockX float64                      // the time of day runs this much faster (1 if 0)
	turn   func(t float64) float64      // the phone turned about its screen, degrees: the GIF shows it
	until  func(d Demo, t float64) bool // stop recording once this holds
}

func TestGIFs(t *testing.T) {
	want := os.Getenv("GIFS")
	if want == "" {
		t.Skip("set GIFS=all or GIFS=demo,demo")
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
	out := os.Getenv("GIFOUT")
	if out == "" {
		out = "docs/gifs"
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
	atlas := gifAtlas(t)
	for _, sc := range gifScenes() {
		if want != "all" && !strings.Contains(","+want+",", ","+sc.demo+",") {
			continue
		}
		t.Run(sc.demo, func(t *testing.T) {
			path := filepath.Join(out, sc.demo+".gif")
			if err := renderGIF(t, sc, cols, rows, atlas, path); err != nil {
				t.Fatal(err)
			}
			if fi, err := os.Stat(path); err == nil {
				t.Logf("%s: %.1f MB", path, float64(fi.Size())/1e6)
			}
		})
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
		r, g, b = uint8(cubeLevels[i/36]), uint8(cubeLevels[i/6%6]), uint8(cubeLevels[i%6])
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
			ch, fg := f.chars[cy*f.W+cx], f.fg[cy*f.W+cx]
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
func renderGIF(t *testing.T, sc gifScene, cols, rows int, atlas [][]float64, path string) error {
	// A scene that ends when something happens, and fades back into its
	// start: run it once to find when, then record just that long.
	if sc.until != nil && sc.fade > 0 {
		stop, err := runGIF(t, sceneNamed(sc.demo), cols, rows, nil, "", true)
		if err != nil {
			return err
		}
		sc = sceneNamed(sc.demo)
		sc.until = nil
		sc.secs = float64(stop+1)/gifFPS - sc.fade
	}
	_, err := runGIF(t, sc, cols, rows, atlas, path, false)
	return err
}

// sceneNamed is a fresh copy of a demo's scene (scenes keep state in
// their closures).
func sceneNamed(name string) gifScene {
	for _, sc := range gifScenes() {
		if sc.demo == name {
			return sc
		}
	}
	panic("no scene for " + name)
}

// runGIF runs a scene and pipes its recorded frames to ffmpeg; dry, it
// only runs it, and returns the recorded frame at which until held.
func runGIF(t *testing.T, sc gifScene, cols, rows int, atlas [][]float64, path string, dry bool) (int, error) {
	ss, _ := OpenMock("") // subscriptions stay silent: the script feeds them
	defer ss.Close()
	start := sc.start
	if start.IsZero() {
		start = time.Date(2026, 9, 26, 21, 0, 0, 0, time.FixedZone("-03", -3*3600))
	}
	simNow := start
	clock = func() time.Time { return simNow }
	speed, clockX := sc.speed, sc.clockX
	if speed == 0 {
		speed = 1
	}
	if clockX == 0 {
		clockX = 1
	}
	defer func() { clock = time.Now }()
	demoArg = sc.arg
	defer func() { demoArg = "" }()
	e := find(sc.demo)
	if e == nil {
		return 0, fmt.Errorf("no demo %q", sc.demo)
	}
	d := e.new(sc.specs)
	// The first readings, so Setup finds the sensors reading.
	feed := newGIFFeed(ss)
	gauges, err := d.Setup(ss)
	if err != nil {
		return 0, err
	}
	gauges = append(gauges, screenGauge(ss)...)
	if sc.setup != nil {
		sc.setup(t, d)
	}
	title := e.name
	if sc.specs != nil {
		title += " (" + strings.Join(sc.specs, ",") + ")"
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
	N := int(math.Round(sc.secs * gifFPS))
	F := int(math.Round(sc.fade * gifFPS))
	n := int(math.Round(sc.warm*gifFPS)) + N + F
	rec := int(math.Round(sc.warm * gifFPS))
	var first [][]byte // the first frames, to crossfade the last ones into
	pressed := map[float64]bool{}
	for i := 0; i < n; i++ {
		tt := float64(i) * dt
		simNow = start.Add(time.Duration(tt * clockX * float64(time.Second)))
		// Two readings a frame: sensors report at 60 Hz.
		feed.push(sc.inputs, tt)
		feed.push(sc.inputs, tt+dt/2)
		for at, k := range sc.keys {
			if !pressed[at] && tt >= at {
				pressed[at] = true
				d.Key(k)
			}
		}
		if sc.every != nil {
			sc.every(d, tt)
		}
		f.Resize(cols, rows)
		hudH := DrawHUD(f.View(0, 0, cols, rows), title, palettes[0].name, gifFPS, gauges, ss)
		d.Draw(f.View(0, hudH, cols, rows-hudH), ss, tt*speed, dt*speed)
		if i < rec {
			continue
		}
		if dry {
			if sc.until != nil && sc.until(d, tt) {
				return i - rec, nil
			}
			continue
		}
		rasterize(&f, atlas, rgb, lut)
		if sc.turn != nil {
			turnFrame(rgb, rot, W, H, sc.turn(tt))
			copy(rgb, rot)
		}
		if sc.until != nil && sc.until(d, tt) {
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
		return 0, fmt.Errorf("%s: what should end it never happened", sc.demo)
	}
	for i := 0; i < int(sc.hold*gifFPS); i++ {
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
	for _, s := range g.ss.byKey {
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
		s.push(client.Event{T: int64(t*1e9) + 1, V: append([]float64(nil), v...)})
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
type gifPose struct{ yaw, pitch, roll float64 }

var gifField = Vec3{0, 18.6, 13.5} // Earth's field, µT: magnetic north, and up (as in the southern hemisphere)

func gifMotion(pose func(t float64) gifPose, lin func(t float64) Vec3, t float64) map[string][]float64 {
	at := func(t float64) (Mat3, []float64) {
		p := pose(t)
		q := quatFromEuler([]float64{p.yaw, p.pitch, p.roll})
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
	m := R.T().Apply(gifField)
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
func merge(a map[string][]float64, more map[string][]float64) map[string][]float64 {
	for k, v := range more {
		a[k] = v
	}
	return a
}

// smooth steps from 0 to 1 between a and b, easing both ends.
func ease(a, b, t float64) float64 { return smoothstep(a, b, t) }

// The place scripts are at: a public square, not anyone's home.
var gifPlace = []float64{-34.60372, -58.38159, 6, math.NaN(), math.NaN(), math.NaN(), -9.3, 22.8}

// Where the map stands: GIFMAP=lat,lon to try another spot; the square
// by default, the Obelisco on 9 de Julio.
var gifMapPlace = func() []float64 {
	p := append([]float64(nil), gifPlace...)
	if s := os.Getenv("GIFMAP"); s != "" {
		fmt.Sscanf(s, "%g,%g", &p[0], &p[1])
	}
	return p
}()

func gifScenes() []gifScene {
	return []gifScene{
		candleScene(), compassScene(), navballScene(), kaleidoscopeScene(),
		skyScene(), eyeScene(), donutScene(), scopeScene(),
		detectorScene(), diceScene(), gesturesScene(), homingScene(),
		hourglassScene(), mapScene(), lavalampScene(), snowglobeScene(),
		fluidScene(), pendulumScene(), planetariumScene(), spaceScene(),
		sundialScene(), mazeScene(),
	}
}

// wave is a sum of sines that repeats every T: a hand's small unsteady
// movement, for scripts that must loop.
func wave(t, T float64, amps ...float64) float64 {
	v := 0.0
	for i, a := range amps {
		k := float64(i + 1)
		v += a * math.Sin(2*math.Pi*k*t/T+k*1.7)
	}
	return v
}

// cycle is t's progress through a loop of length T after warm-up, 0..1.
func cycle(t, warm, T float64) float64 { return math.Max(0, t-warm) / T }

// The compass: the phone lying flat, turned once all the way round, a
// little unsteady in the hand.
func compassScene() gifScene {
	const warm, T = 2.0, 10.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		return gifPose{yaw: 360 * c, pitch: wave(t, T, 0, 3, 0, 1), roll: wave(t, T, 0, 0, 2.5)}
	}
	return gifScene{demo: "compass", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifPlace})
		}}
}

// The navball: the phone turned through heading, pitch and roll, back to
// where it began.
func navballScene() gifScene {
	const warm, T = 2.0, 12.0
	pose := func(t float64) gifPose {
		u := cycle(t, warm, T) * 2 * math.Pi
		return gifPose{yaw: 360 * cycle(t, warm, T), pitch: 35 + 40*math.Sin(u), roll: 30 * math.Sin(2*u)}
	}
	return gifScene{demo: "navball", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifPlace})
		}}
}

// The kaleidoscope: turned slowly one way, then back; the pattern unfolds
// and folds back into where it began.
func kaleidoscopeScene() gifScene {
	const warm, T = 2.0, 12.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		return gifPose{yaw: 120 * (1 - math.Cos(2*math.Pi*c)) / 2}
	}
	return gifScene{demo: "kaleidoscope", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 { return gifMotion(pose, nil, t) }}
}

// The sky: the light sensor from covered to sunlight and back: new moon,
// waxing to full, glowing, the sun blazing.
func skyScene() gifScene {
	const warm, T = 1.0, 12.0
	return gifScene{demo: "sky", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			c := cycle(t, warm, T)
			// Up and back down on a log scale, 0.5 lux to 40000.
			x := (1 - math.Cos(2*math.Pi*c)) / 2
			lux := math.Pow(10, -0.3+4.9*x)
			return map[string][]float64{"light": {math.Round(lux)}}
		}}
}

// The eye: held in the hand, it trembles a little and the iris follows;
// the room dims and the pupil opens, brightens and it narrows, bright sun
// and it squints; a hand passes over and it shuts.
func eyeScene() gifScene {
	const warm, T = 2.0, 13.0
	pose := func(t float64) gifPose {
		return gifPose{yaw: wave(t, T, 0, 1.2, 0, 0.8, 0, 0.5), pitch: 60 + wave(t, T, 0, 0, 2.5, 0, 1.5, 0, 1), roll: wave(t, T, 0, 2, 0, 1.4, 0, 0, 0.8)}
	}
	return gifScene{demo: "eye", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			m := gifMotion(pose, nil, t)
			s := math.Max(0, t-warm)
			// Room light, dimming, then bright, then sun, then the room.
			lux := 150.0
			switch {
			case s < 2:
			case s < 4:
				lux = 150 - 130*ease(2, 3, s)
			case s < 6:
				lux = 20 + 1480*ease(4, 5, s)
			case s < 8:
				lux = 1500 + 20000*ease(6, 6.8, s)
			default:
				lux = 21500 - 21350*ease(8, 9.3, s)
			}
			prox := 5.0
			if s > 10.6 && s < 11.4 {
				prox = 0
			}
			m["light"] = []float64{math.Round(lux)}
			m["proximity"] = []float64{prox}
			return m
		}}
}

// The donut: held still in the room while you walk once around it, and
// look at it from above and below; the room's light rises from dim, where
// only its highlights show, to bright, and falls again.
func donutScene() gifScene {
	const warm, T = 2.0, 10.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		return gifPose{yaw: 360 * c, pitch: 70 + 25*math.Sin(2*math.Pi*c), roll: wave(t, T, 0, 2)}
	}
	return gifScene{demo: "donut", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			x := (1 - math.Cos(2*math.Pi*cycle(t, warm, T))) / 2
			lux := math.Round(math.Pow(10, 0.3+3.2*x)) // 2 to 3000 lux
			return merge(gifMotion(pose, nil, t), map[string][]float64{"light": {lux}})
		}}
}

// The scope: accelerometer, gyroscope and light as a hand moves the phone
// about and taps it, and a shadow passes; every trace repeats with the loop.
func scopeScene() gifScene {
	const warm, T = 9.0, 8.0
	pose := func(t float64) gifPose {
		return gifPose{yaw: wave(t, T, 25, 0, 8), pitch: 50 + wave(t, T, 0, 15, 0, 5), roll: wave(t, T, 0, 0, 12)}
	}
	lin := func(t float64) Vec3 {
		p := math.Mod(t, T/2)
		tap := 6 * math.Exp(-(p-1)*(p-1)/0.002)
		return Vec3{0, 0, tap}
	}
	return gifScene{demo: "scope", specs: []string{"accelerometer", "gyroscope", "light"}, warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			m := gifMotion(pose, lin, t)
			m["light"] = []float64{math.Round(180 + 120*math.Sin(2*math.Pi*t/T) - 150*math.Exp(-math.Pow(math.Mod(t, T)-5.5, 2)/0.1))}
			return m
		}}
}

// The candle: the view swings around it, up to look into the pool and
// down; two breaths of air make the flame lean and waver; a hand snuffs
// it (smoke curls up) and it's lit again. It ends as it began: lit,
// looking from the front, the smoke long gone.
func candleScene() gifScene {
	const warm, T = 3.0, 12.0
	rec := func(t float64) float64 { return math.Max(0, t-warm) }
	pose := func(t float64) gifPose {
		u := rec(t) / T * 2 * math.Pi
		return gifPose{yaw: 44 * math.Sin(u), pitch: 20 * math.Sin(2*u)}
	}
	gust := func(s, at, dur float64) float64 { // one push and back
		if s < at || s > at+dur {
			return 0
		}
		return math.Sin(2 * math.Pi * (s - at) / dur)
	}
	lin := func(t float64) Vec3 {
		s := rec(t)
		return Vec3{4 * gust(s, 1.2, 0.9), 0, 3.5 * gust(s, 8.3, 1.0)}
	}
	return gifScene{
		demo: "candle", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			m := gifMotion(pose, lin, t)
			prox := 5.0
			if s := rec(t); t > warm && s > 3.0 && s < 4.6 {
				prox = 0 // a hand over the phone
			}
			m["proximity"] = []float64{prox}
			return m
		},
	}
}

// The detector: zeroed on a desk, swept slowly past a steel bolt (strong),
// then a screw (faint), and back to nothing.
func detectorScene() gifScene {
	const warm, T = 6.0, 10.0
	pose := func(t float64) gifPose {
		return gifPose{yaw: 20 + wave(t, T, 6, 0, 2), pitch: wave(t, T, 0, 1.5), roll: wave(t, T, 0, 0, 1)}
	}
	return gifScene{demo: "detector", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			m := gifMotion(pose, nil, t)
			// The sweep repeats from the start, so the trace at the
			// bottom (the last few seconds) is the same when the loop
			// wraps; the zero, in the first second, finds nothing near.
			s := math.Mod(t-warm+10*T, T)
			// The metal's field, in the phone's frame: it rises and falls as
			// the phone passes over.
			extra := Vec3{0.3, -0.5, 0.8}.Scale(70 * bump(s, 2.2, 0.8)).Add(Vec3{-0.6, 0.2, 0.7}.Scale(9 * bump(s, 7.0, 0.7)))
			u := m["magnetic_field_uncalibrated"]
			m["magnetic_field_uncalibrated"] = []float64{u[0] + extra[0], u[1] + extra[1], u[2] + extra[2], u[3], u[4], u[5]}
			c := m["magnetic_field"]
			m["magnetic_field"] = []float64{c[0] + extra[0], c[1] + extra[1], c[2] + extra[2], c[3]}
			return m
		}}
}

// Dice: six kinds in the tray, at rest; a shake throws them all, they
// tumble and settle, and the total shows; again. The end fades into the
// first throw's dice.
func diceScene() gifScene {
	const warm, T = 5.0, 8.5
	return gifScene{demo: "dice", arg: "1d4+1d6+1d8+1d10+1d12+1d20", warm: warm, secs: T, fade: 0.8,
		setup: func(t *testing.T, d Demo) { d.(*dice).rng = rand.New(rand.NewSource(11)) },
		inputs: func(t float64) map[string][]float64 {
			s := t - warm
			shake := 0.0
			for _, at := range []float64{1.2, 5.0} {
				if s > at && s < at+0.6 {
					shake = 22 * math.Sin(2*math.Pi*(s-at)/0.2)
				}
			}
			return map[string][]float64{"linear_acceleration": {shake, 0.5 * shake, 0.25 * shake}}
		}}
}

// Gestures: a few steps, a chop, a twist, face down and up again, a lift.
func gesturesScene() gifScene {
	const warm, T = 1.0, 10.5
	return gifScene{demo: "gestures", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			s := t - warm
			count := func(times ...float64) float64 {
				n := 0.0
				for _, at := range times {
					if s >= at {
						n++
					}
				}
				return n
			}
			steps := 0.0
			for at := 0.4; at < 2.8; at += 0.55 {
				if s >= at {
					steps++
				}
			}
			flip := 2.0
			if s >= 6.8 && s < 8.0 {
				flip = 1
			}
			return map[string][]float64{
				"step_detector": {1, steps},
				"chop_chop":     {1, count(3.4)},
				"flip_twist":    {1, count(5.1)},
				"flip":          {flip},
				"wake_gesture":  {1, count(9.2)},
			}
		}}
}

// Homing: the phone lying flat, turned once round; the arrow keeps
// pointing at the saved place, a kilometre off.
func homingScene() gifScene {
	const warm, T = 3.0, 10.0
	pose := func(t float64) gifPose {
		return gifPose{yaw: 360 * cycle(t, warm, T), pitch: 20 + wave(t, T, 0, 4), roll: wave(t, T, 0, 0, 3)}
	}
	return gifScene{demo: "homing", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifPlace})
		}}
}

// The hourglass, at its own minute: the first grains fall; the phone is
// turned over (the GIF turns with it) and those few grains fall back, so
// all the sand is on one side; turned again, the grains start to fall, as
// at the start.
func hourglassScene() gifScene {
	const warm, T = 5.5, 4.6
	flip := func(t, at float64) float64 { return 180 * ease(at, at+0.8, t) }
	angle := func(t float64) float64 { // turned about the screen, degrees
		a := flip(t, 0.4) + flip(t, warm-1.1) // before: all the sand to one side, then back
		s := t - warm
		return a + flip(s, 0.5) + flip(s, 0.5+0.8+1.8+0.4) // a loop ends 0.3 s after the second flip, as it began
	}
	return gifScene{demo: "hourglass", warm: warm, secs: T, turn: angle,
		inputs: func(t float64) map[string][]float64 {
			a := (angle(t) + wave(t, T, 0, 1.2, 0, 0.6)) * math.Pi / 180
			g := Vec3{9.81 * math.Sin(a), 9.81 * math.Cos(a), 0.8}
			return map[string][]float64{"gravity": g[:], "accelerometer": g[:]}
		}}
}

// The map around the square, turning as the phone turns, looking left and
// right.
func mapScene() gifScene {
	const warm, T = 2.0, 8.0
	pose := func(t float64) gifPose { // looking around, left and right
		return gifPose{yaw: 50 * math.Sin(2*math.Pi*cycle(t, warm, T)), pitch: 30}
	}
	return gifScene{demo: "map", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifMapPlace, "gps": gifMapPlace})
		},
		every: func(d Demo, t float64) {
			if t < warm-0.5 || t > warm-0.4 {
				return
			}
			// The map data arrives over the network: wait for it.
			m := d.(*mapDemo)
			for i := 0; i < 1800; i++ {
				m.mu.Lock()
				ok, loading, err := m.ways != nil, m.loading, m.lastErr
				m.mu.Unlock()
				if ok && !loading {
					return
				}
				if i%50 == 0 {
					fmt.Printf("map: waiting (loading %v, %q)\n", loading, err)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}}
}

// The lava lamp: a walk once around it; a shake breaks the wax into
// blobs that merge again. The wax won't repeat, so the end fades into
// the start.
func lavalampScene() gifScene {
	const warm, T = 25.0, 10.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		return gifPose{yaw: 360 * c, pitch: 90 + 8*math.Sin(2*math.Pi*c)}
	}
	lin := func(t float64) Vec3 {
		s := t - warm
		if s > 3.5 && s < 4.1 {
			return Vec3{14 * math.Sin(2*math.Pi*(s-3.5)/0.2), 0, 0}
		}
		return Vec3{}
	}
	return gifScene{demo: "lavalamp", warm: warm, secs: T, fade: 1.2,
		inputs: func(t float64) map[string][]float64 { return gifMotion(pose, lin, t) }}
}

// The snow globe: snow drifting down; a hard shake sends it swirling; a
// twist spins the liquid; the view turns a little. The last shake came
// as long before the start as the one in it before the end, so the end
// fades into a start that looks alike.
func snowglobeScene() gifScene {
	const warm, T = 20.0, 12.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		return gifPose{yaw: 25*math.Sin(2*math.Pi*c) + twist(t, warm), pitch: 60 + 8*math.Sin(4*math.Pi*c)}
	}
	lin := func(t float64) Vec3 {
		for _, at := range []float64{warm + 1.2 - T, warm + 1.2} { // a loop apart
			if s := t - at; s > 0 && s < 1.0 {
				return Vec3{15 * math.Sin(2*math.Pi*s/0.25), 9 * math.Sin(2*math.Pi*s/0.33), 0}
			}
		}
		return Vec3{}
	}
	return gifScene{demo: "snowglobe", warm: warm, secs: T, fade: 1.5,
		inputs: func(t float64) map[string][]float64 { return gifMotion(pose, lin, t) }}
}

// twist is a quick turn about the screen and back, for the snow globe.
func twist(t, warm float64) float64 {
	s := t - warm
	return 60 * (ease(6.5, 7.1, s) - ease(7.4, 8.2, s))
}

// The fluid: tipped one way and the other, and shaken.
func fluidScene() gifScene {
	const warm, T = 4.0, 8.0
	return gifScene{demo: "fluid", warm: warm, secs: T, fade: 1.0,
		inputs: func(t float64) map[string][]float64 {
			s := t - warm
			a := 0.0 // tilt about the screen, degrees
			a += 50 * (ease(0.5, 1.5, s) - ease(1.9, 2.9, s))
			a -= 55 * (ease(2.9, 3.9, s) - ease(4.3, 5.3, s))
			a += wave(t, T, 0, 1.5)
			r := a * math.Pi / 180
			g := Vec3{9.81 * math.Sin(r), 9.81 * math.Cos(r), 1}
			lin := Vec3{}
			if s > 5.8 && s < 6.6 {
				lin = Vec3{12 * math.Sin(2*math.Pi*(s-5.8)/0.27), 0, 0}
			}
			acc := g.Add(lin)
			return map[string][]float64{"accelerometer": acc[:], "gravity": g[:], "linear_acceleration": lin[:]}
		}}
}

// The pendulum, at twice the speed: from an empty tray, the swing draws
// its figure; the end fades back to the empty tray.
func pendulumScene() gifScene {
	return gifScene{demo: "pendulum", warm: 0.5, secs: 12, fade: 1.5, speed: 2,
		setup: func(t *testing.T, d Demo) {
			p := d.(*pendulum)
			p.rng = rand.New(rand.NewSource(3))
			clear(p.h)
			p.sand = 1
			p.swing()
		},
		inputs: func(t float64) map[string][]float64 {
			return map[string][]float64{"linear_acceleration": {0, 0, 0}}
		}}
}

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

// The planetarium: the phone held up to the evening sky, turned across
// it and back.
func planetariumScene() gifScene {
	const warm, T = 3.0, 12.0
	pose := func(t float64) gifPose {
		c := cycle(t, warm, T)
		// From the Southern Cross and Centaurus to Scorpius, and back.
		return gifPose{yaw: -235 + 35*math.Sin(2*math.Pi*c), pitch: 133 + 8*math.Sin(4*math.Pi*c)}
	}
	return gifScene{demo: "planetarium", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifPlace})
		}}
}

// Space: a walk round a square of the world's lanes, between its objects,
// turning at each corner and looking up and down as you go; you end where
// you began, facing the same way.
func spaceScene() gifScene {
	const warm, side, turn = 3.0, 3.15, 0.9
	const P = side + turn
	const T = 4 * P
	yaw := func(t float64) float64 {
		s := t - warm
		y := 10 * ease(1, 2, t) // along a lane (the world is turned to face 190 degrees)
		for k := 0.0; k < 4; k++ {
			y += 90 * ease(k*P+side, k*P+side+turn, s)
		}
		return y
	}
	pose := func(t float64) gifPose {
		return gifPose{yaw: yaw(t), pitch: 90 + 7*math.Sin(2*math.Pi*math.Max(0, t-warm)/P)}
	}
	return gifScene{demo: "space", warm: warm, secs: T,
		inputs: func(t float64) map[string][]float64 {
			m := gifMotion(pose, nil, t)
			steps := 0.0
			if s := t - warm; s > 0 {
				k := math.Floor(s / P)
				in := s - k*P
				steps = k * 7
				if in > 0.2 {
					steps += math.Min(7, math.Floor((in-0.2)/0.45)+1)
				}
			}
			m["step_detector"] = []float64{1, steps}
			return m
		}}
}

// The sundial, from before sunrise to after sunset in 13 seconds: the
// sun comes up, the shadow sweeps round the hours, the sun goes down.
func sundialScene() gifScene {
	start := time.Date(2026, 9, 26, 6, 5, 0, 0, time.FixedZone("-03", -3*3600))
	return gifScene{demo: "sundial", start: start, clockX: 3600, warm: 0.2, secs: 13,
		inputs: func(t float64) map[string][]float64 {
			// From the west-south-west, looking north-east: the gnomon's
			// triangle in view, and its shadow as it swings round.
			pose := func(t float64) gifPose { return gifPose{yaw: -70, pitch: 55} }
			return merge(gifMotion(pose, nil, t), map[string][]float64{"location": gifPlace})
		}}
}

// The maze: a hand tilting the phone rolls the ball along the way to the
// goal, across the bridges; a moment after the ball drops in, the board
// fades back to the start. The board is large at this size, so it runs at
// 1.6 times the speed.
func mazeScene() gifScene {
	const speed = 1.6
	var m *maze
	var gx, gy, last float64
	var solvedAt float64 = -1
	return gifScene{demo: "maze", warm: 1, secs: 60, speed: speed, fade: 0.7,
		setup: func(t *testing.T, d Demo) { m = d.(*maze) },
		until: func(d Demo, t float64) bool {
			if m.state == solved && solvedAt < 0 {
				solvedAt = t
			}
			// The banner a moment, well before the next board (the demo
			// waits winDur of its own time, which runs faster here).
			return solvedAt >= 0 && (t-solvedAt)*speed > winDur-1
		},
		inputs: func(t float64) map[string][]float64 {
			ax, ay := 0.0, 0.0
			if m != nil && m.cells != nil && m.state == rolling {
				ax, ay = mazePilot(m)
			}
			k := math.Min(1, (t-last)*speed/0.15) // a hand, not a servo (in the demo's time)
			last = t
			gx += (ax - gx) * k
			gy += (ay - gy) * k
			// The board's pull is (-x, +y) of gravity on the screen.
			sx, sy := -gx/tiltK, gy/tiltK
			g := Vec3{sx, sy, math.Sqrt(math.Max(0, 9.81*9.81-sx*sx-sy*sy))}
			return map[string][]float64{"gravity": g[:], "accelerometer": g[:]}
		}}
}

// mazePilot is the pull (cells/s^2) that rolls the ball along the way to
// the goal: down the middle of each passage, slowing for the turns.
func mazePilot(m *maze) (float64, float64) {
	cur := m.cellAt(m.x, m.y)
	goal := m.cellAt(m.goal[0], m.goal[1])
	// Which way from each cell leads to the goal.
	next := make([]int, len(m.cells))
	for i := range next {
		next[i] = -1
	}
	next[goal] = goal
	queue := []int{goal}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for d, dv := range dirs {
			if !m.cells[c].open[d] {
				continue
			}
			n := (c/m.cols+dv[1])*m.cols + c%m.cols + dv[0]
			if next[n] < 0 && !m.cells[n].hole {
				next[n] = c
				queue = append(queue, n)
			}
		}
	}
	center := func(i int) [2]float64 { return [2]float64{float64(i%m.cols) + 0.5, float64(i/m.cols) + 0.5} }
	pull := func(want, v float64) float64 { return math.Max(-4, math.Min(4, 2.8*(want-v))) }
	if cur == goal || next[cur] < 0 {
		g := m.goal
		return pull(1.5*(g[0]-m.x), m.vx), pull(1.5*(g[1]-m.y), m.vy)
	}
	// The straight run ahead, to the next turn.
	c := center(cur)
	n1 := next[cur]
	dx, dy := float64(n1%m.cols-cur%m.cols), float64(n1/m.cols-cur/m.cols)
	end, run := n1, 1
	for end != goal && run < 12 {
		n2 := next[end]
		if float64(n2%m.cols-end%m.cols) != dx || float64(n2/m.cols-end/m.cols) != dy {
			break
		}
		end, run = n2, run+1
	}
	e := center(end)
	// Where the ball will be by the time a tilt takes effect.
	px, py := m.x+m.vx*0.25, m.y+m.vy*0.25
	dist := (e[0]-px)*dx + (e[1]-py)*dy // along the run, to its last cell's middle
	vmax, brake := 3.0, 1.5
	for c2 := cur; ; c2 = next[c2] {
		if m.cells[c2].bridge {
			vmax, brake = 1.3, 1.0 // careful over a bridge: no walls to lean on
		}
		if c2 == end {
			break
		}
	}
	along := math.Min(vmax, math.Sqrt(2*brake*math.Max(0, dist))+0.15)
	// Across: back to the passage's middle line.
	crossX, crossY := (c[0]-m.x)*math.Abs(dy), (c[1]-m.y)*math.Abs(dx)
	wantX := dx*along + 3*crossX
	wantY := dy*along + 3*crossY
	return pull(wantX, m.vx), pull(wantY, m.vy)
}
