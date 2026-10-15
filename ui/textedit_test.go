package ui

import (
	"math"
	"os"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

func textEditRig(t *testing.T, e doc.Entity) (*sHarness, doc.Handle) {
	t.Helper()
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(e)
	tx.Commit()
	return openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, 1), h
}

func openTextEdit(t *testing.T, h *sHarness, target doc.Handle) *textEditState {
	t.Helper()
	h.ws.session.Selection().Clear()
	h.ws.session.Selection().Add(target)
	h.ws.run("TEXTEDIT", h.hw.Window())
	h.hw.Pixels()
	if h.ws.textEditor == nil || !h.frameIDs()["textedit-card"] {
		t.Fatalf("TEXTEDIT did not open its editor: state %v active %q history %v", h.ws.textEditor != nil, h.ws.session.Active(), h.ws.session.History())
	}
	return h.ws.textEditor
}

func TestTextEditApplyUndoRedoAndIndex(t *testing.T) {
	h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
	d := h.ws.session.Drawing()
	depth := d.UndoDepth()
	s := openTextEdit(t, h, target)
	for name, value := range map[string]string{"content": "Changed", "height": "20", "rotation": "90", "justify": "TC", "style": "standard"} {
		s.fields[name].SetText(value)
	}
	if d.Entity(target).(*doc.Text).Value != "Original" || h.ws.session.Dirty() {
		t.Fatal("draft changes leaked into the drawing")
	}
	h.clickID("textedit-apply")
	h.hw.Pixels()
	e := d.Entity(target).(*doc.Text)
	if h.ws.textEditor != nil || e.Value != "Changed" || e.Height != 20 || e.Justify() != "TC" || math.Abs(e.Rotation-math.Pi/2) > 1e-12 {
		t.Fatalf("apply: editor %v entity %+v", h.ws.textEditor, e)
	}
	if !h.ws.session.Dirty() || h.ws.session.Modified() != 1 || d.UndoDepth() != depth+1 || d.UndoName() != "TEXTEDIT" || d.Len() != 1 {
		t.Fatal("Apply must modify the same handle in one named, dirty undo step")
	}
	box := e.Bounds()
	if hs := h.ws.session.Index().Query(box); len(hs) != 1 || hs[0] != target {
		t.Fatal("spatial index did not follow the edited text")
	}
	h.ws.session.Run("U")
	if d.Entity(target).(*doc.Text).Value != "Original" || d.Entity(target).(*doc.Text).Height != 10 {
		t.Fatal("Undo did not restore the original text")
	}
	h.ws.session.Run("REDO")
	if d.Entity(target).(*doc.Text).Value != "Changed" {
		t.Fatal("Redo did not restore the edit")
	}
}

func TestTextEditCancelEscapeAndNoOp(t *testing.T) {
	for _, close := range []string{"cancel", "escape", "noop", "equivalent"} {
		t.Run(close, func(t *testing.T) {
			h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			d := h.ws.session.Drawing()
			depth := d.UndoDepth()
			s := openTextEdit(t, h, target)
			switch close {
			case "cancel":
				s.fields["content"].SetText("Changed")
				h.clickID("textedit-cancel")
			case "escape":
				s.fields["content"].SetText("Changed")
				h.key("escape")
			case "noop":
				h.clickID("textedit-apply")
			case "equivalent":
				s.fields["height"].SetText("10.000")
				h.clickID("textedit-apply")
			}
			h.hw.Pixels()
			if h.ws.textEditor != nil || h.ws.session.Dirty() || d.UndoDepth() != depth || d.Entity(target).(*doc.Text).Value != "Original" {
				t.Fatal("Cancel/Escape/no-op must leave drawing, dirty state and undo depth unchanged")
			}
			if fh := h.ws.input.FocusHandle(); fh == nil || !fh.IsFocused(h.hw.Window()) {
				t.Fatal("closing editor did not restore command-line focus")
			}
		})
	}
}

func TestTextEditMTextRawPreservationAndEscaping(t *testing.T) {
	raw := `{\C1;Original}\Ppath\\file\{x\}`
	h, target := textEditRig(t, doc.NewMText(geom.V(0, 0), 10, 50, raw))
	s := openTextEdit(t, h, target)
	if s.fields["content"].Text() != doc.MTextPlain(raw) {
		t.Fatal("editor must show plain MTEXT contents")
	}
	s.fields["height"].SetText("12")
	s.fields["width"].SetText("80")
	s.fields["justify"].SetText("BR")
	h.clickID("textedit-apply")
	h.hw.Pixels()
	e := h.ws.session.Drawing().Entity(target).(*doc.MText)
	if e.Value != raw || e.Height != 12 || e.Width != 80 || e.Attachment != 9 {
		t.Fatalf("property-only edit lost raw MTEXT: %+v", e)
	}
	s = openTextEdit(t, h, target)
	plain := "{literal}\\path\nsecond\n"
	s.fields["content"].SetText(plain)
	h.clickID("textedit-apply")
	h.hw.Pixels()
	e = h.ws.session.Drawing().Entity(target).(*doc.MText)
	if doc.MTextPlain(e.Value) != plain || !strings.Contains(e.Value, `\{literal\}`) || !strings.Contains(e.Value, `\\path\P`) {
		t.Fatalf("literal MTEXT was not escaped: %q", e.Value)
	}
	s = openTextEdit(t, h, target)
	s.fields["content"].SetText("")
	h.clickID("textedit-apply")
	if h.ws.session.Drawing().Entity(target).(*doc.MText).Value != "" {
		t.Fatal("blank content must apply rather than cancel")
	}
}

func TestTextEditRejectsInvalidAndStaleTargets(t *testing.T) {
	for _, change := range []string{"erase", "modify", "lock", "off", "frozen", "transaction", "number", "style", "justify", "newline"} {
		t.Run(change, func(t *testing.T) {
			h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			d := h.ws.session.Drawing()
			s := openTextEdit(t, h, target)
			s.fields["content"].SetText("Changed")
			switch change {
			case "erase", "modify":
				tx, _ := d.Begin()
				if change == "erase" {
					tx.Erase(target)
				} else {
					tx.Modify(target, func(e doc.Entity) { e.(*doc.Text).Value = "Elsewhere" })
				}
				tx.Commit()
			case "lock", "off", "frozen":
				l := d.Layer("0")
				l.Locked, l.Off, l.Frozen = change == "lock", change == "off", change == "frozen"
			case "transaction":
				tx, _ := d.Begin()
				defer tx.Abort()
			case "number":
				s.fields["height"].SetText("NaN")
			case "style":
				s.fields["style"].SetText("Missing")
			case "justify":
				s.fields["justify"].SetText("Wrong")
			case "newline":
				s.fields["content"].SetText("first\nsecond")
			}
			depth, mods := d.UndoDepth(), h.ws.session.Modified()
			h.clickID("textedit-apply")
			h.hw.Pixels()
			if h.ws.textEditor == nil || s.err == "" || d.UndoDepth() != depth || h.ws.session.Modified() != mods {
				t.Fatal("invalid/stale edit was applied or gave no useful error")
			}
		})
	}
}

func TestTextEditDoubleClickOnlyWhenIdle(t *testing.T) {
	for _, command := range []string{"", "LINE", "ERASE"} {
		t.Run(command, func(t *testing.T) {
			h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			if command != "" {
				h.ws.run(command, h.hw.Window())
			}
			p := h.ws.ScreenPoint(geom.V(2, 3))
			h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 2})
			h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 2})
			h.hw.Pixels()
			if (h.ws.textEditor != nil) != (command == "") {
				t.Fatalf("double-click with %q running opened %v", command, h.ws.textEditor != nil)
			}
			if command == "" && (h.ws.textEditor.original.Handle() != target || h.ws.canvas.pressed || h.ws.canvas.boxing) {
				t.Fatal("idle double-click lost target or left selection drag active")
			}
		})
	}
}

func TestTextEditModalBlocksBackgroundAndSupportsTab(t *testing.T) {
	h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
	s := openTextEdit(t, h, target)
	for _, key := range []string{"z", "s", "o", "tab", ","} {
		h.ctrl(key)
		h.hw.Pixels()
	}
	if h.ws.session.Active() != "" || h.ws.SettingsOpen() || h.ws.textEditor != s || h.ws.session.Drawing().Entity(target) == nil {
		t.Fatal("modal let drawing actions reach the workspace")
	}
	h.key("tab")
	h.hw.Pixels()
	if !s.fields["height"].FocusHandle().IsFocused(h.hw.Window()) {
		t.Fatal("Tab did not move from contents to height")
	}
	for _, name := range []string{"rotation", "justify", "style", "textedit-cancel", "textedit-apply", "content"} {
		h.key("tab")
		h.hw.Pixels()
		fh := s.buttons[name]
		if f := s.fields[name]; f != nil {
			fh = f.FocusHandle()
		}
		if fh == nil || !fh.IsFocused(h.hw.Window()) {
			t.Fatalf("Tab did not reach %s within the modal", name)
		}
	}
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Shift}})
	h.hw.Pixels()
	if !s.buttons["textedit-apply"].IsFocused(h.hw.Window()) {
		t.Fatal("Shift+Tab escaped the modal instead of wrapping to Apply")
	}
	h.key("tab")
	h.hw.Pixels()
	h.typeText(" typed")
	if s.fields["content"].Text() != "Original typed" {
		t.Fatal("modal intercepted ordinary typed text")
	}
	view := *h.ws.view
	p := gpui.Point{X: 50, Y: 300}
	h.hw.Dispatch(gpui.ScrollWheelEvent{Position: p, DeltaY: 50})
	h.click(p)
	h.hw.Dispatch(gpui.FileDropEvent{Position: p, Paths: []string{"does-not-exist.dxf"}})
	h.hw.Pixels()
	if h.ws.view.Scale != view.Scale || h.ws.view.Center != view.Center || h.ws.textEditor != s {
		t.Fatal("backdrop pointer events changed the drawing view or dismissed the draft")
	}
	h.ctrl("o")
	h.hw.Pixels()
	if h.ws.session.Active() != "" || h.ws.textEditor != s {
		t.Fatal("backdrop click let keyboard focus escape the modal")
	}
}

func TestTextEditSmallWindowKeepsFooterAndFocusedFieldsReachable(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	target := tx.Add(doc.NewMText(geom.V(0, 0), 10, 50, "first\nsecond\nthird\nfourth\nfifth\nsixth"))
	tx.Commit()
	h := openSettingsWS(t, NewWorkspace(d, nil), 640, 400, 1)
	s := openTextEdit(t, h, target)
	if !h.inFrame("textedit-apply") || !h.inFrame("textedit-cancel") || s.scroll.MaxOffset().H <= 0 {
		t.Fatal("short editor must scroll its body and keep footer buttons visible")
	}
	for _, name := range []string{"height", "rotation", "justify", "style", "width"} {
		h.key("tab")
		h.hw.Pixels()
		if !s.fields[name].FocusHandle().IsFocused(h.hw.Window()) || !h.frameIDs()["textedit-"+name] {
			t.Fatalf("Tab did not reveal the focused %s field", name)
		}
	}
	if err := h.hw.SavePNG("../_tmp/n26/mtext-editor-small.png"); err != nil {
		t.Fatal(err)
	}
	h.clickID("textedit-cancel")
	if h.ws.textEditor != nil || h.ws.session.Dirty() {
		t.Fatal("short-window Cancel did not close without an edit")
	}
}

func TestTextEditAlignedAndFitProperties(t *testing.T) {
	for _, align := range []doc.TextHAlign{doc.TextAligned, doc.TextFit} {
		e := doc.NewText(geom.V(0, 0), 10, 0, "Original")
		e.SetJustify(align, doc.TextBaseline)
		h, target := textEditRig(t, e)
		s := openTextEdit(t, h, target)
		s.fields["height"].SetText("20")
		s.fields["rotation"].SetText("90")
		h.clickID("textedit-apply")
		got := h.ws.session.Drawing().Entity(target).(*doc.Text)
		lay := got.Layout()
		if math.Abs(lay.Height-20) > 1e-9 || math.Abs(lay.Rotation-math.Pi/2) > 1e-9 || got.Insert != geom.V(0, 0) || got.HAlign != align {
			t.Fatalf("alignment %v: layout %+v", align, lay)
		}
	}
}

func TestTextEditRejectsNonTextMissingAndBlockOwnedTargets(t *testing.T) {
	h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
	d := h.ws.session.Drawing()
	tx, _ := d.Begin()
	line := tx.Add(&doc.Line{Start: geom.V(0, 20), End: geom.V(10, 20)})
	tx.Commit()
	for _, bad := range []doc.Handle{0, 99999, line} {
		if h.ws.openTextEditor(h.ws.docTab, bad) {
			t.Fatalf("opened non-text target %v", bad)
		}
	}
	d.Entity(target).Common().Owner = 12345
	if h.ws.openTextEditor(h.ws.docTab, target) {
		t.Fatal("opened block-owned text")
	}
}

func TestTextEditDraftClosesOnTabOrDrawingChange(t *testing.T) {
	for _, change := range []string{"tab", "drawing", "command"} {
		t.Run(change, func(t *testing.T) {
			h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			openTextEdit(t, h, target).fields["content"].SetText("Changed")
			d := h.ws.session.Drawing()
			switch change {
			case "tab":
				h.ws.activate(h.ws.newDocTab(doc.New()))
			case "drawing":
				h.ws.session.SetDrawing(doc.New())
			case "command":
				h.ws.session.Run("LINE")
			}
			h.hw.Pixels()
			if h.ws.textEditor != nil || d.Entity(target).(*doc.Text).Value != "Original" {
				t.Fatal("draft survived a changed editing context or mutated the old drawing")
			}
		})
	}
}

func TestRenderTextEditor(t *testing.T) {
	if err := os.MkdirAll("../_tmp/n26", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, multi := range []bool{false, true} {
		var e doc.Entity = doc.NewText(geom.V(0, 0), 10, 0, "Original text")
		name := "text"
		if multi {
			e, name = doc.NewMText(geom.V(0, 0), 10, 75, `{\C1;Original}\Psecond line`), "mtext"
		}
		h, target := textEditRig(t, e)
		s := openTextEdit(t, h, target)
		if err := h.hw.SavePNG("../_tmp/n26/" + name + "-editor.png"); err != nil {
			t.Fatal(err)
		}
		s.fields["content"].SetText("Edited contents")
		s.fields["height"].SetText("12")
		h.clickID("textedit-apply")
		h.hw.Pixels()
		if err := h.hw.SavePNG("../_tmp/n26/" + name + "-result.png"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTextEditEndsExistingWorkspaceDrags(t *testing.T) {
	for _, control := range []string{"split-side", "cmd-handle", "status-zoom-slider"} {
		t.Run(control, func(t *testing.T) {
			h, target := textEditRig(t, doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			h.ws.session.Selection().Add(target)
			p := h.find(control)
			h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
			if h.ws.lay.drag.kind == splitNone && !h.ws.zoom.Dragging() {
				t.Fatal("pointer press did not begin a workspace drag")
			}
			// A user can still run a command while holding a splitter or zoom thumb.
			h.hw.Window().RequestFocus(h.ws.input.FocusHandle())
			h.hw.Pixels()
			h.typeText("TEXTEDIT")
			h.key("enter")
			h.hw.Pixels()
			if h.ws.textEditor == nil {
				t.Fatal("TEXTEDIT did not open during the held drag")
			}
			side, history, scale := h.ws.lay.sideW, h.ws.cmd.histH, h.ws.view.Scale
			to := gpui.Point{X: 500, Y: 300}
			h.hw.Dispatch(gpui.MouseMoveEvent{Position: to})
			h.hw.Dispatch(gpui.MouseUpEvent{Position: to, Button: gpui.MouseLeft, ClickCount: 1})
			h.hw.Pixels()
			if h.ws.lay.sideW != side || h.ws.cmd.histH != history || h.ws.view.Scale != scale || h.ws.lay.drag.kind != splitNone || h.ws.zoom.Dragging() || h.ws.zoomSaved {
				t.Fatalf("modal retained a workspace drag: side %v→%v history %v→%v scale %v→%v split %v zoom %v", side, h.ws.lay.sideW, history, h.ws.cmd.histH, scale, h.ws.view.Scale, h.ws.lay.drag.kind, h.ws.zoom.Dragging())
			}
			h.clickID("textedit-cancel")
			h.hw.Pixels()
			h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 400}})
			h.hw.Pixels()
			if h.ws.lay.sideW != side || h.ws.cmd.histH != history || h.ws.view.Scale != scale || h.ws.lay.drag.kind != splitNone || h.ws.zoom.Dragging() {
				t.Fatal("workspace drag resumed after cancelling the modal")
			}
		})
	}
}
