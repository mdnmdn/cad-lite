package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
)

func TestTextCommandLinesAreOneUndoStep(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\n10,10\n5\n30\nHello world\nSecond line\n\n")
	if s.Active() != "" {
		t.Fatalf("still active: %s\n%s", s.Active(), history(s))
	}
	ts := entities[*doc.Text](s)
	if len(ts) != 2 || ts[0].Value != "Hello world" || ts[1].Value != "Second line" {
		t.Fatalf("texts = %+v\n%s", ts, history(s))
	}
	a := ts[0]
	if a.Height != 5 || math.Abs(a.Rotation-math.Pi/6) > 1e-12 || a.Insert != geom.V(10, 10) || a.Style != doc.DefaultTextStyle {
		t.Fatalf("first = %+v", a)
	}
	// The next line starts 5/3 of the height below, perpendicular to the baseline.
	down := geom.V(math.Sin(math.Pi/6), -math.Cos(math.Pi/6)).Scale(5 * 5.0 / 3)
	near(t, "second line", ts[1].Insert, a.Insert.Add(down))
	if !strings.Contains(history(s), `Current text style:  "Standard"  Text height:  2.5000`) {
		t.Fatalf("no header:\n%s", history(s))
	}
	if s.Vars().Real("TEXTSIZE") != 5 {
		t.Fatal("TEXTSIZE not remembered")
	}
	s.Submit("U")
	if s.Drawing().Len() != 0 {
		t.Fatalf("after U: %d entities", s.Drawing().Len())
	}
	// Defaults: TEXTSIZE and the last rotation are offered again.
	s.Script("DT\n0,0\n\n\nA\n\n")
	ts = entities[*doc.Text](s)
	if len(ts) != 1 || ts[0].Height != 5 || math.Abs(ts[0].Rotation-math.Pi/6) > 1e-12 {
		t.Fatalf("defaults: %+v", ts)
	}
	// Enter at the first prompt continues under the last text.
	s.Script("DTEXT\n\nB\n\n")
	ts = entities[*doc.Text](s)
	if len(ts) != 2 || ts[1].Value != "B" {
		t.Fatalf("continue: %+v\n%s", ts, history(s))
	}
	near(t, "continued", ts[1].Insert, ts[0].Insert.Add(down))
}

func TestTextJustifyOptions(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\nJ\nTR\n50,50\n2\n0\nTop right\n\n")
	tr := entities[*doc.Text](s)[0]
	if tr.Justify() != "TR" || tr.Align != geom.V(50, 50) {
		t.Fatalf("TR = %+v\n%s", tr, history(s))
	}
	near(t, "TR corner", tr.Layout().Corners[2], geom.V(50, 50))
	if !strings.Contains(history(s), "Specify top-right point of text") {
		t.Fatalf("prompt:\n%s", history(s))
	}
	// The justification is remembered (TEXTJUSTIFY).
	s.Script("TEXT\n0,0\n2\n0\nAgain\n\n")
	if j := entities[*doc.Text](s)[1].Justify(); j != "TR" {
		t.Fatalf("remembered justification = %s", j)
	}
	// Align takes two points and no height or rotation; Fit asks the height.
	s.Script("TEXT\nJ\nA\n0,-20\n40,-20\nAligned\n\n")
	s.Script("TEXT\nJ\nF\n0,-40\n40,-30\n3\nFitted\n\n")
	ts := entities[*doc.Text](s)
	al, fit := ts[2], ts[3]
	if al.HAlign != doc.TextAligned || math.Abs(al.Layout().Width-40) > 1e-9 {
		t.Fatalf("aligned = %+v", al.Layout())
	}
	if fit.HAlign != doc.TextFit || fit.Height != 3 || math.Abs(fit.Layout().Width-math.Hypot(40, 10)) > 1e-9 {
		t.Fatalf("fit = %+v", fit)
	}
	// Middle and Center use their own prompts.
	s.Script("TEXT\nJ\nM\n5,5\n1\n0\nMid\n\n")
	m := entities[*doc.Text](s)[4]
	if m.HAlign != doc.TextMiddle || m.Anchor() != geom.V(5, 5) {
		t.Fatalf("middle = %+v", m)
	}
}

func TestTextStyleOption(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\nS\nBogus\nS\n?\nstandard\n0,0\n1\n0\nx\n\n")
	if !strings.Contains(history(s), `Cannot find text style "Bogus".`) || !strings.Contains(history(s), `Style name: "Standard"`) {
		t.Fatalf("history:\n%s", history(s))
	}
	if ts := entities[*doc.Text](s); len(ts) != 1 || ts[0].Style != "Standard" {
		t.Fatalf("texts = %+v", ts)
	}
}

func TestTextContentIsLiteral(t *testing.T) {
	s := newSession(t)
	// Typed snaps, selection words, a leading quote and numbers are text at the text prompt.
	s.Script("TEXT\n0,0\n1\n0\nEND\nALL\n'ZOOM\n12,5\n\n")
	var got []string
	for _, e := range entities[*doc.Text](s) {
		got = append(got, e.Value)
	}
	if strings.Join(got, "|") != "END|ALL|'ZOOM|12,5" {
		t.Fatalf("values = %q\n%s", got, history(s))
	}
}

func TestTextEditsAndInquiry(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\n0,0\n2\n0\nLabel\n\n")
	selectAll(s)
	s.Script("MOVE\n0,0\n10,5\n")
	if tx := entities[*doc.Text](s)[0]; tx.Insert != geom.V(10, 5) {
		t.Fatalf("moved = %+v", tx)
	}
	selectAll(s)
	s.Script("ROTATE\n10,5\n90\n")
	if tx := entities[*doc.Text](s)[0]; math.Abs(tx.Rotation-math.Pi/2) > 1e-12 {
		t.Fatalf("rotated = %+v", tx)
	}
	selectAll(s)
	s.Script("SCALE\n10,5\n2\n")
	if tx := entities[*doc.Text](s)[0]; tx.Height != 4 {
		t.Fatalf("scaled = %+v", tx)
	}
	selectAll(s)
	s.Script("COPY\n0,0\n100,0\n\n")
	if n := len(entities[*doc.Text](s)); n != 2 {
		t.Fatalf("%d texts after COPY", n)
	}
	s.Selection().Clear()
	s.Script("LIST\n110,5\n\n")
	if h := history(s); !strings.Contains(h, "TEXT  Layer:") || !strings.Contains(h, "text Label") || !strings.Contains(h, "height 4") {
		t.Fatalf("LIST:\n%s", h)
	}
	selectAll(s)
	s.Script("ERASE\n")
	if s.Drawing().Len() != 0 {
		t.Fatal("not erased")
	}
}

func TestTextMirrorHonoursMirrtext(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\n0,0\n2\n0\nMIRROR\n\n")
	selectAll(s)
	s.Script("MIRROR\n10,0\n10,10\nN\n")
	ts := entities[*doc.Text](s)
	if len(ts) != 2 || ts[1].Generation != 0 || ts[1].Justify() != "Right" || !ts[1].Anchor().Near(geom.V(20, 0), 1e-9) {
		t.Fatalf("MIRRTEXT 0: %+v\n%s", ts[1], history(s))
	}
	s.Script("MIRRTEXT\n1\n")
	s.Selection().Clear()
	s.Selection().Add(ts[0].Handle())
	s.Script("MIRROR\n10,0\n10,10\nN\n")
	ts = entities[*doc.Text](s)
	if len(ts) != 3 || ts[2].Generation != doc.TextUpsideDown {
		t.Fatalf("MIRRTEXT 1: %+v", ts[2])
	}
}

func TestTextPickAndInsertionSnap(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\nJ\nMC\n50,50\n10\n0\nWIDE TEXT\n\n")
	d, ix := s.Drawing(), s.Index()
	tx := entities[*doc.Text](s)[0]
	// Inside the box, away from any stroke, picks the text.
	if h, ok := cmdsys.Pick(d, ix, geom.V(50, 50.2), 0.01); !ok || h != tx.Handle() {
		t.Fatalf("pick = %v %v", h, ok)
	}
	// Only the insertion point snaps, and only with INS on.
	if _, ok := cmdsys.FindSnap(d, ix, geom.V(49, 51), 3, cmdsys.DefaultSnaps); ok {
		t.Fatal("text snapped without INS")
	}
	hit, ok := cmdsys.FindSnap(d, ix, geom.V(49, 51), 3, cmdsys.DefaultSnaps|cmdsys.SnapInsertion)
	if !ok || hit.Mode != cmdsys.SnapInsertion || hit.Point != geom.V(50, 50) {
		t.Fatalf("snap = %+v %v", hit, ok)
	}
	// A typed INS override snaps too.
	s.Script("LINE\nINS\n")
	s.Click(geom.V(49, 51), false)
	s.Script("0,0\n\n")
	if l := entities[*doc.Line](s); len(l) != 1 || l[0].Start != geom.V(50, 50) {
		t.Fatalf("lines = %+v\n%s", l, history(s))
	}
}

// TestTextKeywordGroups proves the TEXT prompts' keywords free of shortcut collisions in every
// language (the groups are checked here until i18n.KeywordGroups lists them).
func TestTextKeywordGroups(t *testing.T) {
	groups := map[string][]string{
		"TEXT start point": {"Justify", "Style"},
		"TEXT justify":     {"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"},
		"TEXT style":       {"?"},
	}
	for g, ids := range groups {
		if _, had := i18n.KeywordGroups[g]; !had {
			i18n.KeywordGroups[g] = ids
			defer delete(i18n.KeywordGroups, g)
		}
	}
	if p := i18n.Verify(); len(p) > 0 {
		t.Fatalf("i18n problems: %q", p)
	}
}

func TestMTextCommand(t *testing.T) {
	s := newSession(t)
	s.Script("MTEXT\n0,0\nH\n2\nJ\nMC\n40,-20\nLiving room\nArea 24 m2 with a long description that wraps\n\n")
	ms := entities[*doc.MText](s)
	if len(ms) != 1 || s.Active() != "" {
		t.Fatalf("mtexts = %+v\n%s", ms, history(s))
	}
	m := ms[0]
	if m.Value != `Living room\PArea 24 m2 with a long description that wraps` || m.Width != 40 || m.Height != 2 || m.Attachment != doc.MTextMiddleCenter {
		t.Fatalf("mtext = %+v", m)
	}
	near(t, "insertion", m.Insert, geom.V(20, -10))
	if len(m.Layout().Lines) < 3 {
		t.Fatalf("lines = %q", m.Layout().Lines)
	}
	s.Selection().Clear()
	s.Script("LIST\n20,-10\n\n")
	if h := history(s); !strings.Contains(h, "MTEXT  Layer:") || !strings.Contains(h, "contents Living room / Area 24") || !strings.Contains(h, "attachment MC") {
		t.Fatalf("LIST:\n%s", h)
	}
	selectAll(s)
	s.Script("MIRROR\n0,0\n0,10\nY\n")
	m = entities[*doc.MText](s)[0]
	if m.Attachment != doc.MTextMiddleCenter || !m.Insert.Near(geom.V(-20, -10), 1e-9) || math.Abs(math.Cos(m.Rotation)-1) > 1e-12 {
		t.Fatalf("mirrored = %+v", m)
	}
	s.Script("T\n0,0\nW\n0\nx\n\n")
	if n := len(entities[*doc.MText](s)); n != 2 {
		t.Fatalf("%d mtexts after T", n)
	}
	groups := map[string][]string{
		"MTEXT corner":  {"Height", "Justify", "Rotation", "Style", "Width"},
		"MTEXT justify": {"TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"},
	}
	for g, ids := range groups {
		i18n.KeywordGroups[g] = ids
		defer delete(i18n.KeywordGroups, g)
	}
	if p := i18n.Verify(); len(p) > 0 {
		t.Fatalf("i18n problems: %q", p)
	}
}

func TestMTextCommandEscapesTypedText(t *testing.T) {
	s := newSession(t)
	s.Script("MTEXT\n0,0\nW\n0\nPath C:\\data\\Pictures\nSet {a, b}\n\n")
	ms := entities[*doc.MText](s)
	if len(ms) != 1 {
		t.Fatalf("mtexts = %+v\n%s", ms, history(s))
	}
	if got := doc.MTextPlain(ms[0].Value); got != "Path C:\\data\\Pictures\nSet {a, b}" {
		t.Fatalf("plain = %q (value %q)", got, ms[0].Value)
	}
}

// TestTextSnapAndPickReachTheWholeBox: the spatial index finds a text by its box and anchor, not
// only its ink, so the insertion snap of bottom-justified text and a click in an MTEXT box's
// empty part work.
func TestTextSnapAndPickReachTheWholeBox(t *testing.T) {
	s := newSession(t)
	bl := doc.NewText(geom.V(0, 0), 100, 0, "ABC")
	bl.HAlign, bl.VAlign, bl.Align = doc.TextLeft, doc.TextBottom, geom.V(0, 0)
	m := doc.NewMText(geom.V(500, 0), 10, 200, "short")
	addEntities(t, s, bl, m)
	d, ix := s.Drawing(), s.Index()
	hit, ok := cmdsys.FindSnap(d, ix, geom.V(1, 1), 3, cmdsys.SnapInsertion)
	if !ok || hit.Point != geom.V(0, 0) {
		t.Fatalf("BL insertion snap = %+v %v", hit, ok)
	}
	if h, ok := cmdsys.Pick(d, ix, geom.V(680, -5), 1); !ok || h != m.Handle() {
		t.Fatalf("pick in the empty part of the MTEXT box = %v %v", h, ok)
	}
}

// TestPickPrefersANearStrokeToATextBox: a line 2 units from the cursor wins over the text whose
// box the cursor is in.
func TestPickPrefersANearStrokeToATextBox(t *testing.T) {
	s := newSession(t)
	txt := doc.NewText(geom.V(0, 0), 100, 0, "OO")
	ln := &doc.Line{Start: geom.V(10, 52), End: geom.V(60, 52)}
	addEntities(t, s, txt, ln)
	if h, ok := cmdsys.Pick(s.Drawing(), s.Index(), geom.V(30, 50), 5); !ok || h != ln.Handle() {
		t.Fatalf("pick = %v %v, want the line %v", h, ok, ln.Handle())
	}
	if h, ok := cmdsys.Pick(s.Drawing(), s.Index(), geom.V(30, 20), 5); !ok || h != txt.Handle() {
		t.Fatalf("pick = %v %v, want the text", h, ok)
	}
}

func TestTextKeepsTypedSpaces(t *testing.T) {
	s := newSession(t)
	s.Script("TEXT\n0,0\n1\n0\n  two  spaces \n   \n")
	ts := entities[*doc.Text](s)
	if len(ts) != 1 || ts[0].Value != "  two  spaces " || s.Active() != "" {
		t.Fatalf("texts = %+v (active %q)\n%s", ts, s.Active(), history(s))
	}
}
