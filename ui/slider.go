package ui

import (
	"math"

	"go-draw/internal/gpui"
)

// Slider geometry in logical pixels.
const (
	sliderHeight = 24
	sliderTrack  = 4
	sliderThumb  = 16
)

// SliderState is the part of a slider that outlives a frame: the painted
// track bounds and the drag in progress. The owner keeps one per slider and
// passes it to NewSlider every frame. A host that wants the drag to continue
// when the pointer leaves the slider forwards its own mouse-move and mouse-up
// events to Move and End.
type SliderState struct {
	track    gpui.Bounds
	dragging bool
	value    float64
	min, max float64
	step     float64
	onChange func(float64, *gpui.Window)
	onCommit func(float64, *gpui.Window)
}

// Dragging reports whether the thumb is being dragged.
func (s *SliderState) Dragging() bool { return s.dragging }

// Move continues a drag with the pointer at p (window coordinates). It does
// nothing when no drag is in progress.
func (s *SliderState) Move(p gpui.Point, w *gpui.Window) {
	if s.dragging {
		s.setFromX(float64(p.X), w)
	}
}

// End finishes a drag and, when one was in progress, reports the final value
// to OnCommit.
func (s *SliderState) End(w *gpui.Window) {
	if !s.dragging {
		return
	}
	s.dragging = false
	if s.onCommit != nil {
		s.onCommit(s.value, w)
	}
}

// SliderOptions configures NewSlider.
type SliderOptions struct {
	// Value is the current value; Min and Max bound it and Step is the snap
	// granularity (Step <= 0 means continuous).
	Value, Min, Max, Step float64
	// OnChange is called with the snapped value whenever a click, drag or key
	// press changes it. The owner stores it and the next frame shows it.
	OnChange func(v float64, w *gpui.Window)
	// OnCommit is called with the final value when a drag ends.
	OnCommit func(v float64, w *gpui.Window)
}

// SnapSlider returns v clamped to [min, max] and rounded to the nearest
// multiple of step counted from min.
func SnapSlider(v, min, max, step float64) float64 {
	v = math.Min(math.Max(v, min), max)
	if step > 0 {
		v = min + math.Round((v-min)/step)*step
		v = math.Round(v*1e9) / 1e9
		v = math.Min(math.Max(v, min), max)
	}
	return v
}

// NewSlider returns a horizontal slider: a track with a filled part and a
// thumb. Click or drag sets the value, snapped to Step; when focused (Tab or
// click) Left/Down and Right/Up move by one step, PageDown/PageUp by ten, Home
// and End jump to the ends. id must be unique and stable among elements. The
// returned div has no width: give it one or Flex1.
func NewSlider(id string, st *SliderState, o SliderOptions) *gpui.Div {
	st.min, st.max, st.step, st.onChange, st.onCommit = o.Min, o.Max, o.Step, o.OnChange, o.OnCommit
	st.value = SnapSlider(o.Value, o.Min, o.Max, o.Step)
	frac := 0.0
	if o.Max > o.Min {
		frac = (st.value - o.Min) / (o.Max - o.Min)
	}
	paint := func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		st.track = b
		x0 := float32(b.Origin.X) + sliderThumb/2
		span := float32(b.Size.W) - sliderThumb
		cy := float32(b.Origin.Y) + float32(b.Size.H)/2
		quad := func(x, y, wd, h, r float32, c gpui.Rgba) {
			w.PaintQuad(gpui.Quad{
				Bounds:     gpui.NewBounds(x, y, wd, h),
				Background: gpui.BackgroundSolid, Fill: c,
				CornerRadii: gpui.Corners{}.Uniform(r),
			})
		}
		quad(x0, cy-sliderTrack/2, span, sliderTrack, sliderTrack/2, uiBorder)
		quad(x0, cy-sliderTrack/2, span*float32(frac), sliderTrack, sliderTrack/2, uiAccent)
		tx := x0 + span*float32(frac) - sliderThumb/2
		quad(tx-1, cy-sliderThumb/2-1, sliderThumb+2, sliderThumb+2, sliderThumb/2+1, uiBorder)
		quad(tx, cy-sliderThumb/2, sliderThumb, sliderThumb, sliderThumb/2, uiText)
	}
	return gpui.NewDiv().ID(id).SizeH(sliderHeight).Rounded(4).TabIndex(0).
		FocusVisible(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(1, uiAccent) }).
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button != gpui.MouseLeft {
				return
			}
			st.dragging = true
			st.setFromX(float64(e.Position.X), w)
		}).
		OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) { st.Move(e.Position, w) }).
		OnMouseUp(func(e *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button == gpui.MouseLeft {
				st.End(w)
			}
		}).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
			ten := math.Max(st.step, (st.max-st.min)/100) * 10
			s := math.Max(st.step, (st.max-st.min)/100)
			switch e.Keystroke.Key {
			case "left", "down":
				st.set(st.value-s, w)
			case "right", "up":
				st.set(st.value+s, w)
			case "pagedown":
				st.set(st.value-ten, w)
			case "pageup":
				st.set(st.value+ten, w)
			case "home":
				st.set(st.min, w)
			case "end":
				st.set(st.max, w)
			default:
				return
			}
			w.StopPropagation()
		}).
		Child(gpui.NewCanvas(paint))
}

// setFromX maps a window x coordinate on the track to a value.
func (s *SliderState) setFromX(x float64, w *gpui.Window) {
	x0 := float64(s.track.Origin.X) + sliderThumb/2
	span := float64(s.track.Size.W) - sliderThumb
	if span <= 0 {
		return
	}
	f := math.Min(math.Max((x-x0)/span, 0), 1)
	s.set(s.min+f*(s.max-s.min), w)
}

// set snaps v and reports it when it differs from the current value.
func (s *SliderState) set(v float64, w *gpui.Window) {
	v = SnapSlider(v, s.min, s.max, s.step)
	if v == s.value {
		return
	}
	s.value = v
	if s.onChange != nil {
		s.onChange(v, w)
	}
	w.Refresh()
}
