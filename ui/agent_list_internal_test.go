package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"agentmanager"
	"agentmanager/transcript"
	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// longTranscriptRig opens the agent panel on a transcript of turns*5 blocks (user, thought, tool,
// agent message, debug note) in a headless window.
type longTranscriptRig struct {
	t    testing.TB
	ws   *Workspace
	p    *AgentPanel
	hw   *gpui.HeadlessWindow
	plat *gpui.HeadlessPlatform
}

func newLongTranscriptRig(t testing.TB, turns int) *longTranscriptRig {
	t.Helper()
	ws := NewWorkspace(doc.New(), nil)
	ws.SetProjectPath(filepath.Join(t.TempDir(), "plate.dxf"))
	p := ws.AgentPanel()
	p.hist.loaded, p.hist.off = true, true
	p.Configure(agentmanager.Claude, "")
	p.Show()
	p.Seed(func(tr *transcript.T) {
		for i := 0; i < turns; i++ {
			tr.AddUser(fmt.Sprintf("Request %d: draw a plate with holes", i))
			tr.Apply(agentmanager.ThoughtDelta{MessageID: fmt.Sprint("th", i), Text: strings.Repeat("reasoning about the plate ", 12)})
			tr.Apply(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: fmt.Sprint("tool", i), Title: "cad add_rect",
				Kind: agentmanager.KindEdit, Status: agentmanager.ToolInProgress}})
			tr.Apply(agentmanager.TextDelta{MessageID: fmt.Sprint("m", i), Text: strings.Repeat("Done with step. ", 1+i%7)})
			tr.Apply(agentmanager.Log{Level: "debug", Message: "hidden"})
		}
	})
	plat := gpui.NewHeadlessPlatform()
	if err := plat.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	r := &longTranscriptRig{t: t, ws: ws, p: p, hw: plat.Windows()[0], plat: plat}
	r.hw.Pixels()
	t.Cleanup(ws.CloseAgent)
	return r
}

func (r *longTranscriptRig) frame() {
	r.hw.Window().Refresh()
	r.hw.Pixels()
}

// measured counts the transcript items with a cached height.
func (r *longTranscriptRig) measured(n int) int {
	c := 0
	for i := 0; i < n; i++ {
		if _, ok := r.p.cur.list.MeasuredHeight(i); ok {
			c++
		}
	}
	return c
}

// find returns the centre of the element whose id is key, by hit testing the panel.
func (r *longTranscriptRig) find(key string) (gpui.Point, bool) {
	w := r.hw.Window()
	b := w.Bounds()
	for y := gpui.Pixels(0); y < b.Size.H; y += 3 {
		for x := b.Size.W - 300; x < b.Size.W; x += 20 {
			id, ok := w.HitTest(gpui.Point{X: x, Y: y})
			if !ok {
				continue
			}
			for _, k := range id.Path {
				if k.Key == key {
					return gpui.Point{X: x, Y: y + 2}, true
				}
			}
		}
	}
	return gpui.Point{}, false
}

func (r *longTranscriptRig) click(at gpui.Point) {
	r.hw.Dispatch(gpui.MouseMoveEvent{Position: at})
	r.hw.Dispatch(gpui.MouseDownEvent{Position: at, Button: gpui.MouseLeft, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: at, Button: gpui.MouseLeft, ClickCount: 1}
	r.hw.Dispatch(up)
	r.hw.Dispatch(gpui.ClickEvent{Up: &up})
}

func TestAgentTranscriptBuildsOnlyVisibleBlocks(t *testing.T) {
	const turns = 500 // 2,000 shown blocks, 500 hidden debug notes
	r := newLongTranscriptRig(t, turns)
	shown := 4 * turns
	l := r.p.cur.list
	// The transcript follows its tail: the view sits at the bottom of the true content height.
	s, e := l.VisibleRange()
	if e != shown {
		t.Fatalf("visible range [%d,%d), want it to end at %d", s, e, shown)
	}
	if e-s > 40 {
		t.Fatalf("built %d blocks for one viewport", e-s)
	}
	if o, m := r.p.cur.scroll.Offset().Y, r.p.cur.scroll.MaxOffset().H; o != m || m <= 0 {
		t.Fatalf("offset %v, max %v: not at the bottom", o, m)
	}
	if n := r.measured(shown); n > 80 {
		t.Fatalf("%d of %d blocks laid out: the transcript is not virtualized", n, shown)
	}
	// Unmeasured blocks still count: the content is far taller than what was built.
	if h := l.ContentHeight(); h < 20*gpui.Pixels(shown) {
		t.Fatalf("content height %v for %d blocks", h, shown)
	}
	// Steady frames build the same small range.
	r.frame()
	if s2, e2 := l.VisibleRange(); s2 != s || e2 != e {
		t.Fatalf("steady frame range [%d,%d), was [%d,%d)", s2, e2, s, e)
	}
}

func TestAgentTranscriptThoughtExpandsAndStreamingFollowsTail(t *testing.T) {
	r := newLongTranscriptRig(t, 200)
	shown := 4 * 200
	// The last thought is block 5*199+1 of the transcript (debug notes count) and item 4*199+1
	// of the list (they are not shown).
	block, item := 5*199+1, 4*199+1
	at, ok := r.find(fmt.Sprintf("agent-thought-%d", block))
	if !ok {
		t.Fatal("last thought not on screen")
	}
	before, _ := r.p.cur.list.MeasuredHeight(item)
	r.click(at)
	if !r.p.cur.expanded[block] {
		t.Fatal("click did not expand the thought")
	}
	r.frame()
	if after, ok := r.p.cur.list.MeasuredHeight(item); !ok || after <= before {
		t.Fatalf("expanded thought height %v (was %v)", after, before)
	}

	// The streaming message grows; the view stays at the true bottom.
	last := shown - 1
	h0, _ := r.p.cur.list.MeasuredHeight(last)
	r.p.cur.tr.Apply(agentmanager.TextDelta{MessageID: "m199", Text: strings.Repeat(" more streamed text", 40)})
	// The turn is still running: the message is re-parsed after agentParseEvery.
	if err := r.plat.Advance(agentParseEvery); err != nil {
		t.Fatal(err)
	}
	r.frame()
	if h1, _ := r.p.cur.list.MeasuredHeight(last); h1 <= h0 {
		t.Fatalf("streamed message height %v (was %v)", h1, h0)
	}
	if o, m := r.p.cur.scroll.Offset().Y, r.p.cur.scroll.MaxOffset().H; o != m {
		t.Fatalf("offset %v, max %v after streaming", o, m)
	}
}

func TestAgentTranscriptOffscreenChangeInvalidates(t *testing.T) {
	r := newLongTranscriptRig(t, 200)
	r.p.cur.scroll.ScrollToTop()
	r.frame()
	tool := 2 // the first tool card
	if _, ok := r.p.cur.list.MeasuredHeight(tool); !ok {
		t.Fatal("first tool card not measured at the top")
	}
	// A tool update far above the viewport (the panel follows the tail on every change).
	done := agentmanager.ToolCompleted
	out := strings.Repeat("output line ", 20)
	r.p.cur.tr.Apply(agentmanager.ToolCallUpdate{Update: agentmanager.ToolCallPatch{ID: "tool0", Status: &done, Output: &out}})
	r.frame()
	if s, _ := r.p.cur.list.VisibleRange(); s <= tool {
		t.Fatalf("view did not follow the tail (range starts at %d)", s)
	}
	if _, ok := r.p.cur.list.MeasuredHeight(tool); ok {
		t.Fatal("changed tool card kept its stale height")
	}
}

func TestAgentTranscriptThemeChangeDropsHeights(t *testing.T) {
	r := newLongTranscriptRig(t, 200)
	r.p.cur.scroll.ScrollToTop()
	r.frame()
	r.p.cur.scroll.ScrollToBottom()
	r.frame()
	if _, ok := r.p.cur.list.MeasuredHeight(0); !ok {
		t.Fatal("first block not measured after visiting the top")
	}
	prev := CurrentTheme()
	t.Cleanup(func() { ApplyTheme(prev) })
	next := themeLight
	if prev.Name == next.Name {
		next = themeDark
	}
	ApplyTheme(next)
	r.frame()
	if _, ok := r.p.cur.list.MeasuredHeight(0); ok {
		t.Fatal("off-screen block kept its height across a theme change")
	}
	if s, e := r.p.cur.list.VisibleRange(); e != 4*200 || e-s > 40 {
		t.Fatalf("range after theme change [%d,%d)", s, e)
	}
}
