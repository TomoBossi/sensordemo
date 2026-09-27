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
| `kaleidoscope` | gyroscope | a two-mirror kaleidoscope: a twelve-fold mandala of jewel-colored glass, lit from behind, the colors multiplying where pieces overlap, with leading, bright bevels and a glow at the center. Turn the phone about its screen as you'd turn a kaleidoscope and the glass turns against the mirrors (loose beads lag and settle); turn back and the pattern returns; hold still and it rests. `a` turn by itself, `r` new glass |
| `lavalamp` | game rotation vector (+ linear acceleration) | a 3D lava lamp, ray marched: it stays put in the room like the donut, so turn the phone to walk around it, the chrome's shine and the glass's reflections shifting with the room's light. Hot wax pinches off the pool on the bulb, rises to the top, cools and sinks back; the violet liquid glows, most near the bulb and around hot wax, and its light falls on the room around the lamp, following what's inside. A hard shake breaks the wax into smaller blobs, which merge again as they meet. `r` face it again, `n` fresh wax, `space` shake |
| `candle` | game rotation vector (+ linear acceleration, proximity) | a 3D candle, ray marched from distance functions and framed on its flame: blue at the root, a white core, orange and red tips, gathered through the flame's volume. The wax shows only near the top, lit by the flame and glowing through the uneven rim and the dried drips that spilled over its notches; the rim's lips glint white and the pool of molten wax around the wick shines with a trembling blur of the flame. Turn the phone to look around it, within limits, as in the eye; move it and the air it leaves behind blows on the flame, which leans, whips back and wavers. Cover the proximity sensor to snuff it: a curl of smoke, a fading ember, the pool slowly setting. `space` snuff or light, `r` face it again |
| `pendulum` | linear acceleration | a sand pendulum over a round tray, seen from above at a slant: a brass funnel on a Y-hung cord swings with one period across and another along, so its path is a Lissajous figure that slowly turns and shrinks, evenly, as friction takes the swing. Sand leaks from the funnel and builds up along the path in relief, sliding where it piles too steep; thin lines are drawn with characters that follow them. Move the phone to push it (the tray moves, the funnel lags); shake hard to level the sand. `space` a fresh swing, `f` next ratio (2:3, 3:4, 1:2, 1:1, 3:5, 4:5), `s` smooth the sand and refill |
| `koi` | linear acceleration, proximity, light, gravity | a koi pond seen from above. The surface carries ripples (the wave equation), which bend the view of the pebbled bottom, draw the sun's caustics on it and glint. Koi of seven varieties swim beneath, each a spine following its head with a beating tail and rayed fins; they wander, keep off the bank and each other, rise and sink, and cast shadows on the bottom that fall further the higher they swim. Lily pads, some in flower, drift down the phone's tilt and on the ripples. Tap the phone to scatter food and the koi come up for it; cover the proximity sensor and they bolt for the depths; in the dark, dusk falls and the moon shows in the water. `space` feed, `n` day, night or follow the light, `r` a new pond |
| `sundial` | rotation vector, location | a brass horizontal sundial on a stone pedestal, ray marched, laid out for your latitude: the gnomon's edge rises at your latitude toward the celestial pole and the hour lines fan out at the angles it gives them. It stands facing true north and the Sun, placed from your location and the time, lights it and casts the gnomon's shadow, which reads the sun's time; the caption compares it with the clock and gives the day's sunrise, noon and sunset. Point the phone somewhere to look at the dial from that side, hold it flat to look down on it. `+`/`-` time-lapse, `[`/`]` a month back or ahead, `space` back to now |
| `map` | location + rotation vector | OpenStreetMap around you in ASCII, turning with the phone. `+`/`-` zoom, `n` north-up |
| `gestures` | Moto gestures, step detector | chop, twist, flip, lift or walk: each burst of big letters is one sensor event |
| `eye` | proximity + light + orientation | a ray-marched 3D eye lit by a lamp above you: turn the phone and the light and glint move; cover the top of the phone and it closes; tilt and it looks that way; light sets the pupil size, and direct sun makes it squint |
| `sky` | light | one disc driven by light on a log scale: a new moon among stars in the dark, waxing to full with room light, glowing brighter, then turning cell by cell into a churning sun with corona and flares in sunlight. The moon is the real near side, upside down as seen from the southern hemisphere |
| `hourglass` | gravity | a sand timer in a turned walnut and brass frame, ray marched once and seen from a little above (brass collars and beading, an inlaid band, bun feet, turned spindles, a brass ring at the neck, glass that mirrors the room's lights): flip the phone to turn it over. Thousands of fine grains (eight per character) fall along the phone's real down direction and settle at sand's natural slope; tilt it and they avalanche gradually. The neck is metered so a full bulb empties in the chosen time (`sensordemo hourglass 5m`, default 1m). `r` refill |
| `snowglobe` | linear acceleration + gravity (+ game rotation vector, gyroscope) | a 3D glass globe on a turned wooden base with a village inside (a cabin with firelit windows, pines, a lamppost, a snowman). Shake it and the snow swirls through the whole globe, harder shakes lifting more, and the globe sways on its base; twist the phone to spin the liquid. Turn the phone and you see the globe from that side, with the room's lights gliding over the glass; hold still and it eases back to the front. The snow drifts down and piles up on the ground, roof and branches, slumping where it's too steep; tilt far enough and it slides. `space` shake, `r` start over |
| `maze` | gravity | a marble maze seen from above in 3D: a big shaded ball that rolls with momentum (its stripe turns as it rolls) and bounces a little off walls; tilting shows the walls' sides and moves the light and shadows. Some stretches are bridges: passages as wide as any other but with no walls: roll off one, or into a hole, and it falls back to the start. Each solved board adds bridges. `n` new board, `r` restart, `b` vibrate on bumps |
| `dice` | linear acceleration | real 3D dice in a felt tray, ray cast with numbers (pips on a d6) on their faces and shadows: shake the phone and every die is thrown its own random way; rigid bodies bounce off the walls, the glass lid and each other, and the total shows when they rest. Choose them in dice notation: `sensordemo dice 3d20`, `sensordemo dice 1d6+1d10+1d20` (d4, d6, d8, d10, d12, d20; default 2d6). `space` throw |
| `detector` | uncalibrated magnetometer + gravity | a metal detector for iron, steel, nickel and magnets (it can't tell them apart, and aluminum, copper, brass, gold and silver don't show). First it calibrates itself: turn the phone through the six poses on screen, and it fits the magnetometer's offset so turning the phone doesn't move the needle (saved in `~/.local/state/sensordemo/magcal.json`; `k` redoes it). `z` zeroes it like a scale's tare, once the phone is still, and the zero stays put. A log dial, pings that speed up, a strip chart, and optional vibration (`b`) |
| `homing` | location + rotation vector | a 3D arrow over a compass disc pointing to a saved place, with the distance and walking time. Save where you stand with `sensordemo homing save` (or `save:NAME`), point with `sensordemo homing [NAME]`; places stay in `~/.config/sensordemo/places.json` |
| `planetarium` | rotation vector + location | point the phone at the sky: the real stars (to magnitude 5, colored by temperature), constellation figures and names, planets, Sun and Moon (with phase) in that direction right now, with the horizon and cardinal points. `+`/`-` time-lapse, `l` lines, `n` names |
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

## Data

The planetarium's stars and constellations come from
[d3-celestial](https://github.com/ofrohn/d3-celestial) (BSD-3-Clause), whose
star data is from the HYG database (CC BY-SA 2.5); planet positions use
JPL's approximate Keplerian elements. `skydata.go` is generated from them.

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
