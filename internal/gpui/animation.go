package gpui

import "time"

// Easing maps normalized animation progress to normalized output.
type Easing func(float32) float32

func animationClamp(p float32) float32 {
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// Linear returns clamped, linear progress.
func Linear(p float32) float32 { return animationClamp(p) }

// EaseIn accelerates quadratically from rest.
func EaseIn(p float32) float32 { p = animationClamp(p); return p * p }

// EaseOut decelerates quadratically toward rest.
func EaseOut(p float32) float32 { p = animationClamp(p); return 1 - (1-p)*(1-p) }

// EaseInOut uses symmetric quadratic acceleration and deceleration.
func EaseInOut(p float32) float32 {
	p = animationClamp(p)
	if p < .5 {
		return 2 * p * p
	}
	return 1 - 2*(1-p)*(1-p)
}

// Animation is a view-owned progress sampler. Keep it across frames and sample
// it in one window; it schedules frames itself while running.
type Animation struct {
	duration              time.Duration
	easing                Easing
	repeat, started, done bool
	start                 time.Time
	last                  float32
	owner                 *Window
}

// NewAnimation creates a finite animation; nil easing uses Linear. Nonpositive
// duration completes immediately on its first sample, even with Repeat.
func NewAnimation(duration time.Duration, easing Easing) *Animation {
	if easing == nil {
		easing = Linear
	}
	return &Animation{duration: duration, easing: easing}
}

// Repeat enables looping for a positive duration. Refresh the owning window
// to wake an idle view; this method does not restart or implicitly redraw.
func (a *Animation) Repeat() *Animation {
	if a != nil {
		a.repeat = true
	}
	return a
}

// Cancel freezes the last sample (zero before first sampling). It is nil-safe.
func (a *Animation) Cancel() {
	if a != nil {
		a.done = true
	}
}

// Restart resets the sampler so its next Progress starts at zero. Refresh the
// owning window to wake an idle view. It is nil-safe and retains Repeat/easing.
func (a *Animation) Restart() {
	if a != nil {
		a.started = false
		a.done = false
		a.last = 0
	}
}

// Done reports completion, cancellation or closure of the sampled window.
// A nil animation is done.
func (a *Animation) Done() bool { return a == nil || a.done || (a.owner != nil && a.owner.closed) }

// Progress samples this frame's timestamp and requests another frame while
// running. Sampling one Animation in different windows panics. It is UI-only.
func (a *Animation) Progress(w *Window) float32 {
	if a == nil {
		return 1
	}
	if a.owner != nil && w != nil && a.owner != w {
		panic("gpui: Animation must be sampled in one window")
	}
	if a.Done() {
		return a.last
	}
	if w == nil {
		return a.last
	}
	a.owner = w
	if a.duration <= 0 {
		a.last = 1
		a.done = true
		return a.last
	}
	now := w.FrameTime()
	if !a.started {
		a.started = true
		a.start = now
	}
	elapsed := now.Sub(a.start)
	if elapsed < 0 {
		elapsed = 0
	}
	var progress float32
	if a.repeat {
		progress = float32(float64(elapsed%a.duration) / float64(a.duration))
	} else {
		progress = float32(float64(elapsed) / float64(a.duration))
		if progress >= 1 {
			a.last = 1
			a.done = true
			return 1
		}
	}
	progress = animationClamp(progress)
	value := a.easing(progress)
	if value != value {
		value = progress
	}
	a.last = animationClamp(value)
	w.RequestNextFrame()
	return a.last
}
