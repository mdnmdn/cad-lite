package ui

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

type textEditState struct {
	tab         *docTab
	drawing     *doc.Drawing
	original    doc.Entity
	fields      map[string]*gpui.TextInput
	initial     map[string]string
	scroll      *gpui.ScrollHandle
	focusWanted bool
	err         string
	buttons     map[string]*gpui.FocusHandle
}

func textEditTarget(d *doc.Drawing, h doc.Handle) doc.Entity {
	e := d.Entity(h)
	if e == nil || !d.EntityEditable(e) || e.OwnerHandle() != doc.ModelSpace {
		return nil
	}
	switch e.(type) {
	case *doc.Text, *doc.MText:
		return e
	}
	return nil
}

func textEditNumber(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func (ws *Workspace) openTextEditor(t *docTab, h doc.Handle) bool {
	if t != ws.docTab || ws.win == nil || ws.textEditor != nil {
		return false
	}
	d := t.session.Drawing()
	e := textEditTarget(d, h)
	if e == nil {
		return false
	}
	ws.zoom.End(ws.win)
	ws.endDrag(ws.win)
	s := &textEditState{tab: t, drawing: d, original: e.Clone().(doc.Entity),
		fields: map[string]*gpui.TextInput{}, initial: map[string]string{}, buttons: map[string]*gpui.FocusHandle{}, scroll: gpui.NewScrollHandle(), focusWanted: true}
	var height, rotation, width float64
	var value, justify, style string
	switch e := e.(type) {
	case *doc.Text:
		lay := e.Layout()
		height, rotation, value, justify, style = lay.Height, lay.Rotation, e.Value, e.Justify(), e.Style
	case *doc.MText:
		height, rotation, width, value, style = e.Height, e.Rotation, e.Width, doc.MTextPlain(e.Value), e.Style
		a := e.Attachment
		if a < 1 || a > 9 {
			a = 1
		}
		justify = string("TMB"[(a-1)/3]) + string("LCR"[(a-1)%3])
	}
	if style == "" {
		style = doc.DefaultTextStyle
	}
	values := map[string]string{"content": value, "height": textEditNumber(height), "rotation": textEditNumber(rotation * 180 / math.Pi),
		"justify": justify, "style": style, "width": textEditNumber(width)}
	for name, value := range values {
		f := themeField(gpui.NewTextInput(""))
		f.SetText(value)
		f.OnEscape(func(w *gpui.Window, _ *gpui.App) { ws.closeTextEditor(w) })
		if name == "content" {
			if _, ok := e.(*doc.MText); ok {
				f.Multiline(6)
			}
		}
		s.fields[name], s.initial[name] = f, value
	}
	ws.closeLayerPopover(ws.win)
	ws.recent.menu, ws.cmd.open, ws.cmd.ac.open = false, false, false
	ws.canvas.pressed, ws.canvas.boxing, ws.canvas.panning = false, false, false
	ws.textEditor = s
	ws.win.Refresh()
	return true
}

func (ws *Workspace) beginTextEditAt(p gpui.Point, w *gpui.Window) bool {
	h, ok := cmdsys.Pick(ws.session.Drawing(), ws.session.Index(), ws.model(p), ws.session.Aperture*ws.view.PixelSize())
	return ok && ws.openTextEditor(ws.docTab, h)
}

func (ws *Workspace) closeTextEditor(w *gpui.Window) {
	if ws.textEditor == nil {
		return
	}
	ws.textEditor = nil
	if w != nil {
		if fh := ws.input.FocusHandle(); fh != nil {
			w.RequestFocus(fh)
		}
		w.Refresh()
	}
}

func (ws *Workspace) validateTextEditor(w *gpui.Window) {
	if s := ws.textEditor; s != nil && (s.tab != ws.docTab || s.drawing != ws.session.Drawing() || ws.session.Active() != "") {
		ws.closeTextEditor(w)
	}
}

// sameTextEditEntity ignores derived layout caches; reading or painting text cannot make a
// draft stale, but changing any persisted field does.
func sameTextEditEntity(a, b doc.Entity) bool {
	if a == nil || b == nil || reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}
	x, y := reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem()
	for i := 0; i < x.NumField(); i++ {
		if x.Type().Field(i).IsExported() && !reflect.DeepEqual(x.Field(i).Interface(), y.Field(i).Interface()) {
			return false
		}
	}
	return true
}

func (s *textEditState) changed(name string) bool { return s.fields[name].Text() != s.initial[name] }

func (s *textEditState) number(name string, positive bool) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s.fields[name].Text()), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > doc.TextMaxSize || positive && v <= 0 || name == "width" && v < 0 {
		s.err = i18n.T("ui.textedit.invalid_number", i18n.T("ui.textedit."+name))
		return 0, false
	}
	return v, true
}

func (s *textEditState) entity() doc.Entity {
	value := s.fields["content"].Text()
	_, multi := s.original.(*doc.MText)
	for _, r := range value {
		if !multi && (r == '\n' || r == '\r') || r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0x7f {
			s.err = i18n.T("ui.textedit.invalid_content")
			return nil
		}
	}
	height, ok := s.number("height", true)
	if !ok {
		return nil
	}
	rotation, ok := s.number("rotation", false)
	if !ok {
		return nil
	}
	h, v, ok := doc.ParseJustify(s.fields["justify"].Text())
	if !ok || multi && (v == doc.TextBaseline || len(strings.TrimSpace(s.fields["justify"].Text())) != 2) {
		s.err = i18n.T("ui.textedit.invalid_justify")
		return nil
	}
	style, ok := doc.FindTextStyle(s.drawing, s.fields["style"].Text())
	if !ok {
		s.err = i18n.T("ui.textedit.invalid_style")
		return nil
	}
	rotation = geom.NormAngle(rotation * math.Pi / 180)
	next := s.original.Clone().(doc.Entity)
	switch e := next.(type) {
	case *doc.Text:
		if s.changed("justify") {
			e.SetJustify(h, v)
		}
		if s.changed("content") {
			e.Value = value
		}
		pair := e.HAlign == doc.TextAligned || e.HAlign == doc.TextFit
		if s.changed("height") {
			if e.HAlign == doc.TextAligned {
				lay := e.Layout()
				e.Align = e.Insert.Add(lay.Dir().Scale(doc.TextAdvance(e.Value) * height * lay.WidthFactor))
			}
			e.Height = height
		}
		if s.changed("rotation") {
			if pair {
				e.Align = e.Insert.Add(geom.V(math.Cos(rotation), math.Sin(rotation)).Scale(e.Insert.Dist(e.Align)))
			}
			e.Rotation = rotation
		}
		if s.changed("style") {
			e.Style = style
		}
	case *doc.MText:
		width, ok := s.number("width", false)
		if !ok {
			return nil
		}
		if s.changed("content") {
			e.Value = strings.NewReplacer("\r\n", `\P`, "\r", `\P`, "\n", `\P`).Replace(doc.MTextEscape(value))
		}
		if s.changed("height") {
			e.Height = height
		}
		if s.changed("rotation") {
			e.Rotation = rotation
		}
		if s.changed("width") {
			e.Width = width
		}
		if s.changed("justify") {
			e.Attachment = int(doc.TextTop-v)*3 + int(h) + 1
		}
		if s.changed("style") {
			e.Style = style
		}
	}
	return next
}

func (ws *Workspace) applyTextEditor(w *gpui.Window) {
	s := ws.textEditor
	if s == nil {
		return
	}
	stale := func() { s.err = i18n.T("ui.textedit.stale"); w.Refresh() }
	if s.tab != ws.docTab || s.drawing != ws.session.Drawing() || ws.session.Active() != "" {
		stale()
		return
	}
	h := s.original.Handle()
	if e := textEditTarget(s.drawing, h); !sameTextEditEntity(s.original, e) {
		stale()
		return
	}
	changed := false
	for name := range s.fields {
		changed = changed || s.changed(name)
	}
	if !changed {
		ws.closeTextEditor(w)
		return
	}
	next := s.entity()
	if next == nil {
		w.Refresh()
		return
	}
	if sameTextEditEntity(s.original, next) {
		ws.closeTextEditor(w)
		return
	}
	tx, err := s.drawing.Begin()
	if err != nil {
		s.err = i18n.T("ui.textedit.busy")
		w.Refresh()
		return
	}
	tx.Modify(h, func(e doc.Entity) {
		switch e := e.(type) {
		case *doc.Text:
			*e = *next.(*doc.Text)
		case *doc.MText:
			*e = *next.(*doc.MText)
		}
	})
	tx.Commit()
	s.drawing.NameLastStep("TEXTEDIT")
	ws.closeTextEditor(w)
}
