// Command cad is the Cad-lite desktop application: an AutoCAD-style 2D
// drafting editor built on the gpui library and shown in an ebiten window.
//
// Files named after the flags (cad [flags] [file ...], as Windows and Linux pass a file opened with
// the application) are opened at startup as files dropped on the window are.
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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentmanager"
	"cad-lite/appenv"
	"cad-lite/assets/appicon"
	"cad-lite/doc"
	"cad-lite/filedialog"
	"cad-lite/fileio"
	"cad-lite/fileio/dwg"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/internal/gpui/ebitenplatform"
	"cad-lite/ui"
)

// demoScript draws a mounting plate: outline, holes, a slot and centre
// lines, on three layers. Single spaces separate inputs; two in a row are
// Enter (AutoCAD script rules).
const demoScript = `LAYER outline=4 REC 0,0 220,130 PLINE 70,95 150,95 150,111 70,111 c ` +
	`LAYER holes=1 CIRCLE 25,25 8 CIRCLE 195,25 8 CIRCLE 195,105 8 CIRCLE 25,105 8 CIRCLE 110,50 20 ` +
	`ARC 80,50 110,80 140,50 ` +
	`LAYER center=3 LINE 110,10 110,90  LINE 70,50 150,50  LAYER outline`

// version is the release version; packaging sets it with -ldflags "-X main.version=...".
var version = "dev"

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
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [file ...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	// ExitOnError: a bad flag exits with status 2 inside Parse.
	_ = flag.CommandLine.Parse(launchArgs(os.Args[1:]))
	if *showVersion {
		fmt.Println("cad-lite", version)
		return
	}
	args, argErr := fileArgs(*open, flag.Args())
	if argErr != nil {
		log.Fatal(argErr)
	}

	// The system language until the settings file is read (-png renders keep it: LANG=it_IT.UTF-8).
	i18n.Set(i18n.Detect())

	files := &fileio.Registry{}
	files.AddReader(dxf.Codec{})
	files.AddReader(dwg.Codec{})
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
	ws.SetVersion(version)
	// Deferred first, so it runs last: after the settings are saved and the agent is closed,
	// start the version an in-app update installed.
	defer func() {
		if err := ws.RelaunchIfPending(); err != nil {
			log.Printf("update: restart: %v", err)
		}
	}()
	defer ws.CloseAgent()
	// fatal stops the agent subprocess first: log.Fatal and os.Exit skip deferred calls.
	fatal := func(v ...any) { ws.CloseAgent(); log.Fatal(v...) }
	if *open != "" {
		ws.SetProjectPath(*open)
	}
	if *png == "" {
		ws.EnableUpdates()
		ws.EnableAgentDiscovery()
		ws.SetFileDialog(filedialog.System{})
		if err := ws.LoadSettings(); err != nil {
			log.Printf("settings: %v", err)
		}
		i18n.Set(i18n.FromSetting(ws.Settings().Language))
		// A language change in the settings panel switches the catalogs from then on.
		ws.SetLanguageHandler(func(lang string) { i18n.Set(i18n.FromSetting(lang)) })
		set := map[string]bool{}
		flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if win := ws.Settings().Window; win.Width > 0 && win.Height > 0 {
			if !set["width"] {
				*width = win.Width
			}
			if !set["height"] {
				*height = win.Height
			}
		}
	}
	if len(args) > 0 {
		ws.OpenPaths(args)
	}
	ws.Brand = brandFromEnv()
	if *png != "" {
		// CADLITE_THEME picks the theme of a -png render, which reads no settings file.
		if t, ok := ui.ThemeByName(appenv.Get("THEME")); ok {
			ui.ApplyTheme(t)
		}
	}
	if *agent != "" || *agentCmd != "" {
		h := agentmanager.Harness(*agent)
		switch h {
		case agentmanager.OpenCode, agentmanager.Claude, agentmanager.Codex:
		default:
			fatal(fmt.Sprintf("-agent %q: want opencode, claude or codex", *agent))
		}
		ws.AgentPanel().Configure(h, *agentCmd)
		ws.AgentPanel().Show()
	}
	if *agentDemo {
		ui.SeedAgentDemo(ws)
	}
	if *demo || (*png != "" && *open == "" && len(args) == 0 && *script == "") {
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
			fatal(err)
		}
		return
	}

	p := ebitenplatform.New(ebitenplatform.WithWindowIcon(appicon.Images()))
	err := p.Run(func(cx *gpui.App) {
		if *saveOnExit != "" {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			go func() { <-sig; cx.Quit() }()
		}
		ws.Session().SetQuitHandler(cx.Quit)
		cx.Keymap().AddBindings(ui.Bindings()...)
		// Files opened from Finder or dropped on the Dock icon (macOS).
		cx.OnOpenPaths(func(paths []string, _ *gpui.App) { ws.OpenPaths(paths) })
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Cad-lite", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			fatal(err)
		}
		if *smoke > 0 {
			time.AfterFunc(time.Duration(*smoke)*time.Millisecond, cx.Quit)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		ws.CloseAgent()
		os.Exit(1)
	}
	if err := ws.SaveSettings(); err != nil {
		log.Printf("settings: %v", err)
	}
	if *saveOnExit != "" {
		if _, err := files.Save(*saveOnExit, ws.Session().Drawing()); err != nil {
			fatal(err)
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
		ws.Session().SetQuitHandler(cx.Quit)
		cx.Keymap().AddBindings(ui.Bindings()...)
		opts := gpui.WindowOptions{Width: float32(w), Height: float32(h), Title: "Cad-lite"}
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

// brandFromEnv picks the toolbar brand: CADLITE_BRAND=mac|win|text forces one (for -png renders),
// otherwise it follows the OS.
func brandFromEnv() ui.Brand {
	if b, ok := ui.ParseBrand(appenv.Get("BRAND")); ok {
		return b
	}
	return ui.BrandForOS(runtime.GOOS)
}

// launchArgs drops the process serial number argument (-psn_0_1234) that macOS LaunchServices
// passes to an application it starts on some systems; the flag package would reject it.
func launchArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if !strings.HasPrefix(a, "-psn_") {
			out = append(out, a)
		}
	}
	return out
}

// fileArgs checks the files named after the flags. -open names the drawing a script or a GUI test
// works on and stops on a bad file; a file argument is opened as a dropped file is, reporting
// problems in the command history. The two do not mix.
func fileArgs(open string, args []string) ([]string, error) {
	if open != "" && len(args) > 0 {
		return nil, fmt.Errorf("-open %s and file arguments %q: give one or the other", open, args)
	}
	return args, nil
}
