package commands_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// TestACADCorpus runs the AutoCAD acceptance corpus in testdata/acad: each NAME.scr is fed to
// Session.Script on a fresh drawing and the result is compared with NAME.expect. The format is
// documented in testdata/acad/README.md.
//
// It is skipped unless GODRAW_ACAD_CORPUS is set:
//
//	report  log every script's outcome and never fail
//	1       fail when a script's outcome differs from the "go-draw:" status its .expect declares
//	        (so a newly passing script and a regression both need a deliberate edit)
//	strict  fail every script that does not pass
func TestACADCorpus(t *testing.T) {
	mode := os.Getenv("GODRAW_ACAD_CORPUS")
	if mode == "" {
		t.Skip("set GODRAW_ACAD_CORPUS=report|1|strict to run the AutoCAD acceptance corpus")
	}
	files, err := filepath.Glob(filepath.Join("testdata", "acad", "*.scr"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus scripts found: %v", err)
	}
	sort.Strings(files)
	passed := 0
	var rows []string
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".scr")
		if strings.HasPrefix(name, "_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			scr, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			exp, err := os.ReadFile(strings.TrimSuffix(f, ".scr") + ".expect")
			if err != nil {
				t.Fatal(err)
			}
			fails, declared := acadRun(string(scr), string(exp))
			got := "pass"
			if len(fails) > 0 {
				got = "fail"
			}
			if got == "pass" {
				passed++
			}
			rows = append(rows, fmt.Sprintf("%-4s (declared %-4s) %s", strings.ToUpper(got), declared, name))
			for _, m := range fails {
				t.Logf("%s: %s", name, m)
			}
			switch {
			case mode == "report":
			case mode == "strict" && got != "pass":
				t.Fail()
			case mode != "strict" && got != declared:
				t.Errorf("%s now %s but its .expect declares go-draw: %s; update the declaration", name, got, declared)
			}
		})
	}
	t.Logf("AutoCAD corpus: %d of %d scripts pass\n%s", passed, len(rows), strings.Join(rows, "\n"))
}

// acadRun feeds scr to a fresh session and checks it against exp. It returns the failures and the
// status the .expect file declares ("pass" when it declares none).
func acadRun(scr, exp string) (fails []string, declared string) {
	declared = "pass"
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	// The corpus assumes the preamble: running object snaps and ortho off.
	s.SnapOn, s.Ortho = false, false
	// The preamble also sets Standard TRIM and EXTEND (TRIMEXTENDMODE 0): edges first, Enter = all.
	_ = s.Vars().Set("TRIMEXTENDMODE", 0)
	start := s.HistoryTotal()
	s.Script(acadScriptText(scr))
	hist := s.History()
	if n := s.HistoryTotal() - start; n < len(hist) {
		hist = hist[len(hist)-n:]
	}

	ents := acadEntities(s.Drawing())
	for i, raw := range strings.Split(exp, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fail := func(format string, a ...any) {
			fails = append(fails, fmt.Sprintf("expect line %d (%s): %s", i+1, line, fmt.Sprintf(format, a...)))
		}
		verb, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch verb {
		case "go-draw:":
			declared = rest
		case "autocad:", "note:", "source:":
		case "count":
			acadCheckCount(ents, rest, fail)
		case "line", "circle", "arc", "lwpolyline":
			e, err := acadParseEntity(verb, rest)
			if err != nil {
				fail("%v", err)
				continue
			}
			found := false
			for j := range ents {
				if !ents[j].used && ents[j].matches(e) {
					ents[j].used = true
					found = true
					break
				}
			}
			if !found {
				fail("no matching entity")
			}
		case "layer":
			acadCheckLayer(s.Drawing(), rest, fail)
		case "current-layer":
			if got := s.Drawing().CurrentLayer; got != rest {
				fail("current layer = %q", got)
			}
		case "var":
			name, want, _ := strings.Cut(rest, " ")
			got, ok := acadVar(s, strings.ToUpper(name))
			switch {
			case !ok:
				fail("system variable %s is not implemented", name)
			case !acadValueEqual(got, strings.TrimSpace(want)):
				fail("%s = %s", name, got)
			}
		case "message":
			found := false
			for _, h := range hist {
				if strings.Contains(strings.ToLower(h), strings.ToLower(rest)) {
					found = true
					break
				}
			}
			if !found {
				fail("not in the command history")
			}
		case "selected":
			if n, _ := strconv.Atoi(rest); s.Selection().Len() != n {
				fail("%d objects selected", s.Selection().Len())
			}
		case "active":
			got := s.Active()
			if got == "" {
				got = "none"
			}
			if !strings.EqualFold(got, rest) {
				fail("active command = %s", got)
			}
		default:
			fail("unknown directive %q", verb)
		}
	}
	for _, e := range ents {
		if !e.used {
			fails = append(fails, "unexpected entity: "+e.String())
		}
	}
	if s.Active() != "" {
		fails = append(fails, "script ended inside "+s.Active()+" (prompt: "+s.Prompt()+")")
	}
	return fails, declared
}

// acadScriptText returns the script text Session.Script takes: the file as it is, because Script
// itself drops ";" comments and reads the final newline as the file's terminator.
func acadScriptText(scr string) string { return scr }

type acadEnt struct {
	class     string
	layer     string
	color     int
	pts       []geom.Vec2
	bulges    []float64
	r, a0, a1 float64
	closed    bool
	used      bool

	// Expectation-only attributes.
	wantLayer, wantColor bool
}

func (e acadEnt) String() string {
	var b strings.Builder
	b.WriteString(strings.ToLower(e.class))
	switch e.class {
	case "CIRCLE":
		fmt.Fprintf(&b, " %s r=%g", cmdsys.FormatPoint(e.pts[0]), e.r)
	case "ARC":
		fmt.Fprintf(&b, " %s r=%g %g..%g", cmdsys.FormatPoint(e.pts[0]), e.r, e.a0, e.a1)
	default:
		for i, p := range e.pts {
			fmt.Fprintf(&b, " %s", cmdsys.FormatPoint(p))
			if i < len(e.bulges) && e.bulges[i] != 0 {
				fmt.Fprintf(&b, "/%g", e.bulges[i])
			}
		}
		if e.closed {
			b.WriteString(" closed")
		}
	}
	fmt.Fprintf(&b, " layer=%s", e.layer)
	return b.String()
}

func acadEntities(d *doc.Drawing) []acadEnt {
	var out []acadEnt
	for _, h := range d.Handles() {
		e := d.Entity(h)
		c := e.Common()
		a := acadEnt{class: e.Class(), layer: c.Layer, color: c.Color}
		switch e := e.(type) {
		case *doc.Line:
			a.pts = []geom.Vec2{e.Start, e.End}
		case *doc.Circle:
			a.pts, a.r = []geom.Vec2{e.Center}, e.Radius
		case *doc.Arc:
			a.pts, a.r = []geom.Vec2{e.Center}, e.Radius
			a.a0, a.a1 = e.StartAngle*180/math.Pi, e.EndAngle*180/math.Pi
		case *doc.LWPolyline:
			a.pts, a.bulges, a.closed = e.Points, e.Bulges, e.Closed
		}
		out = append(out, a)
	}
	return out
}

const acadTol = 1e-6

func acadAngleEq(a, b float64) bool {
	d := math.Mod(a-b, 360)
	if d < 0 {
		d += 360
	}
	return d < acadTol || 360-d < acadTol
}

func (a acadEnt) bulge(i int) float64 {
	if i < len(a.bulges) {
		return a.bulges[i]
	}
	return 0
}

func (a acadEnt) matches(w acadEnt) bool {
	if a.class != w.class {
		return false
	}
	if w.wantLayer && a.layer != w.layer {
		return false
	}
	if w.wantColor && a.color != w.color {
		return false
	}
	switch a.class {
	case "LINE":
		return (a.pts[0].Near(w.pts[0], acadTol) && a.pts[1].Near(w.pts[1], acadTol)) ||
			(a.pts[0].Near(w.pts[1], acadTol) && a.pts[1].Near(w.pts[0], acadTol))
	case "CIRCLE":
		return a.pts[0].Near(w.pts[0], acadTol) && math.Abs(a.r-w.r) < acadTol
	case "ARC":
		return a.pts[0].Near(w.pts[0], acadTol) && math.Abs(a.r-w.r) < acadTol &&
			acadAngleEq(a.a0, w.a0) && acadAngleEq(a.a1, w.a1)
	case "LWPOLYLINE":
		return a.polyMatches(w)
	}
	return false
}

// polyMatches compares polylines up to the start vertex of a closed one and, when neither has
// bulges, up to direction.
func (a acadEnt) polyMatches(w acadEnt) bool {
	n := len(a.pts)
	if n != len(w.pts) || a.closed != w.closed {
		return false
	}
	flat := func(e acadEnt) bool {
		for _, b := range e.bulges {
			if b != 0 {
				return false
			}
		}
		return true
	}
	try := func(shift int, rev bool) bool {
		segs := n - 1
		if a.closed {
			segs = n
		}
		for i := 0; i < n; i++ {
			j := i
			if a.closed {
				j = (i + shift) % n
			}
			k := j
			if rev {
				k = n - 1 - j
			}
			if !a.pts[i].Near(w.pts[k], acadTol) {
				return false
			}
		}
		if !rev {
			for i := 0; i < segs; i++ {
				j := i
				if a.closed {
					j = (i + shift) % n
				}
				if math.Abs(a.bulge(i)-w.bulge(j)) > acadTol {
					return false
				}
			}
		}
		return true
	}
	shifts := 1
	if a.closed {
		shifts = n
	}
	for s := 0; s < shifts; s++ {
		if try(s, false) {
			return true
		}
		if flat(a) && flat(w) && try(s, true) {
			return true
		}
	}
	return false
}

func acadXY(s string) (geom.Vec2, error) {
	x, y, ok := strings.Cut(s, ",")
	if !ok {
		return geom.Vec2{}, fmt.Errorf("bad point %q", s)
	}
	fx, e1 := strconv.ParseFloat(x, 64)
	fy, e2 := strconv.ParseFloat(y, 64)
	if e1 != nil || e2 != nil {
		return geom.Vec2{}, fmt.Errorf("bad point %q", s)
	}
	return geom.V(fx, fy), nil
}

// acadParseEntity reads the arguments of an expected entity line, with its layer= and color=
// attributes.
func acadParseEntity(verb, rest string) (acadEnt, error) {
	e := acadEnt{class: strings.ToUpper(verb)}
	var args []string
	for _, f := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(f, "layer="):
			e.layer, e.wantLayer = strings.TrimPrefix(f, "layer="), true
		case strings.HasPrefix(f, "color="):
			v := strings.ToLower(strings.TrimPrefix(f, "color="))
			c := doc.ByLayer
			if v != "bylayer" {
				n, err := strconv.Atoi(v)
				if err != nil {
					return e, fmt.Errorf("bad colour %q", v)
				}
				c = n
			}
			e.color, e.wantColor = c, true
		default:
			args = append(args, f)
		}
	}
	num := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	switch verb {
	case "line":
		if len(args) != 2 {
			return e, fmt.Errorf("line wants two points")
		}
		for _, a := range args {
			p, err := acadXY(a)
			if err != nil {
				return e, err
			}
			e.pts = append(e.pts, p)
		}
	case "circle":
		if len(args) != 2 {
			return e, fmt.Errorf("circle wants centre and radius")
		}
		p, err := acadXY(args[0])
		if err != nil {
			return e, err
		}
		e.pts, e.r = []geom.Vec2{p}, num(args[1])
	case "arc":
		if len(args) != 4 {
			return e, fmt.Errorf("arc wants centre, radius, start and end angle")
		}
		p, err := acadXY(args[0])
		if err != nil {
			return e, err
		}
		e.pts, e.r, e.a0, e.a1 = []geom.Vec2{p}, num(args[1]), num(args[2]), num(args[3])
	case "lwpolyline":
		if len(args) < 3 || (args[0] != "open" && args[0] != "closed") {
			return e, fmt.Errorf("lwpolyline wants open|closed and vertices")
		}
		e.closed = args[0] == "closed"
		for _, a := range args[1:] {
			pt, b, _ := strings.Cut(a, "/")
			p, err := acadXY(pt)
			if err != nil {
				return e, err
			}
			e.pts = append(e.pts, p)
			e.bulges = append(e.bulges, num(b))
		}
	}
	return e, nil
}

func acadCheckCount(ents []acadEnt, rest string, fail func(string, ...any)) {
	f := strings.Fields(rest)
	var class string
	var want int
	switch len(f) {
	case 1:
		want, _ = strconv.Atoi(f[0])
	case 2:
		class = strings.ToUpper(f[0])
		want, _ = strconv.Atoi(f[1])
	default:
		fail("count wants [CLASS] N")
		return
	}
	got := 0
	for _, e := range ents {
		if class == "" || e.class == class {
			got++
		}
	}
	if got != want {
		fail("count = %d", got)
	}
}

func acadCheckLayer(d *doc.Drawing, rest string, fail func(string, ...any)) {
	f := strings.Fields(rest)
	if len(f) == 0 {
		fail("layer wants a name")
		return
	}
	l := d.Layer(f[0])
	if l == nil {
		fail("layer %s does not exist", f[0])
		return
	}
	for _, a := range f[1:] {
		switch {
		case strings.HasPrefix(a, "color="):
			if n, _ := strconv.Atoi(strings.TrimPrefix(a, "color=")); l.Color != n {
				fail("layer %s colour = %d", l.Name, l.Color)
			}
		case a == "on" && l.Off, a == "off" && !l.Off:
			fail("layer %s off = %v", l.Name, l.Off)
		case a == "locked" && !l.Locked, a == "unlocked" && l.Locked:
			fail("layer %s locked = %v", l.Name, l.Locked)
		}
	}
}

// acadVar reads the system variables the session can answer today; the rest report false.
func acadVar(s *cmdsys.Session, name string) (string, bool) {
	if name == "VIEWCTR" { // a point is "x,y,z" in AutoCAD
		return cmdsys.FormatPoint(s.View().Center) + ",0", true
	}
	if s.Vars().Has(name) {
		return s.Vars().String(name), true
	}
	return "", false
}

// acadValueEqual compares comma-separated numbers within the tolerance, other text exactly.
func acadValueEqual(got, want string) bool {
	g, w := strings.Split(got, ","), strings.Split(want, ",")
	if len(g) != len(w) {
		return false
	}
	for i := range g {
		a, e1 := strconv.ParseFloat(strings.TrimSpace(g[i]), 64)
		b, e2 := strconv.ParseFloat(strings.TrimSpace(w[i]), 64)
		if e1 == nil && e2 == nil {
			if math.Abs(a-b) > 1e-4 {
				return false
			}
		} else if !strings.EqualFold(strings.TrimSpace(g[i]), strings.TrimSpace(w[i])) {
			return false
		}
	}
	return true
}
