# sensordemo

TUI ASCII demos of the phone's sensors, fed by [`sensord`](../sensord). Meant to be viewed from within [`Termux`](https://github.com/termux/termux-app).

Developed and tested on a Moto G17 Power.

```sh
sensordemo list               # the demos
sensordemo donut              # a demo by name
sensordemo gyroscope          # a sensor: picks a demo that uses it
sensordemo light,proximity    # several sensors: picks a demo that uses all of them
sensordemo --gray fluid       # start in grayscale
sensordemo hourglass 5m       # some demos take an argument
```

Keys shared by every demo: 
- `q` quit
- `c` cycle color palettes
- `?` info and help

## Demos

| | | |
|:-:|:-:|:-:|
| <code>candle</code><br><sub>game rotation vector, linear acceleration, proximity</sub><br><img src="demos/candle/candle.gif" width="240" alt="candle"> | <code>compass</code><br><sub>rotation vector, magnetometer, location</sub><br><img src="demos/compass/compass.gif" width="240" alt="compass"> | <code>detector</code><br><sub>uncalibrated magnetometer, gravity</sub><br><img src="demos/detector/detector.gif" width="240" alt="detector"> |
| <code>dice</code><br><sub>linear acceleration</sub><br><img src="demos/dice/dice.gif" width="240" alt="dice"> | <code>donut</code><br><sub>game rotation vector, gyroscope, light</sub><br><img src="demos/donut/donut.gif" width="240" alt="donut"> | <code>eye</code><br><sub>proximity, light, game rotation vector, accelerometer</sub><br><img src="demos/eye/eye.gif" width="240" alt="eye"> |
| <code>fluid</code><br><sub>accelerometer</sub><br><img src="demos/fluid/fluid.gif" width="240" alt="fluid"> | <code>gestures</code><br><sub>Moto gestures, step detector</sub><br><img src="demos/gestures/gestures.gif" width="240" alt="gestures"> | <code>homing</code><br><sub>location, rotation vector</sub><br><img src="demos/homing/homing.gif" width="240" alt="homing"> |
| <code>hourglass</code><br><sub>gravity</sub><br><img src="demos/hourglass/hourglass.gif" width="240" alt="hourglass"> | <code>kaleidoscope</code><br><sub>gyroscope</sub><br><img src="demos/kaleidoscope/kaleidoscope.gif" width="240" alt="kaleidoscope"> | <code>lavalamp</code><br><sub>game rotation vector, linear acceleration</sub><br><img src="demos/lavalamp/lavalamp.gif" width="240" alt="lavalamp"> |
| <code>map</code><br><sub>location, rotation vector</sub><br><img src="demos/map/map.gif" width="240" alt="map"> | <code>maze</code><br><sub>gravity</sub><br><img src="demos/maze/maze.gif" width="240" alt="maze"> | <code>navball</code><br><sub>rotation vector, magnetometer, location</sub><br><img src="demos/navball/navball.gif" width="240" alt="navball"> |
| <code>pendulum</code><br><sub>linear acceleration</sub><br><img src="demos/pendulum/pendulum.gif" width="240" alt="pendulum"> | <code>planetarium</code><br><sub>rotation vector, location</sub><br><img src="demos/planetarium/planetarium.gif" width="240" alt="planetarium"> | <code>scope</code><br><sub>any</sub><br><img src="demos/scope/scope.gif" width="240" alt="scope"> |
| <code>sky</code><br><sub>light</sub><br><img src="demos/sky/sky.gif" width="240" alt="sky"> | <code>snowglobe</code><br><sub>linear acceleration, gravity, game rotation vector, gyroscope</sub><br><img src="demos/snowglobe/snowglobe.gif" width="240" alt="snowglobe"> | <code>space</code><br><sub>game rotation vector, step detector</sub><br><img src="demos/space/space.gif" width="240" alt="space"> |
| <code>sundial</code><br><sub>rotation vector, location</sub><br><img src="demos/sundial/sundial.gif" width="240" alt="sundial"> |  |  |

## Building

```sh
go build -o $PREFIX/bin/sensordemo .
```

The `sensord` client comes from `../sensord` via a `replace` in `go.mod`.

To check a demo without the phone, `--snapshot WxH` renders about a second off-screen and prints the last frame, and `--mock` feeds it fixed readings:

```sh
sensordemo --snapshot 70x30 --mock 'accelerometer=6,6,3' fluid
sensordemo --snapshot 70x30 --mock 'orient=45,0,0' compass       # yaw,pitch,roll
sensordemo --snapshot 70x30 --mock 'CHOP_CHOP=1' gestures
```

`--frames N` renders longer (30 frames is about a second).

To also see what the colors look like, the preview test renders a frame to a PNG, with each character as a patch of its terminal color and a rough glyph shape:

```sh
PREVIEW='hourglass:70x56:90:gravity=0,9.8,0:/tmp/hg.png' go test -run TestPreview ./tests/
```

## Data sources

### Map

The `map` demo downloads streets, buildings, water and parks data from the public [Overpass API](overpass-api.de). That request contains your approximate position. Results are cached in `~/.cache/sensordemo/osm/`, so revisiting an area doesn't ask again. Delete that directory to clear it. 

### Planetarium

The `planetarium`'s stars and constellations come from [d3-celestial](https://github.com/ofrohn/d3-celestial), whose star data comes from the HYG database. Planet positions use JPL's approximate Keplerian elements.
