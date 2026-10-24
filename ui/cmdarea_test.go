package ui

import (
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// cmdAreaRig is a workspace with a few lines of history, the last one far too long for the
// command area, in a 1280x800 window.
func cmdAreaRig(t *testing.T) *sHarness {
	t.Helper()
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	ws.Session().Script("LINE 0,0 100,0 100,50 C")
	ws.Session().Say("ui.file.unsupported", strings.Repeat("very-long-folder-name/", 12)+"plate.xyz", ws.readFormats())
	h.frame()
	return h
}

// TestCommandAreaRenders writes the command area expanded and folded to _tmp/cmdline for a look.
func TestCommandAreaRenders(t *testing.T) {
	h := cmdAreaRig(t)
	h.savePNG("cmdline", "area-expanded.png")
	h.clickID("cmd-collapse")
	h.frame()
	h.savePNG("cmdline", "area-folded.png")
	h.clickID("cmd-recent")
	h.frame()
	h.savePNG("cmdline", "area-folded-recent.png")

	e := chromeRig(t, NewWorkspace(doc.New(), nil))
	e.ws.lay.histOff = true
	e.frame()
	e.savePNG("cmdline", "area-folded-empty.png")
}

// band returns the rows, top to bottom, that hit-test to id at x; 0, 0 when none does.
func (h *sHarness) band(id string, x float32) (top, bottom float32) {
	w := h.hw.Window()
	top = -1
	for y := float32(0); y < float32(w.Bounds().Size.H); y++ {
		gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
		if ok && len(gid.Path) > 0 && gid.Path[len(gid.Path)-1].Key == id {
			if top < 0 {
				top = y
			}
			bottom = y + 1
		}
	}
	if top < 0 {
		return 0, 0
	}
	return top, bottom
}

func TestFoldedHistoryShowsItsLastLine(t *testing.T) {
	h := cmdAreaRig(t)
	ws, w := h.ws, h.hw.Window()
	h.clickID("cmd-collapse")
	h.frame()
	if !ws.lay.histOff || h.has("cmd-history") {
		t.Fatal("history did not fold")
	}
	// One row of the line's height over the box, the fold button still on screen.
	top, bottom := h.band("cmd-last", 100)
	if bottom-top != cmdFoldRowHeight {
		t.Fatalf("folded row %v px high, want %v", bottom-top, float32(cmdFoldRowHeight))
	}
	if bt, _ := h.band("cmd-box", 100); bt < bottom || bt > bottom+cmdBoxGap+1 {
		t.Fatalf("box top %v, want right under the folded row's bottom %v", bt, bottom)
	}
	if !h.has("cmd-collapse") {
		t.Fatal("no fold button on the folded history")
	}

	// The long last line is cut to the row with an ellipsis; a short one shows whole.
	width := ws.mainWidth(ws.winW, 0, 0)
	got := ws.foldedLine(w, width)
	last := ws.Session().History()[len(ws.Session().History())-1]
	if !strings.HasSuffix(got, "…") || !strings.HasPrefix(last, strings.TrimSuffix(got, "…")) || len(got) >= len(last) {
		t.Fatalf("folded line %q, want a cut of %q with an ellipsis", got, last)
	}
	tw, _ := w.TextMeasurer().MeasureWrapped(got, gpui.TextStyle{FontFamily: monoFamily, FontSize: fontUI, LineHeight: cmdLineHeight}, 1e9)
	if float32(tw) > width-cmdHistPadL-cmdHistPadR {
		t.Fatalf("folded line %v px wide, room for %v", tw, width-cmdHistPadL-cmdHistPadR)
	}
	ws.Session().Script("FOO")
	h.frame()
	last = ws.Session().History()[len(ws.Session().History())-1]
	if got := ws.foldedLine(w, width); got != last {
		t.Fatalf("folded line %q, want the last line %q", got, last)
	}

	// Unfolding brings the history region back.
	h.clickID("cmd-collapse")
	h.frame()
	if ws.lay.histOff || !h.has("cmd-history") || h.has("cmd-last") {
		t.Fatal("history did not unfold")
	}
}

func TestFoldedEmptyHistoryKeepsTheFoldButton(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	ws.lay.histOff = true
	h.frame()
	if n := len(ws.Session().History()); n != 0 {
		t.Skipf("a new workspace starts with %d history lines", n)
	}
	if got := ws.foldedLine(h.hw.Window(), 800); got != "" {
		t.Fatalf("folded line %q for an empty history", got)
	}
	h.clickID("cmd-collapse")
	h.frame()
	if ws.lay.histOff {
		t.Fatal("the fold button of an empty folded history does not unfold it")
	}
}

func TestCommandAreaIsCompact(t *testing.T) {
	h := cmdAreaRig(t)
	ws := h.ws
	// Expanded: drag strip, history, gap, box, padding.
	top, _ := h.band("cmd-handle", 100)
	_, bottom := h.band("cmd-box", 100)
	if want := float32(cmdHandleHeight + cmdBoxGap + cmdBoxHeight); !nearPx(bottom-top, want+ws.historyHeight()) {
		t.Fatalf("expanded command area %v px from the handle to the box bottom, want %v", bottom-top, want+ws.historyHeight())
	}
	// The suggestion list sits right above the box.
	h.typeCmd("li")
	bt, _ := h.band("cmd-box", 100)
	_, sb := h.band("cmd-suggest", 10) // the list's padding, left of its rows
	if sb == 0 || sb > bt || bt-sb > 16 {
		t.Fatalf("suggestions end at %v, box starts at %v: want the list just above the box", sb, bt)
	}
}
