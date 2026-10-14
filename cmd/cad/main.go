// Command cad is the go-draw desktop application: an AutoCAD-style 2D
// drafting editor built on the gpui library and shown in an ebiten window.
//
// With -png it runs headlessly instead: it seeds a drawing (the demo plate,
// -open and/or -script), renders one frame with the pointer at -hover and
// writes it to the PNG path.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentmanager"
	"go-draw/doc"
	"go-draw/fileio"
	"go-draw/fileio/dxf"
	"go-draw/geom"
	"go-draw/internal/gpui"
	"go-draw/internal/gpui/ebitenplatform"
	"go-draw/ui"
)

// demoScript draws a mounting plate: outline, holes, a slot and centre
// lines, on three layers. Single spaces separate inputs; two in a row are
// Enter (AutoCAD script rules).
const demoScript = `LAYER outline=4 REC 0,0 220,130 PLINE 70,95 150,95 150,111 70,111 c ` +
	`LAYER holes=1 CIRCLE 25,25 8 CIRCLE 195,25 8 CIRCLE 195,105 8 CIRCLE 25,105 8 CIRCLE 110,50 20 ` +
	`ARC 80,50 110,80 140,50 ` +
	`LAYER center=3 LINE 110,10 110,90  LINE 70,50 150,50  LAYER outline`

func main() {
	png := flag.String("png", "", "render one frame headlessly to this PNG path and exit")
	width := flag.Int("width", 1280, "window width in logical pixels")
	height := flag.Int("height", 800, "window height in logical pixels")
	scale := flag.Float64("scale", 1, "device scale for -png")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	open := flag.String("open", "", "DXF file to open")
	script := flag.String("script", "", "command script to run after opening (AutoCAD script syntax)")
	demo := flag.Bool("demo", false, "draw the demo plate at startup (default in -png mode without -open/-script)")
	hover := flag.String("hover", "", "with -png: model point x,y to put the pointer on before rendering")
	agent := flag.String("agent", "", "agent harness to select in the agent panel: opencode, claude or codex (opens the panel)")
	agentCmd := flag.String("agent-cmd", "", "executable to run for the selected agent instead of the one on PATH")
	agentDemo := flag.Bool("agent-demo", false, "open the agent panel on a canned conversation (for -png)")
	saveOnExit := flag.String("save-on-exit", "", "dev: write the drawing to this DXF path when the window closes or on SIGINT/SIGTERM (GUI tests)")
	flag.Parse()

	files := &fileio.Registry{}
	files.AddReader(dxf.Codec{})
	files.AddWriter(dxf.Codec{})

	d := doc.New()
	if *open != "" {
		var warns []string
		var err error
		d, warns, err = files.Open(*open)
		if err != nil {
			log.Fatal(err)
		}
		for _, w := range warns {
			log.Printf("%s: %s", *open, w)
		}
	}
	ws := ui.NewWorkspace(d, files)
	defer ws.CloseAgent()
	if *agent != "" || *agentCmd != "" {
		h := agentmanager.Harness(*agent)
		switch h {
		case agentmanager.OpenCode, agentmanager.Claude, agentmanager.Codex:
		default:
			log.Fatalf("-agent %q: want opencode, claude or codex", *agent)
		}
		ws.AgentPanel().Configure(h, *agentCmd)
		ws.AgentPanel().Show()
	}
	if *agentDemo {
		ui.SeedAgentDemo(ws)
	}
	if *demo || (*png != "" && *open == "" && *script == "") {
		ws.Session().Script(demoScript)
	}
	if *script != "" {
		ws.Session().Script(*script)
	}
	if *png != "" || *open != "" || *demo {
		ws.FitOnFirstFrame()
	}

	if *png != "" {
		if err := renderPNG(ws, *png, *width, *height, float32(*scale), *hover); err != nil {
			log.Fatal(err)
		}
		return
	}

	p := ebitenplatform.New()
	err := p.Run(func(cx *gpui.App) {
		if *saveOnExit != "" {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			go func() { <-sig; cx.Quit() }()
		}
		cx.Keymap().AddBindings(ui.Bindings()...)
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "go-draw", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			log.Fatal(err)
		}
		if *smoke > 0 {
			time.AfterFunc(time.Duration(*smoke)*time.Millisecond, cx.Quit)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *saveOnExit != "" {
		if _, err := files.Save(*saveOnExit, ws.Session().Drawing()); err != nil {
			log.Fatal(err)
		}
	}
}

// renderPNG draws ws in a headless window, moves the pointer to the model
// point hover (if any) through the normal event path, and saves the frame.
func renderPNG(ws *ui.Workspace, path string, w, h int, scale float32, hover string) error {
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(scale)
	var hw *gpui.HeadlessWindow
	err := p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(ui.Bindings()...)
		opts := gpui.WindowOptions{Width: float32(w), Height: float32(h), Title: "go-draw"}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			log.Fatal(err)
		}
		hw = p.Windows()[0]
	})
	if err != nil {
		return err
	}
	hw.Pixels()
	if hover != "" {
		xs, ys, _ := strings.Cut(hover, ",")
		x, err1 := strconv.ParseFloat(xs, 64)
		y, err2 := strconv.ParseFloat(ys, 64)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("-hover %q: want x,y", hover)
		}
		hw.Dispatch(gpui.MouseMoveEvent{Position: ws.ScreenPoint(geom.V(x, y))})
	}
	return hw.SavePNG(path)
}
