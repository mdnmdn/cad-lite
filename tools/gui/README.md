# tools/gui: the real window under Xvfb

Runs `cmd/cad` (the real ebiten window) on a virtual X display with Mesa software GL, drives it with
xdotool and captures screenshots. Needs `Xvfb`, `xdotool`, `import` (ImageMagick) and, for `-video`,
`ffmpeg`. On Ubuntu: `apt-get update && apt-get install -y xvfb xdotool x11-utils imagemagick ffmpeg`.

```
go run ./tools/gui [-out DIR] [-video] [-bin PATH] scenario.gui...   # just gui-run
CADLITE_GUI_TEST=1 go test -count=1 ./tools/gui                        # just gui-test
```

The driver builds `cmd/cad` into `_tmp/gui/cad` (or uses `-bin`), starts Xvfb on a free display
(`-displayfd`, 24-bit colour, screen = window size, no window manager, scale factor 1) with
`LIBGL_ALWAYS_SOFTWARE=1`, waits for the `Cad-lite` window, runs the steps, quits the app with SIGTERM
and writes `OUT/<scenario>/`: the PNGs, `app.log`, `drawing.dxf` (from `-save-on-exit`) and
`session.mp4` with `-video`. Exit status is non-zero if any scenario fails.

## Scenario language

One verb per line; `#` comments; double quotes group words (`\n` is Enter inside `type`).
Coordinates are window-relative logical pixels (scale 1).

| Verb | Meaning |
|---|---|
| `app FLAGS...` | `cmd/cad` flags (`-demo`, `-open f.dxf`, ...); `size W H` sets the window size |
| `move X Y` | move the pointer |
| `click [left\|middle\|right] X Y`, `dclick ...` | click (left by default) |
| `drag X1 Y1 X2 Y2 [button]` | press, move in 8 steps, release |
| `key combo...` | xdotool key names with modifiers: `Return`, `Escape`, `F2`, `ctrl+z`; each held 120 ms |
| `type "text"` | type characters; `\n` presses Return |
| `wheel DY [X Y]` | DY > 0 scrolls down, < 0 up, one notch each |
| `hwheel DX [X Y]` | DX > 0 scrolls right, < 0 left, one horizontal notch each |
| `hold MOD` / `release MOD` | keep `ctrl`, `shift`, `alt` or `super` down across other steps (a Ctrl+wheel) |
| `wait MS` | sleep |
| `shot NAME` | save `NAME.png` of the window |
| `expect-diff A B` / `expect-same A B` | two shots differ / are pixel-identical |
| `expect-count TYPE N` | after quitting, the saved DXF has N entities of that DXF type |
| `quit` | quit early (otherwise at the end) |

Key presses are held because ebiten polls key state once per tick and software GL ticks are slow: an
instant press-release (`xdotool key`) is sometimes lost.
