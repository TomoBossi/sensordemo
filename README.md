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

| Demo | Sensors | What you do |
|---|---|---|
| `donut` | game rotation vector (+ gyroscope, light) | the donut.c torus stays still in the room; turn the phone to see it from other sides; its brightness follows the room light. `r` recenter, `a` auto-spin, `+`/`-` zoom, `l` light on/off |
| `fluid` | accelerometer | tilt to pour, shake for waves, lay flat for zero-g. `s` splash, `r` refill, `+`/`-` amount |
| `space` | game rotation vector (+ step detector) | the screen is a window into a ray-marched world (pillars, trees, blocks, portals, crystals, a rare monolith; a sun that lights and shadows them, a ringed planet, a moon, fixed stars); turn to look around. `w`/`s` step, `g` glide forward, `p` turns step mode off/on (on at start: real steps move you, and its bar beats with each step) |
| `navball` | rotation vector + magnetometer | a 3D globe of directions fixed in the room (sky on top, ground below, N E S W, elevation lines), seen from outside through the phone: flat, you look down on it like a compass rose; upright, you see its side. Heading, pitch and roll of the phone below |
| `compass` | rotation vector + magnetometer | a flat compass rose that turns so N points north, following the phone's long axis however it is rolled; the heading in big digits |
| `map` | location + rotation vector | OpenStreetMap around you in ASCII, turning with the phone. `+`/`-` zoom, `n` north-up |
| `gestures` | Moto gestures, step detector | chop, twist, flip, lift or walk: each burst of big letters is one sensor event |
| `eye` | proximity + light + orientation | a ray-marched 3D eye lit by a lamp above you: turn the phone and the light and glint move; cover the top of the phone and it closes; tilt and it looks that way; light sets the pupil size, and direct sun makes it squint |
| `sky` | light | one disc driven by light on a log scale: a new moon among stars in the dark, waxing to full with room light, glowing brighter, then turning cell by cell into a churning sun with corona and flares in sunlight. The moon is the real near side, upside down as seen from the southern hemisphere |
| `hourglass` | gravity | a sand timer: flip the phone to turn it over; grains really fall and pile toward wherever is down; the neck is metered to the duration (`sensordemo hourglass 5m`, default 1m). `r` refill |
| `snowglobe` | accelerometer + linear acceleration | a glass dome with a cabin, a snowman and a pine tree: shake the phone for a flurry that slowly settles on everything; tilt it and the snow drifts. `space` shake, `r` reset |
| `maze` | gravity | a marble maze: tilt to roll the ball to the flag; holes in dead ends send you back; your time and best time. `n` new maze, `r` restart |
| `detector` | magnetometer | a metal detector: zero it away from metal (`c`), then sweep over walls and objects; a needle dial, pings that speed up near steel and magnets, a strip chart, and optional vibration (`b`) |
| `homing` | location + rotation vector | a 3D arrow over a compass disc pointing to a saved place, with the distance and walking time. Save where you stand with `sensordemo homing save` (or `save:NAME`), point with `sensordemo homing [NAME]`; places stay in `~/.config/sensordemo/places.json` |
| `scope` | any | an oscilloscope: every value of any sensors as scrolling traces. `space` pause |

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
