# go-gpui

A proof-of-concept port of [Zed's GPUI](https://github.com/zed-industries/zed/tree/main/crates/gpui)
UI framework to Go. Retained entities, per-frame element trees, a flexbox
layout engine, a keymap/action system and a text stack, all drawn by a **CPU
rasterizer** - no GPU and no cgo. The same view renders to an ebiten window or
straight to a PNG, which makes UI snapshots and headless tests cheap.

Status: POC. The core (`gpui/`) is covered by tests and 17 examples render
headless; the interactive ebiten window has not been exercised in the agent
CI environment (no Metal device). See [_docs/roadmap.md](_docs/roadmap.md).

## Quick start

```
go run ./examples/todo                 # interactive window
go run ./examples/todo -png todo.png   # headless: render one frame and exit
```

Every example takes `-png PATH`, `-width`, `-height`, `-smoke MS`. In a running
window, `cmd-shift-s` (macOS) / `ctrl-shift-s` saves the current frame as
`snapshot-YYYYMMDD-HHMMSS.png`.

## Hello world

```go
package main

import (
	"log"

	"go-gpui/gpui"
	"go-gpui/gpui/ebitenplatform"
)

func main() {
	hello := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Bg(gpui.Gray50).ItemsCenter().JustifyCenter().
			Child(gpui.NewText("Hello, GPUI").TextSize(32).FontWeight(700))
	})
	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: 400, Height: 200, Title: "Hello", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return hello }); err != nil {
			log.Fatal(err)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## Features

Supported: flexbox (wrapping, grow/shrink/basis, margins incl. auto, row/column gaps, min/max),
absolute positioning,
`Div`/`Text`/`StyledText`/`TextInput`, `UniformList`, scroll containers and
scrollbars with thumb dragging and track paging, borders, rounded corners and clips, linear gradients (sRGB/Oklab),
box shadows (outset/inset), opacity, hover/focus/focus-visible/active styles, text
style inheritance, font family with system fallback, HarfBuzz shaping and
wrapping, mouse/keyboard events, actions and key contexts, tab order, clipboard,
entities with observe/emit, headless rendering and input injection.

Not supported yet: animation API (examples step state in `Render`), images and
SVG, CSS grid, colour
emoji, IME composition UI, multiple windows in the ebiten backend, italic and
intermediate font weights, drag and drop, key chords. The full list is in
`_docs/roadmap.md` (section 3, "Open work").

## Layout of the repo

```
gpui/                core package (stdlib + x/image + go-text/typesetting)
gpui/ebitenplatform/ ebiten window backend, clipboard, snapshot key
examples/            17 runnable programs (docs/examples.md)
docs/                user documentation
_docs/               internal index, contract, roadmap, project structure
AGENTS.md            rules for AI agents working in the repo
```

## Documentation

- [docs/getting-started.md](docs/getting-started.md) - tutorial from first
  window to headless tests.
- [docs/examples.md](docs/examples.md) - the 17 examples and what each shows.
- [docs/README.md](docs/README.md) - index: architecture and per-subsystem
  concept pages.
- [_docs/index.md](_docs/index.md) - entry point for internal docs: API
  contract, roadmap, project structure.

Verify a checkout with `gofmt -l .`, `go vet ./...`, `go test ./...`.
