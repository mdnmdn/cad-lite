package ui

import (
	"math"
	"weak"

	"cad-lite/cmdsys"
	"cad-lite/internal/gpui"
)

type hudPosition struct {
	x, y, height float32
	corner       int // 0 top right, 1 top left, 2 bottom left, 3 bottom right
	free         bool
	scroll       *gpui.ScrollHandle
}

type hudDrag struct {
	active  bool
	session weak.Pointer[cmdsys.Session]
	start   gpui.Point
	x, y    float32
}

type hudRect struct{ x, y, w, h float32 }

func (a hudRect) overlaps(b hudRect) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func (ws *Workspace) hudPosition() *hudPosition {
	s := ws.status()
	if s.hudPlaces == nil {
		s.hudPlaces = map[weak.Pointer[cmdsys.Session]]*hudPosition{}
	}
	for k := range s.hudPlaces {
		if k.Value() == nil {
			delete(s.hudPlaces, k)
		}
	}
	k := weak.Make(ws.session)
	if p := s.hudPlaces[k]; p != nil {
		return p
	}
	p := &hudPosition{height: 60, scroll: gpui.NewScrollHandle()}
	s.hudPlaces[k] = p
	return p
}

func (ws *Workspace) hudArea() hudRect {
	b := ws.canvas.bounds
	l, t := ws.rulerInsets()
	return hudRect{float32(b.Origin.X) + l + overlayInset, float32(b.Origin.Y) + t + overlayInset,
		max(0, float32(b.Size.W)-l-2*overlayInset), max(0, float32(b.Size.H)-t-2*overlayInset)}
}

func (ws *Workspace) hudBox(p *hudPosition) hudRect {
	a := ws.hudArea()
	b := hudRect{a.x, a.y, min(hudWidth, a.w), min(p.height, a.h)}
	if p.free {
		b.x, b.y = float32(ws.canvas.bounds.Origin.X)+p.x, float32(ws.canvas.bounds.Origin.Y)+p.y
	} else {
		if p.corner == 0 || p.corner == 3 {
			b.x = a.x + a.w - b.w
		}
		if p.corner >= 2 {
			b.y = a.y + a.h - b.h
		}
	}
	b.x = min(max(b.x, a.x), a.x+a.w-b.w)
	b.y = min(max(b.y, a.y), a.y+a.h-b.h)
	// Clear the top-right zoom column, the top-left viewport label and the bottom-left UCS.
	zoom := hudRect{a.x + a.w - zoomHUDWidth, a.y, zoomHUDWidth, zoomHUDHeight}
	if b.overlaps(zoom) {
		if x := zoom.x - 8 - b.w; x >= a.x {
			b.x = x
		} else {
			b.y = zoom.y + zoom.h + 8
		}
	}
	top, bottom := ws.hudVerticalRange(b.x, b.w)
	b.h = min(b.h, max(0, bottom-top))
	b.y = min(max(b.y, top), max(top, bottom-b.h))
	b.x = min(max(b.x, a.x), a.x+a.w-b.w)
	b.y = min(max(b.y, a.y), a.y+a.h-b.h)
	return b
}

func (ws *Workspace) hudVerticalRange(x, width float32) (top, bottom float32) {
	a := ws.hudArea()
	top, bottom = a.y, a.y+a.h
	if x < a.x+196 {
		top = max(top, a.y+28)
	}
	if x < a.x+72 {
		bottom = min(bottom, a.y+a.h-80)
	}
	if x+width > a.x+a.w-zoomHUDWidth {
		top = max(top, a.y+zoomHUDHeight+8)
	}
	return top, max(top, bottom)
}

func (ws *Workspace) propsDragActive() bool {
	s := ws.status()
	if !s.hud || s.hudDrag.session != weak.Make(ws.session) || ws.textEditor != nil || ws.SettingsOpen() {
		s.hudDrag.active = false
	}
	return s.hudDrag.active
}

func (ws *Workspace) beginPropsDrag(at gpui.Point, w *gpui.Window) {
	p := ws.hudPosition()
	b := ws.hudBox(p)
	p.x, p.y, p.free = b.x-float32(ws.canvas.bounds.Origin.X), b.y-float32(ws.canvas.bounds.Origin.Y), true
	ws.status().hudDrag = hudDrag{true, weak.Make(ws.session), at, p.x, p.y}
	ws.canvas.pressed, ws.canvas.boxing, ws.canvas.panning = false, false, false
	ws.zoom.End(w)
	ws.endDrag(w)
	w.Refresh()
}

func (ws *Workspace) movePropsHUD(at gpui.Point, w *gpui.Window) bool {
	if !ws.propsDragActive() {
		return false
	}
	d := ws.status().hudDrag
	p := ws.hudPosition()
	p.x, p.y = d.x+float32(at.X-d.start.X), d.y+float32(at.Y-d.start.Y)
	w.Refresh()
	return true
}

func (ws *Workspace) endPropsDrag(w *gpui.Window) {
	if !ws.propsDragActive() {
		return
	}
	s := ws.status()
	s.hudDrag.active = false
	p := ws.hudPosition()
	b := ws.hudBox(p)
	best, dist := -1, float64(28)
	for corner := 0; corner < 4; corner++ {
		candidate := *p
		candidate.free, candidate.corner = false, corner
		c := ws.hudBox(&candidate)
		if d := math.Hypot(float64(b.x-c.x), float64(b.y-c.y)); d < dist {
			best, dist = corner, d
		}
	}
	if best >= 0 {
		p.corner, p.free = best, false
	} else {
		p.x, p.y = b.x-float32(ws.canvas.bounds.Origin.X), b.y-float32(ws.canvas.bounds.Origin.Y)
	}
	if w != nil {
		w.Refresh()
	}
}
