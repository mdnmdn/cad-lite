package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// repoRoot finds the directory holding go.mod, searching upward from the cwd.
func repoRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		if p := filepath.Dir(d); p != d {
			d = p
			continue
		}
		return "", errors.New("go.mod not found above the working directory")
	}
}

// buildApp compiles cmd/cad into _tmp/gui/cad and returns its path.
func buildApp(root string) (string, error) {
	exe := filepath.Join(root, "_tmp", "gui", "cad")
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/cad")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ./cmd/cad: %v\n%s", err, b)
	}
	return exe, nil
}

// RunFile parses the scenario at path and runs it with the app binary exe,
// writing screenshots, app.log, drawing.dxf and optionally session.mp4 to out.
func RunFile(path, exe, out string, video bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc, err := ParseScenario(f)
	if err != nil {
		return err
	}
	return Run(sc, exe, out, video)
}

type runner struct {
	sc      *Scenario
	out     string
	display string
	win     string
	env     []string
	app     *exec.Cmd
	exited  chan error
	quit    bool
}

// Run starts Xvfb, launches exe with the scenario's flags, waits for the
// window, runs the steps, quits the app, checks the expect-count assertions
// against the saved drawing and tears everything down.
func Run(sc *Scenario, exe, out string, video bool) (err error) {
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, tool := range []string{"Xvfb", "xdotool", "import"} {
		if _, e := exec.LookPath(tool); e != nil {
			return fmt.Errorf("%s not found (apt-get install xvfb xdotool imagemagick)", tool)
		}
	}
	r := &runner{sc: sc, out: out}

	xvfb, display, err := startXvfb(sc.Width, sc.Height)
	if err != nil {
		return err
	}
	defer func() { xvfb.Process.Kill(); xvfb.Wait() }()
	r.display = display
	r.env = append(os.Environ(), "DISPLAY="+display, "LIBGL_ALWAYS_SOFTWARE=1")

	var rec *exec.Cmd
	if video {
		rec = exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "x11grab",
			"-video_size", fmt.Sprintf("%dx%d", sc.Width, sc.Height), "-framerate", "15",
			"-i", display, "-c:v", "libx264", "-pix_fmt", "yuv420p", filepath.Join(out, "session.mp4"))
		stdin, _ := rec.StdinPipe()
		rec.Env = r.env
		if e := rec.Start(); e != nil {
			return e
		}
		defer func() {
			stdin.Write([]byte("q"))
			stdin.Close()
			rec.Wait()
		}()
	}

	logf, err := os.Create(filepath.Join(out, "app.log"))
	if err != nil {
		return err
	}
	defer logf.Close()
	dxfPath := filepath.Join(out, "drawing.dxf")
	args := append(append([]string{}, sc.AppFlags...), "-save-on-exit", dxfPath)
	r.app = exec.Command(exe, args...)
	r.app.Env = r.env
	r.app.Stdout, r.app.Stderr = logf, logf
	if err := r.app.Start(); err != nil {
		return err
	}
	r.exited = make(chan error, 1)
	go func() { r.exited <- r.app.Wait() }()
	defer func() {
		if !r.quit {
			r.app.Process.Kill()
			<-r.exited
		}
	}()

	if err := r.waitWindow(30 * time.Second); err != nil {
		return err
	}
	time.Sleep(800 * time.Millisecond) // first frame
	var expects []Step
	for _, s := range sc.Steps {
		if s.Op == "expect-count" {
			expects = append(expects, s)
			continue
		}
		if e := r.step(s); e != nil {
			return fmt.Errorf("line %d (%s %s): %w%s", s.Line, s.Op, strings.Join(s.Args, " "), e, r.logTail())
		}
	}
	if err := r.quitApp(); err != nil {
		return err
	}
	if len(expects) == 0 {
		return nil
	}
	b, err := os.ReadFile(dxfPath)
	if err != nil {
		return fmt.Errorf("drawing not saved: %w", err)
	}
	counts := CountDXF(string(b))
	for _, s := range expects {
		want, e := strconv.Atoi(s.Args[1])
		if e != nil {
			return fmt.Errorf("line %d: bad count %q", s.Line, s.Args[1])
		}
		if got := counts[strings.ToUpper(s.Args[0])]; got != want {
			return fmt.Errorf("line %d: expect-count %s: got %d, want %d (all: %v)", s.Line, s.Args[0], got, want, counts)
		}
	}
	return nil
}

// startXvfb starts Xvfb on a free display (it picks the number) and returns
// it as ":N".
func startXvfb(w, h int) (*exec.Cmd, string, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, "", err
	}
	cmd := exec.Command("Xvfb", "-displayfd", "3", "-screen", "0", fmt.Sprintf("%dx%dx24", w, h), "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{pw}
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	pw.Close()
	line := make(chan string, 1)
	go func() { s, _ := bufio.NewReader(pr).ReadString('\n'); line <- strings.TrimSpace(s) }()
	select {
	case s := <-line:
		pr.Close()
		if s == "" {
			cmd.Process.Kill()
			cmd.Wait()
			return nil, "", errors.New("xvfb exited before reporting a display")
		}
		return cmd, ":" + s, nil
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return nil, "", errors.New("xvfb did not start")
	}
}

// sh runs a helper command on the virtual display and returns its output.
func (r *runner) sh(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.env
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(buf.String()))
	}
	return strings.TrimSpace(buf.String()), nil
}

func (r *runner) waitWindow(d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case err := <-r.exited:
			r.quit = true
			return fmt.Errorf("app exited before its window appeared: %v%s", err, r.logTail())
		default:
		}
		if s, err := r.sh(5*time.Second, "xdotool", "search", "--onlyvisible", "--name", "^Cad-lite$"); err == nil && s != "" {
			r.win = strings.Fields(s)[0]
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("window not mapped after %v%s", d, r.logTail())
}

func (r *runner) logTail() string {
	b, _ := os.ReadFile(filepath.Join(r.out, "app.log"))
	if len(b) == 0 {
		return ""
	}
	if len(b) > 1500 {
		b = b[len(b)-1500:]
	}
	return "\napp.log:\n" + string(b)
}

func (r *runner) xdo(args ...string) error {
	_, err := r.sh(15*time.Second, "xdotool", args...)
	return err
}

// move positions the pointer at window-relative logical pixels.
func (r *runner) move(x, y string) error {
	return r.xdo("mousemove", "--window", r.win, x, y)
}

var buttons = map[string]string{"left": "1", "middle": "2", "right": "3"}

// btnXY parses "[button] x y" arguments.
func btnXY(a []string) (btn, x, y string, rest []string, err error) {
	btn = "1"
	if len(a) > 0 {
		if b, ok := buttons[a[0]]; ok {
			btn, a = b, a[1:]
		}
	}
	if len(a) < 2 {
		return "", "", "", nil, errors.New("want [left|middle|right] x y")
	}
	return btn, a[0], a[1], a[2:], nil
}

func (r *runner) step(s Step) error {
	a := s.Args
	switch s.Op {
	case "move":
		if err := r.move(a[0], a[1]); err != nil {
			return err
		}
	case "click", "dclick":
		btn, x, y, _, err := btnXY(a)
		if err != nil {
			return err
		}
		if err := r.move(x, y); err != nil {
			return err
		}
		time.Sleep(80 * time.Millisecond)
		reps := "1"
		if s.Op == "dclick" {
			reps = "2"
		}
		if err := r.xdo("click", "--repeat", reps, "--delay", "60", btn); err != nil {
			return err
		}
	case "drag":
		// drag x1 y1 x2 y2 [button]: press, move in steps, release.
		if len(a) < 4 {
			return errors.New("want x1 y1 x2 y2 [button]")
		}
		btn := "1"
		if len(a) == 5 {
			b, ok := buttons[a[4]]
			if !ok {
				return fmt.Errorf("bad button %q", a[4])
			}
			btn = b
		}
		x1, y1, x2, y2 := atoi(a[0]), atoi(a[1]), atoi(a[2]), atoi(a[3])
		if err := r.move(a[0], a[1]); err != nil {
			return err
		}
		time.Sleep(80 * time.Millisecond)
		if err := r.xdo("mousedown", btn); err != nil {
			return err
		}
		const n = 8
		for i := 1; i <= n; i++ {
			time.Sleep(40 * time.Millisecond)
			if err := r.move(strconv.Itoa(x1+(x2-x1)*i/n), strconv.Itoa(y1+(y2-y1)*i/n)); err != nil {
				r.xdo("mouseup", btn)
				return err
			}
		}
		time.Sleep(80 * time.Millisecond)
		if err := r.xdo("mouseup", btn); err != nil {
			return err
		}
	case "key":
		for _, k := range a {
			if err := r.pressKey(k); err != nil {
				return err
			}
		}
	case "type":
		for i, part := range strings.Split(a[0], "\n") {
			if i > 0 {
				if err := r.pressKey("Return"); err != nil {
					return err
				}
			}
			if part == "" {
				continue
			}
			if err := r.xdo("type", "--delay", "60", "--", part); err != nil {
				return err
			}
		}
	case "wheel", "hwheel":
		// wheel dy [x y]: positive scrolls down (zoom out), negative up.
		// hwheel dx [x y]: positive scrolls right, negative left.
		if len(a) == 3 {
			if err := r.move(a[1], a[2]); err != nil {
				return err
			}
			time.Sleep(80 * time.Millisecond)
		}
		n := atoi(a[0])
		down, up := "5", "4"
		if s.Op == "hwheel" {
			down, up = "7", "6"
		}
		btn := up
		if n > 0 {
			btn = down
		} else {
			n = -n
		}
		if err := r.xdo("click", "--repeat", strconv.Itoa(n), "--delay", "80", btn); err != nil {
			return err
		}
	case "hold", "release":
		// hold MOD / release MOD: keep a modifier (ctrl, shift, alt, super) down
		// across other steps, such as a wheel.
		cmd := "keydown"
		if s.Op == "release" {
			cmd = "keyup"
		}
		if err := r.xdo(cmd, a[0]); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	case "wait":
		ms, err := strconv.Atoi(a[0])
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	case "shot":
		time.Sleep(250 * time.Millisecond) // let the frame settle
		_, err := r.sh(30*time.Second, "import", "-window", r.win, r.shotPath(a[0]))
		return err
	case "expect-diff", "expect-same":
		same, err := samePNG(r.shotPath(a[0]), r.shotPath(a[1]))
		if err != nil {
			return err
		}
		if s.Op == "expect-diff" && same {
			return fmt.Errorf("%s and %s are identical", a[0], a[1])
		}
		if s.Op == "expect-same" && !same {
			return fmt.Errorf("%s and %s differ", a[0], a[1])
		}
		return nil
	case "quit":
		return r.quitApp()
	}
	time.Sleep(150 * time.Millisecond) // let the app handle the input
	return nil
}

// pressKey holds a key combo (e.g. ctrl+shift+s) down for several frames. A
// press shorter than one ebiten tick can be missed entirely, because ebiten
// polls key state once per tick; xdotool key releases immediately. The hold
// is long because a CPU-rasterised frame can take well over 100 ms on a busy
// runner (the app's startup probes run alongside), and a press and release
// inside one slow frame is lost.
func (r *runner) pressKey(k string) error {
	if err := r.xdo("keydown", "--delay", "30", k); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return r.xdo("keyup", "--delay", "30", k)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func (r *runner) shotPath(name string) string {
	return filepath.Join(r.out, strings.TrimSuffix(name, ".png")+".png")
}

func samePNG(a, b string) (bool, error) {
	load := func(p string) (*image.NRGBA, error) {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		im, err := png.Decode(f)
		if err != nil {
			return nil, err
		}
		n := image.NewNRGBA(im.Bounds())
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				n.Set(x, y, im.At(x, y))
			}
		}
		return n, nil
	}
	ia, err := load(a)
	if err != nil {
		return false, err
	}
	ib, err := load(b)
	if err != nil {
		return false, err
	}
	return ia.Bounds() == ib.Bounds() && bytes.Equal(ia.Pix, ib.Pix), nil
}

// quitApp asks the app to quit with SIGTERM (cmd/cad's -save-on-exit turns
// that into a normal quit that saves) and waits for a clean exit.
func (r *runner) quitApp() error {
	if r.quit {
		return nil
	}
	r.quit = true
	r.app.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-r.exited:
		if err != nil {
			return fmt.Errorf("app exit: %v%s", err, r.logTail())
		}
		return nil
	case <-time.After(10 * time.Second):
		r.app.Process.Kill()
		<-r.exited
		return errors.New("app did not quit within 10s")
	}
}
