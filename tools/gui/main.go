// Command gui runs go-draw's real interactive window under Xvfb with software
// GL, drives it with xdotool from a scenario file, and captures screenshots.
//
//	go run ./tools/gui [-out DIR] [-video] [-bin PATH] scenario.gui...
//
// It needs Xvfb, xdotool, ImageMagick's import and (for -video) ffmpeg. The
// scenario language is described in README.md. The exit status is non-zero if
// any scenario fails.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	out := flag.String("out", "_tmp/gui/out", "output directory; one subdirectory per scenario")
	video := flag.Bool("video", false, "also record the whole session to session.mp4")
	bin := flag.String("bin", "", "prebuilt cmd/cad binary (default: build it into _tmp/gui/cad)")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: gui [-out DIR] [-video] [-bin PATH] scenario.gui...")
		os.Exit(2)
	}
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := *bin
	if exe == "" {
		if exe, err = buildApp(root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	failed := 0
	for _, path := range flag.Args() {
		name := strings.TrimSuffix(filepath.Base(path), ".gui")
		if err := RunFile(path, exe, filepath.Join(*out, name), *video); err != nil {
			fmt.Printf("FAIL %s: %v\n", name, err)
			failed++
		} else {
			fmt.Printf("ok   %s\n", name)
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}
