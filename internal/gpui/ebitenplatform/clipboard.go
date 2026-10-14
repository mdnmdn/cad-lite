package ebitenplatform

import (
	"reflect"

	"cad-lite/internal/gpui"
)

// clipResult is a finished clipboard operation, applied on the main loop.
type clipResult struct {
	op      string
	text    string
	err     error
	handler gpui.PlatformInputHandler
	rng     gpui.IntRange
}

// clipWorker runs blocking OS clipboard tools off the ebiten Update loop and
// hands their results back through a channel drained by Update.
type clipWorker struct {
	results chan clipResult
}

func newClipWorker() *clipWorker { return &clipWorker{results: make(chan clipResult, 16)} }

func (c *clipWorker) run(job func() clipResult) {
	go func() { c.results <- job() }()
}

// drain applies every result that has arrived, without blocking.
func (c *clipWorker) drain(apply func(clipResult)) {
	for {
		select {
		case r := <-c.results:
			apply(r)
		default:
			return
		}
	}
}

// startClipboard begins a clipboard shortcut and reports whether it consumed
// the key press. Copy and cut read the selection now, on the main loop; the
// OS tool runs on the worker.
func startClipboard(cw *clipWorker, op string, h gpui.PlatformInputHandler,
	read func() (string, error), write func(string) error) bool {
	switch op {
	case "paste":
		cw.run(func() clipResult {
			text, err := read()
			return clipResult{op: op, text: text, err: err}
		})
		return true
	case "copy", "cut":
		if h == nil {
			return false
		}
		sel, ok := h.SelectedTextRange()
		if !ok || sel.IsNil || sel.Range.Start == sel.Range.End {
			return false
		}
		text, ok := h.TextForRange(sel.Range)
		if !ok {
			return false
		}
		rng := sel.Range
		cw.run(func() clipResult {
			return clipResult{op: op, text: text, err: write(text), handler: h, rng: rng}
		})
		return true
	}
	return false
}

// applyClipboard finishes a clipboard operation on the main loop. Paste is
// delivered as character input so it reaches the focused field through the
// normal dispatch path. Cut deletes the copied text only if the write
// succeeded and the same handler still has the same selection over the same
// text.
func applyClipboard(r clipResult, current gpui.PlatformInputHandler, dispatch func(gpui.PlatformInput)) {
	if r.err != nil {
		return
	}
	switch r.op {
	case "paste":
		text := printableLine(r.text)
		if text == "" {
			return
		}
		dispatch(gpui.KeyDownEvent{
			Keystroke:            gpui.Keystroke{Key: text, KeyChar: &text},
			PreferCharacterInput: true,
		})
	case "cut":
		if !sameHandler(current, r.handler) {
			return
		}
		sel, ok := current.SelectedTextRange()
		if !ok || sel.IsNil || sel.Range != r.rng {
			return
		}
		if t, ok := current.TextForRange(r.rng); !ok || t != r.text {
			return
		}
		rng := r.rng
		current.ReplaceTextInRange(&rng, "")
	}
}

// sameHandler compares handlers without panicking on non-comparable dynamic
// types.
func sameHandler(a, b gpui.PlatformInputHandler) bool {
	if a == nil || b == nil {
		return false
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

func printableLine(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case r >= 0x20 && r != 0x7f:
			out = append(out, r)
		}
	}
	return string(out)
}
