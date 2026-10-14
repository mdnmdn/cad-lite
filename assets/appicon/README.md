# Application icon

The logo of the macOS `.app`, the Windows `.exe` and the window. Unlike the UI icons in
`assets/icons/` it is full colour. The mark is a drafting **line** ending in an endpoint **grip**
(blue dot) and an **endpoint snap square with crosshair** (light blue), on the canvas-dark rounded
square.

## Spec

- Sources: `appicon.svg` (1024x1024 viewBox) and `appicon-small.svg`, a simplified, heavier
  drawing the generator uses for sizes up to 32 px (no crosshair, thicker strokes).
- Big Sur grid: 824x824 rounded rect at (100,100), corner radius 185, so a 100 px transparent
  margin. Everything inside is at least 40 px from the plate's edge.
- Palette (ui/theme.go): plate gradient `#2D3744` to `#1A2027` (canvas `#212830` family), line
  `#E8EDF2`, grip `#3B82F6` (uiAccent), snap square and crosshair `#60A5FA` (uiOn).
- Must stay legible at 16 px: the diagonal line, the blue dot at the lower left and the square at
  the upper right. The crosshair is decoration and may vanish. Check the 8x enlarged 16 px
  tile in the review sheet.
- Only gradients and strokes; no text, filters or images, so any rasteriser agrees.

## Outputs (committed; `*.png` is gitignored repo-wide, `.gitignore` negates `assets/appicon/*.png`)

| File | Contents |
|---|---|
| `AppIcon.icns` | the ten iconset sizes 16 to 512 and @2x (1024), written by Pillow plus appended icp4/icp5 for 16 and 32 @1x |
| `AppIcon.ico` | PNG entries at 16, 24, 32, 48, 64, 128, 256 |
| `window-{16,32,48,64,128,256}.png` | embedded by the Go package `appicon` (`appicon.Images()`) |
| `source.sha256` | hash of both SVGs, so `check` can detect stale outputs |

## Regenerate

```
just appicon          # build every output
just appicon-sheet    # review sheet in assets/appicon/preview/ (gitignored): read sheet.png
uv run tools/icons/appicon.py check
```

Needs `uv` only (resvg-py and Pillow are inline script dependencies). After editing an SVG, build,
read the sheet at 16/24/32 on light and dark, redraw, and build again.
