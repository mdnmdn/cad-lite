package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"cad-lite/assets/icons"
	"cad-lite/feedback"
	"cad-lite/filedialog"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

type feedbackResult struct {
	report    *feedback.Report
	receipt   feedback.Receipt
	info      *feedback.Info
	shot      *image.NRGBA
	err       error
	operation string
}

type feedbackState struct {
	client                        *feedback.Client
	ctx                           context.Context
	cancel                        context.CancelFunc
	results                       chan feedbackResult
	poll                          *gpui.Timer
	open, editing, busy, dragging bool
	sending                       bool
	background, shot              *image.NRGBA
	editor                        *feedback.Editor
	title, description            *gpui.TextInput
	kinds                         []string
	kind, message                 string
	pending                       *feedback.Report
	revision, pendingRevision     int
	ink                           color.NRGBA
	erase                         bool
	width                         float64
	slider                        SliderState
	imageBounds                   gpui.Bounds
	scroll                        *gpui.ScrollHandle
	focus                         *gpui.FocusHandle
	buttons                       map[string]*gpui.FocusHandle
	focusWanted                   bool
	focusOrder                    []*gpui.FocusHandle
	draftStarted                  bool
}

func (ws *Workspace) feedbackState() *feedbackState {
	if ws.fb == nil {
		ctx, cancel := context.WithCancel(context.Background())
		ws.fb = &feedbackState{
			ctx: ctx, cancel: cancel, results: make(chan feedbackResult, 8),
			kinds: []string{"bug", "improvement", "note"}, kind: "bug",
			width: 5, ink: color.NRGBA{255, 110, 30, 255},
			scroll: gpui.NewScrollHandle(), buttons: map[string]*gpui.FocusHandle{},
			title:       gpui.NewTextInput(i18n.T("ui.feedback.title_placeholder")),
			description: gpui.NewTextInput(i18n.T("ui.feedback.description_placeholder")),
		}
		ws.fb.description.Multiline(6)
	}
	return ws.fb
}

// ConfigureFeedback installs the desktop intake client and retries saved reports at startup.
// A missing URL or key leaves the dialog available without enabling submission.
func (ws *Workspace) ConfigureFeedback(client *feedback.Client) {
	f := ws.feedbackState()
	f.client = client
	if client == nil || !client.Configured() {
		return
	}
	if client.Outbox == nil {
		path, err := settings.Path()
		if err != nil {
			f.message = i18n.T("ui.feedback.storage_unavailable")
			f.client = nil
			return
		}
		client.Outbox = &feedback.Outbox{Dir: filepath.Join(filepath.Dir(path), "feedback-outbox")}
	}
	f.busy = true
	go func() {
		n, err := client.RetryPending(f.ctx)
		msg := ""
		if n > 0 {
			msg = i18n.T("ui.feedback.sent_saved", n)
		}
		f.results <- feedbackResult{operation: msg, err: err}
	}()
}

// ShutdownFeedback cancels background work; unacknowledged reports remain in the private outbox.
func (ws *Workspace) ShutdownFeedback() {
	if ws.fb != nil {
		ws.fb.cancel()
		if ws.fb.poll != nil {
			ws.fb.poll.Cancel()
		}
	}
}

func (ws *Workspace) feedbackButton() *gpui.Div {
	return iconButton("tool-feedback", i18n.T("ui.feedback.tip"), icons.AppFeedback, false, func(w *gpui.Window) { ws.openFeedback(w) })
}

func (ws *Workspace) openFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	f.background = w.Snapshot()
	if !f.draftStarted {
		f.draftStarted = true
		f.shot = feedback.PrepareImage(f.background)
		f.revision++
	}
	f.open = true
	f.focusWanted = true
	ws.pollFeedback(w)
	w.Refresh()
	if f.client.Configured() && !f.busy {
		ws.testFeedback(w)
	}
}

func (ws *Workspace) pollFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	if f.poll != nil {
		f.poll.Cancel()
		f.poll = nil
	}
	for {
		select {
		case r := <-f.results:
			f.busy = false
			f.sending = false
			if r.info != nil {
				f.kinds = r.info.Kinds
				if !slices.Contains(f.kinds, f.kind) {
					f.kind = f.kinds[0]
				}
			}
			if r.shot != nil {
				f.shot = r.shot
				f.revision++
			}
			if r.report != nil {
				f.pending = r.report
				f.pendingRevision = f.revision
			}
			if r.err != nil {
				f.message = r.err.Error()
			} else {
				f.message = r.operation
			}
			if r.receipt.Reference != "" {
				f.message = i18n.T("ui.feedback.receipt", r.receipt.Reference)
				f.pending = nil
				f.draftStarted = false
				f.title.SetText("")
				f.description.SetText("")
				f.shot = nil
				f.revision++
			}
			w.Refresh()
		default:
			if f.busy {
				f.poll = w.After(100*time.Millisecond, func(w *gpui.Window, _ *gpui.App) { ws.pollFeedback(w) })
			}
			return
		}
	}
}

func (ws *Workspace) testFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	if f.busy || !f.client.Configured() {
		return
	}
	f.busy = true
	f.message = i18n.T("ui.feedback.checking")
	go func() {
		info, err := f.client.Test(f.ctx)
		var p *feedback.Info
		if err == nil {
			p = &info
		}
		f.results <- feedbackResult{info: p, err: err, operation: i18n.T("ui.feedback.ready")}
	}()
	ws.pollFeedback(w)
	w.Refresh()
}

func (ws *Workspace) sendFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	if f.busy || !f.client.Configured() {
		return
	}
	title, description, kind := strings.TrimSpace(f.title.Text()), f.description.Text(), f.kind
	if title == "" {
		f.message = i18n.T("ui.feedback.need_title")
		w.Refresh()
		return
	}
	pending := f.pending
	reuse := pending != nil && pending.Title == title && pending.Description == description && pending.Kind == kind && f.pendingRevision == f.revision
	shot := feedback.PrepareImage(f.shot)
	locale := ws.Settings().Language
	version, viewport := ws.Version(), fmt.Sprintf("%dx%d", int(ws.winW), int(ws.winH))
	f.busy = true
	f.sending = true
	f.message = i18n.T("ui.feedback.sending")
	go func() {
		var r feedback.Report
		var err error
		if reuse {
			r = *pending
		} else {
			r.ID, err = feedback.NewID()
			if err == nil {
				r.Screenshot, err = feedback.EncodeScreenshot(shot)
			}
			r.Title, r.Description, r.Kind, r.Version, r.Platform = title, description, kind, version, runtime.GOOS
			r.Context = feedback.Metadata(f.ctx, version, locale, viewport)
			if err == nil {
				err = r.Validate()
			}
			if err == nil && pending != nil {
				err = f.client.Outbox.Save(r)
				if err == nil {
					err = f.client.Outbox.Remove(pending.ID)
				}
			}
		}
		var receipt feedback.Receipt
		var report *feedback.Report
		if err == nil {
			report = &r
			receipt, err = f.client.Deliver(f.ctx, r)
		}
		f.results <- feedbackResult{report: report, receipt: receipt, err: err}
	}()
	ws.pollFeedback(w)
	w.Refresh()
}

func (ws *Workspace) attachFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	dialog := ws.dlg.dialog
	if f.busy || dialog == nil {
		return
	}
	f.busy = true
	f.message = i18n.T("ui.feedback.choose")
	go func() {
		path, err := dialog.Open(f.ctx, i18n.T("ui.feedback.attach_title"), "", []filedialog.Filter{{Name: i18n.T("ui.feedback.images"), Extensions: []string{"png", "jpg", "jpeg"}}})
		var shot *image.NRGBA
		if err == nil && path != "" {
			var file *os.File
			file, err = os.Open(path)
			if err == nil {
				shot, err = feedback.DecodeImage(file)
				file.Close()
			}
		}
		if errors.Is(err, filedialog.ErrCancelled) {
			err = nil
		}
		if err != nil {
			err = errors.New(i18n.T("ui.feedback.import_error"))
		}
		f.results <- feedbackResult{shot: shot, err: err}
	}()
	ws.pollFeedback(w)
	w.Refresh()
}

func (ws *Workspace) clearFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	if f.busy {
		return
	}
	if f.pending != nil && f.client != nil && f.client.Outbox != nil {
		if err := f.client.Outbox.Remove(f.pending.ID); err != nil {
			f.message = err.Error()
			w.Refresh()
			return
		}
	}
	f.pending = nil
	f.draftStarted = true
	f.title.SetText("")
	f.description.SetText("")
	f.shot = nil
	f.revision++
	f.message = ""
	w.Refresh()
}

func (ws *Workspace) closeFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	f.open = false
	f.background = nil
	if focus := ws.input.FocusHandle(); focus != nil {
		w.RequestFocus(focus)
	}
	w.Refresh()
}

func (ws *Workspace) retryFeedback(w *gpui.Window) {
	f := ws.feedbackState()
	if f.busy || !f.client.Configured() {
		return
	}
	f.busy = true
	f.message = i18n.T("ui.feedback.retrying")
	go func() {
		n, err := f.client.RetryPending(f.ctx)
		f.results <- feedbackResult{operation: i18n.T("ui.feedback.sent_saved", n), err: err}
	}()
	ws.pollFeedback(w)
	w.Refresh()
}
