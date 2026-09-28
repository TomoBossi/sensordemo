# sensordemo

Full-terminal ASCII demos of the phone's sensors, fed by
[sensord](../sensord). Each one is meant to be fun to look at, and it's also
a sensor check: a data strip on top shows the live values on bars, along with
the rate actually delivered.

```sh
sensordemo list               # the demos
sensordemo donut              # a demo by name
sensordemo gyroscope          # a sensor: picks the demo that uses it
sensordemo light,proximity    # several sensors: a demo that uses all of them,
                              # or the scope, which shows anything
sensordemo --gray fluid       # start in grayscale (c cycles palettes)
sensordemo hourglass 5m       # some demos take an argument
```

Keys in every demo: `q` quit, `c` cycle colors (each demo's own palette first,
then gray, amber, green, ice, fire, violet), `?` help for that demo. The
picture follows the terminal size frame by frame, so opening the keyboard or
rotating the phone just re-lays it out.

## Demos

Each GIF is the demo as it runs on the phone (97x94 cells, a small font),
data strip included, fed scripted sensor readings; see [the GIFs](#the-gifs).
`sensordemo list` and each demo's `?` tell what they do.

| | | |
|:-:|:-:|:-:|
| <code>candle</code><br><img src="demos/candle/candle.gif" width="240" alt="candle"> | <code>compass</code><br><img src="demos/compass/compass.gif" width="240" alt="compass"> | <code>detector</code><br><img src="demos/detector/detector.gif" width="240" alt="detector"> |
| <code>dice</code><br><img src="demos/dice/dice.gif" width="240" alt="dice"> | <code>donut</code><br><img src="demos/donut/donut.gif" width="240" alt="donut"> | <code>eye</code><br><img src="demos/eye/eye.gif" width="240" alt="eye"> |
| <code>fluid</code><br><img src="demos/fluid/fluid.gif" width="240" alt="fluid"> | <code>gestures</code><br><img src="demos/gestures/gestures.gif" width="240" alt="gestures"> | <code>homing</code><br><img src="demos/homing/homing.gif" width="240" alt="homing"> |
| <code>hourglass</code><br><img src="demos/hourglass/hourglass.gif" width="240" alt="hourglass"> | <code>kaleidoscope</code><br><img src="demos/kaleidoscope/kaleidoscope.gif" width="240" alt="kaleidoscope"> | <code>lavalamp</code><br><img src="demos/lavalamp/lavalamp.gif" width="240" alt="lavalamp"> |
| <code>map</code><br><img src="demos/map/map.gif" width="240" alt="map"> | <code>maze</code><br><img src="demos/maze/maze.gif" width="240" alt="maze"> | <code>navball</code><br><img src="demos/navball/navball.gif" width="240" alt="navball"> |
| <code>pendulum</code><br><img src="demos/pendulum/pendulum.gif" width="240" alt="pendulum"> | <code>planetarium</code><br><img src="demos/planetarium/planetarium.gif" width="240" alt="planetarium"> | <code>scope</code><br><img src="demos/scope/scope.gif" width="240" alt="scope"> |
| <code>sky</code><br><img src="demos/sky/sky.gif" width="240" alt="sky"> | <code>snowglobe</code><br><img src="demos/snowglobe/snowglobe.gif" width="240" alt="snowglobe"> | <code>space</code><br><img src="demos/space/space.gif" width="240" alt="space"> |
| <code>sundial</code><br><img src="demos/sundial/sundial.gif" width="240" alt="sundial"> |  |  |

## Compass accuracy

`compass` and `navball` show true north when sensord has a location fix
(magnetic north is about 10 degrees off in Buenos Aires), and a status line
comparing the measured magnetic field with the one Earth should have there.
A big mismatch means magnets or steel nearby, the usual cause of a wrong
compass indoors. Low calibration asks for a figure 8.

## Data strip

The title line shows the frame rate actually drawn (`fps`). Below it, one line
per value: label, number, unit, bar, and on the right how that sensor
delivers data: `Hz` for steady streams (readings per second from sensord,
independent of the frame rate), `ev` for sensors that report only on change
or on events (the number of readings received so far).

- Signed values (acceleration, rotation) sit on a bar centered on zero that
  widens to fit what it has seen.
- Light uses an asymptotic bar, lux/(lux+300): typical indoor light gets most
  of the bar, and sunlight crowds toward the end without running off.
- The step counter starts at its first value and counts up from there.
- Event sensors (the step detector) beat: the bar fills on each event and
  drains. Two-state sensors (proximity, which on this phone reports only 0
  or 5 cm) show as an on/off NEAR / far bar.

## Speed

The heavy demos (space, eye, navball) render their rows on all CPU cores,
and fluid simulates at a normal screen's resolution and scales the picture
up. So a smaller font gives more detail without slowing down: all demos hold
30 fps at 150x90 cells on this phone.

## The map and privacy

`map` downloads streets, buildings, water and parks from the public Overpass
API (overpass-api.de). That request contains your approximate position: the
center and radius of the area, a few hundred meters. Results are cached in
`~/.cache/sensordemo/osm/`, so revisiting an area doesn't ask again. Delete
that directory to clear it. Map data © OpenStreetMap contributors, ODbL.

## Data

The planetarium's stars and constellations come from
[d3-celestial](https://github.com/ofrohn/d3-celestial) (BSD-3-Clause), whose
star data is from the HYG database (CC BY-SA 2.5); planet positions use
JPL's approximate Keplerian elements. `skydata.go` is generated from them.

## The GIFs

Each demo's GIF is rendered offline by its own test, not recorded: the
demo runs on simulated time, fed a script of sensor readings (a hand
turning the phone, a shake, a covered sensor, lamp light rising and
falling, a place and a time of day), and is drawn with the app's own data
strip. Each frame is drawn in the phone's font (Android's monospace, as
Termux uses it) at the phone's cell size, shrunk to 4x8 pixels a cell in
linear light, and ffmpeg makes the GIF. Most loop exactly; those whose
state can't repeat (dice, snow, wax, water, sand, the maze) fade their
end into their start. Places are public ones in Buenos Aires (the
Obelisco; Plaza de Mayo for `homing`), and the demos' saved places,
calibration and map cache are kept apart from the real ones.

```sh
GIFS=all go test -run TestGIF -timeout 60m ./demos/...     # all of them
GIFS=candle,eye GIFSIZE=106x109 go test -run TestGIF ./demos/...   # some, at another size
```

Each GIF is written next to its script, `demos/<name>/gif_test.go`. It
needs ffmpeg (`pkg install ffmpeg`) and fetches map data for `map`.

## Layout

- `main.go` and friends: the app (command line, terminal, frame loop,
  help).
- `internal/core`: what the demos share: the frame and its colors, the
  sensor streams and the mock, the data strip, vectors and rotations,
  noise, distance functions, color ramps, the Sun and planets, the
  registry demos add themselves to.
- `internal/gifs`: the GIF renderer.
- `demos/<name>`: one demo each: its code, its tests, its GIF's script
  and the GIF.

## Building

```sh
go build -o $PREFIX/bin/sensordemo .
```

The sensord client comes from `../sensord` via a `replace` in go.mod.

To check a demo without the phone, `--snapshot WxH` renders about a second
off-screen and prints the last frame, and `--mock` feeds it fixed readings:

```sh
sensordemo --snapshot 70x30 --mock 'accelerometer=6,6,3' fluid
sensordemo --snapshot 70x30 --mock 'orient=45,0,0' compass       # yaw,pitch,roll
sensordemo --snapshot 70x30 --mock 'CHOP_CHOP=1' gestures
```

`--frames N` renders longer (30 frames is about a second).

To see a demo in color without the phone, the preview test renders a frame
to a PNG, with each character as a patch of its terminal color and a rough
glyph shape:

```sh
PREVIEW='hourglass:70x56:90:gravity=0,9.8,0:/tmp/hg.png' go test -run TestPreview .
```
