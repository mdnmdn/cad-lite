package ui

import (
	"bytes"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cad-lite/doc"
	"cad-lite/feedback"
	"cad-lite/filedialog"
	"cad-lite/internal/gpui"
)

func feedbackSettle(t *testing.T, h *sHarness) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.ws.fb.busy {
		if time.Now().After(deadline) {
			t.Fatal("feedback job did not finish")
		}
		time.Sleep(10 * time.Millisecond)
		h.hp.Advance(110 * time.Millisecond)
		h.hw.Pixels()
	}
}

func TestFeedbackDialogCaptureEditorAndIsolation(t *testing.T) {
	h := openSettingsApp(t)
	t.Cleanup(h.ws.ShutdownFeedback)
	h.clickID("tool-feedback")
	h.hw.Pixels()
	f := h.ws.fb
	if f == nil || !f.open || f.shot == nil {
		t.Fatal("feedback did not capture application")
	}
	if _, ok := h.locate("feedback-close"); !ok {
		t.Fatal("dialog missing")
	}
	if _, ok := h.locate("canvas"); ok {
		t.Fatal("drawing remains interactive behind feedback")
	}
	h.clickID("feedback-send")
	if f.busy {
		t.Fatal("unconfigured build submitted feedback")
	}
	h.png("feedback-dialog.png")
	original := bytes.Clone(f.shot.Pix)
	h.clickID("feedback-preview")
	h.hw.Pixels()
	if !f.editing {
		t.Fatal("screenshot editor did not open")
	}
	b := f.imageBounds
	start := gpui.Point{X: b.Origin.X + b.Size.W/3, Y: b.Origin.Y + b.Size.H/2}
	end := start
	end.X += b.Size.W / 4
	h.hw.Dispatch(gpui.MouseDownEvent{Position: start, Button: gpui.MouseLeft})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: end})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: end, Button: gpui.MouseLeft})
	h.hw.Pixels()
	if !f.editor.CanUndo() {
		t.Fatal("pen stroke not recorded")
	}
	if bytes.Equal(f.editor.Image().Pix, original) {
		t.Fatal("pen did not annotate screenshot")
	}
	h.png("feedback-editor.png")
	h.clickID("feedback-edit-cancel")
	h.hw.Pixels()
	if f.editing || !bytes.Equal(f.shot.Pix, original) {
		t.Fatal("cancel mutated screenshot")
	}
	h.clickID("feedback-preview")
	h.hw.Pixels()
	f.editor.Begin(feedback.Point{X: 10, Y: 10}, 10, color.NRGBA{255, 0, 0, 255}, false)
	f.editor.End()
	h.clickID("feedback-edit-save")
	h.hw.Pixels()
	if bytes.Equal(f.shot.Pix, original) {
		t.Fatal("save lost annotation")
	}
	if h.ws.Session().Dirty() {
		t.Fatal("annotation changed drawing")
	}
	h.key("escape")
	h.hw.Pixels()
	if f.open {
		t.Fatal("escape did not close feedback")
	}
}

func TestFeedbackSubmissionAndRetryDraft(t *testing.T) {
	var received []feedback.Report
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/feedback/test" {
			w.Write([]byte(`{"ok":true,"kinds":["bug","improvement","note"]}`))
			return
		}
		var report feedback.Report
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
		received = append(received, report)
		if fail {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"reference":"FB-77","url":"https://private/admin"}`))
	}))
	defer srv.Close()
	h := openSettingsApp(t)
	t.Cleanup(h.ws.ShutdownFeedback)
	h.ws.SetVersion("1.2.3")
	h.ws.ConfigureFeedback(&feedback.Client{URL: srv.URL, Key: "test", Attempts: 1, Outbox: &feedback.Outbox{Dir: t.TempDir()}})
	h.clickID("tool-feedback")
	h.hw.Pixels()
	feedbackSettle(t, h)
	f := h.ws.fb
	f.title.SetText("Test report")
	f.description.SetText("Steps to reproduce")
	h.clickID("feedback-send")
	h.hw.Pixels()
	feedbackSettle(t, h)
	if f.pending == nil || len(received) != 1 {
		t.Fatal("failed report was not retained")
	}
	first := received[0]
	if first.Title != "Test report" || first.Version != "1.2.3" || first.Screenshot == "" || first.Context["architecture"] == nil || first.Context["os_version"] == nil {
		t.Fatalf("missing report content or metadata: %+v", first.Context)
	}
	fail = false
	h.clickID("feedback-send")
	h.hw.Pixels()
	feedbackSettle(t, h)
	if len(received) != 2 || received[1].ID != first.ID {
		t.Fatal("manual retry changed report ID")
	}
	if !strings.Contains(f.message, "FB-77") || strings.Contains(f.message, "private") || f.pending != nil {
		t.Fatalf("bad receipt: %s", f.message)
	}
	pending, _ := f.client.Outbox.List()
	if len(pending) != 0 {
		t.Fatal("successful delivery remains in outbox")
	}
}

func TestFeedbackFitsSmallWindow(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 480, 520, 1)
	t.Cleanup(h.ws.ShutdownFeedback)
	h.clickID("tool-feedback")
	h.hw.Pixels()
	if _, ok := h.locate("feedback-close"); !ok {
		t.Fatal("close button clipped")
	}
	if _, ok := h.locate("feedback-send"); !ok {
		t.Fatal("send button clipped")
	}
}

func TestFeedbackAttachCancelKeepsDraft(t *testing.T) {
	h := openSettingsApp(t)
	t.Cleanup(h.ws.ShutdownFeedback)
	h.ws.SetFileDialog(&fakeDialog{err: filedialog.ErrCancelled})
	h.clickID("tool-feedback")
	h.hw.Pixels()
	f := h.ws.fb
	shot := f.shot
	h.clickID("feedback-attach")
	h.hw.Pixels()
	feedbackSettle(t, h)
	if f.shot != shot || f.message != "" {
		t.Fatalf("cancelling attachment changed draft or reported a failure: %q", f.message)
	}
}

func TestFeedbackRemovedScreenshotStaysRemovedAfterClose(t *testing.T) {
	h := openSettingsApp(t)
	t.Cleanup(h.ws.ShutdownFeedback)
	h.clickID("tool-feedback")
	h.hw.Pixels()
	h.ws.fb.title.SetText("Report without a screenshot")
	h.clickID("feedback-remove")
	h.clickID("feedback-close")
	h.hw.Pixels()
	h.clickID("tool-feedback")
	h.hw.Pixels()
	if h.ws.fb.shot != nil || h.ws.fb.title.Text() != "Report without a screenshot" {
		t.Fatal("reopening changed the draft's screenshot choice")
	}
}

func TestFeedbackEditorUsesOppositeTheme(t *testing.T) {
	for _, appTheme := range []Theme{themeDark, themeLight, themeClassic, themeHighContrast} {
		t.Run(appTheme.Name, func(t *testing.T) {
			h := openSettingsApp(t)
			t.Cleanup(h.ws.ShutdownFeedback)
			ApplyTheme(appTheme)
			h.hw.Window().Refresh()
			h.hw.Pixels()
			h.clickID("tool-feedback")
			h.hw.Pixels()
			opposite := themeLight
			if !appTheme.Dark() {
				opposite = themeDark
			}
			closeButton := h.find("feedback-close")
			themedDialog := false
			for _, q := range h.hw.Window().Scene().Quads {
				if q.Bounds.Contains(closeButton) && q.Fill == opposite.Panel {
					themedDialog = true
				}
			}
			if !themedDialog {
				t.Fatal("feedback dialog does not share the editor's contrasting theme")
			}
			fields := 0
			for _, q := range h.hw.Window().Scene().Quads {
				if q.Fill == opposite.Field && q.Bounds.Size.W > 300 && q.Bounds.Size.H >= 30 && q.Bounds.Size.H < 80 {
					fields++
				}
			}
			if fields != 2 {
				t.Fatalf("expected two fields in the contrasting palette; got %d", fields)
			}
			h.png("feedback-dialog-opposite-" + appTheme.Name + ".png")
			original := bytes.Clone(h.ws.fb.shot.Pix)
			h.clickID("feedback-preview")
			h.hw.Pixels()
			toolbar := h.hw.Image().NRGBAAt(5, 5)
			if toolbar != (color.NRGBA{R: opposite.Panel.R, G: opposite.Panel.G, B: opposite.Panel.B, A: 255}) {
				t.Fatalf("editor toolbar uses %v; want opposite theme %s (%v)", toolbar, opposite.Name, opposite.Panel)
			}
			pen := h.find("feedback-pen")
			themedButton := false
			for _, q := range h.hw.Window().Scene().Quads {
				if q.Bounds.Contains(pen) && q.Fill == opposite.PanelAlt {
					themedButton = true
				}
			}
			if !themedButton {
				t.Fatal("editor button did not use the opposite palette")
			}
			track := h.ws.fb.slider.track
			thumbX := int(float64(track.Origin.X) + sliderThumb/2 + (float64(track.Size.W)-sliderThumb)*(h.ws.fb.width-1)/49)
			thumbY := int(track.Origin.Y + track.Size.H/2)
			thumb := h.hw.Image().NRGBAAt(thumbX, thumbY)
			if thumb != (color.NRGBA{R: opposite.Text.R, G: opposite.Text.G, B: opposite.Text.B, A: 255}) {
				t.Fatalf("slider thumb uses %v; want %v", thumb, opposite.Text)
			}
			margin := h.hw.Image().NRGBAAt(0, 699)
			if margin != (color.NRGBA{R: opposite.Bg.R, G: opposite.Bg.G, B: opposite.Bg.B, A: 255}) {
				t.Fatalf("editor background uses %v; want %v", margin, opposite.Bg)
			}
			if CurrentTheme() != appTheme || !bytes.Equal(original, h.ws.fb.editor.Image().Pix) {
				t.Fatal("editor changed application theme or screenshot")
			}
			h.png("feedback-editor-opposite-" + appTheme.Name + ".png")
			h.clickID("feedback-edit-cancel")
			h.hw.Pixels()
			if CurrentTheme() != appTheme || !bytes.Equal(original, h.ws.fb.shot.Pix) {
				t.Fatal("closing editor changed application theme or attachment")
			}
		})
	}
}
