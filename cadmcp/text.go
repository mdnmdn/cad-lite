package cadmcp

import (
	"errors"
	"fmt"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

// MaxTextLen caps a TEXT value in bytes; MaxMTextLen caps an MTEXT value after escaping.
const (
	MaxTextLen  = 1000
	MaxMTextLen = 20000
)

// textArgs are add_text's arguments.
type textArgs struct {
	Value       string
	Point       Point
	SecondPoint *Point `json:"second_point"`
	Height      *float64
	Rotation    *float64
	Justify     string
	Style       string
	WidthFactor *float64 `json:"width_factor"`
	Layer       string
}

// mtextArgs are add_mtext's arguments.
type mtextArgs struct {
	Value      string
	Point      Point
	Height     float64
	Width      *float64
	Attachment string
	Rotation   *float64
	Style      string
	Raw        bool
	Layer      string
}

// checkTextValue refuses an empty value, control characters other than tab, and for TEXT (single
// line, as AutoCAD's TEXT is) line breaks.
func checkTextValue(v string, multiline bool) error {
	if v == "" {
		return errors.New("value must not be empty")
	}
	for _, r := range v {
		switch {
		case r == '\n' || r == '\r':
			if !multiline {
				return errors.New("TEXT is a single line: value must not contain line breaks (use add_mtext, or one add_text per line)")
			}
		case r == '\t':
		case r < 0x20 || r == 0x7f:
			return fmt.Errorf("value must not contain control character %U", r)
		}
	}
	return nil
}

// mtextValue turns value into an MTEXT value: unless raw, doc.MTextEscape escapes the MTEXT
// format characters \ { } so the text shows as typed; line breaks become \P.
func mtextValue(v string, raw bool) string {
	if !raw {
		v = doc.MTextEscape(v)
	}
	return strings.NewReplacer("\r\n", `\P`, "\n", `\P`, "\r", `\P`).Replace(v)
}

func checkHeight(h float64) error {
	if err := inRange(h); err != nil {
		return err
	}
	if h <= 0 {
		return errors.New("height must be positive")
	}
	return nil
}

func checkWidthFactor(w float64) error {
	if err := finite(w); err != nil || w < doc.TextMinWidthFactor || w > doc.TextMaxWidthFactor {
		return fmt.Errorf("width_factor must be %g-%g", float64(doc.TextMinWidthFactor), float64(doc.TextMaxWidthFactor))
	}
	return nil
}

// parseAttachment reads an MTEXT attachment name, TL TC TR ML MC MR BL BC BR, into 1-9.
func parseAttachment(s string) (int, error) {
	h, v, ok := doc.ParseJustify(s)
	if !ok || len(strings.TrimSpace(s)) != 2 || v == doc.TextBaseline {
		return 0, fmt.Errorf("attachment %q is not one of TL, TC, TR, ML, MC, MR, BL, BC, BR", s)
	}
	row := int(doc.TextTop - v) // T 0, M 1, B 2
	return row*3 + int(h) + 1, nil
}

// attachmentName is the inverse of parseAttachment; 0 and out-of-range values read as TL.
func attachmentName(a int) string {
	if a < 1 || a > 9 {
		a = doc.MTextTopLeft
	}
	return string("TMB"[(a-1)/3]) + string("LCR"[(a-1)%3])
}

func isPair(h doc.TextHAlign) bool { return h == doc.TextAligned || h == doc.TextFit }

// textEntity builds a TEXT as AutoCAD's TEXT command places it: point is the insertion point the
// justification refers to, or the first baseline point for Aligned and Fit, whose second_point
// ends the baseline. Aligned derives the height and both derive the rotation from the two points;
// Fit derives the width factor.
func textEntity(a textArgs) (doc.Entity, error) {
	if err := checkTextValue(a.Value, false); err != nil {
		return nil, err
	}
	if len(a.Value) > MaxTextLen {
		return nil, fmt.Errorf("value too long (max %d bytes)", MaxTextLen)
	}
	justify := a.Justify
	if justify == "" {
		justify = "Left"
	}
	h, v, ok := doc.ParseJustify(justify)
	if !ok {
		return nil, fmt.Errorf("justify %q is not an AutoCAD justification (Left, Center, Right, Align, Middle, Fit, TL, TC, TR, ML, MC, MR, BL, BC, BR)", a.Justify)
	}
	pair := isPair(h)
	switch {
	case pair && a.SecondPoint == nil:
		return nil, fmt.Errorf("justify %s needs second_point, the end of the baseline", doc.JustifyName(h, v))
	case !pair && a.SecondPoint != nil:
		return nil, errors.New("second_point is only for justify Align and Fit")
	case pair && a.Rotation != nil:
		return nil, errors.New("Align and Fit take the rotation from point and second_point; omit rotation")
	case h == doc.TextAligned && a.Height != nil:
		return nil, errors.New("Align derives the height from the baseline; omit height")
	case h == doc.TextFit && a.WidthFactor != nil:
		return nil, errors.New("Fit derives the width factor from the baseline; omit width_factor")
	case h != doc.TextAligned && a.Height == nil:
		return nil, errors.New("height is required")
	}
	height, rot, wf := 1.0, 0.0, 1.0
	if a.Height != nil {
		if err := checkHeight(*a.Height); err != nil {
			return nil, err
		}
		height = *a.Height
	}
	if a.Rotation != nil {
		if err := inRange(*a.Rotation); err != nil {
			return nil, err
		}
		rot = radians(*a.Rotation)
	}
	if a.WidthFactor != nil {
		if err := checkWidthFactor(*a.WidthFactor); err != nil {
			return nil, err
		}
		wf = *a.WidthFactor
	}
	t := doc.NewText(a.Point.vec(), height, rot, a.Value)
	t.WidthFactor = wf
	t.Style = a.Style // resolved against the drawing's styles later; "" is the default
	t.SetJustify(h, v)
	if pair {
		if a.SecondPoint.vec().Dist(a.Point.vec()) == 0 {
			return nil, errors.New("point and second_point must differ")
		}
		t.Insert, t.Align = a.Point.vec(), a.SecondPoint.vec()
		lay := t.Layout()
		t.Rotation = lay.Rotation
		t.Height, t.WidthFactor = lay.Height, lay.WidthFactor
	}
	return t, nil
}

// mtextEntity builds an MTEXT placed by its attachment point (default TL) at point.
func mtextEntity(a mtextArgs) (doc.Entity, error) {
	if err := checkTextValue(a.Value, true); err != nil {
		return nil, err
	}
	if err := checkHeight(a.Height); err != nil {
		return nil, err
	}
	width := 0.0
	if a.Width != nil {
		if err := inRange(*a.Width); err != nil || *a.Width < 0 {
			return nil, errors.Join(err, errors.New("width must be >= 0 (0 does not wrap)"))
		}
		width = *a.Width
	}
	att := doc.MTextTopLeft
	if a.Attachment != "" {
		var err error
		if att, err = parseAttachment(a.Attachment); err != nil {
			return nil, err
		}
	}
	value := mtextValue(a.Value, a.Raw)
	if len(value) > MaxMTextLen {
		return nil, fmt.Errorf("value too long (max %d bytes after escaping)", MaxMTextLen)
	}
	t := doc.NewMText(a.Point.vec(), a.Height, width, value)
	t.Attachment = att
	t.Style = a.Style
	if a.Rotation != nil {
		if err := inRange(*a.Rotation); err != nil {
			return nil, err
		}
		t.Rotation = radians(*a.Rotation)
	}
	return t, nil
}

func addText(s *cmdsys.Session, a textArgs) (any, error) {
	return addBuilt(s, a.Layer)(textEntity(a))
}

func addMText(s *cmdsys.Session, a mtextArgs) (any, error) {
	return addBuilt(s, a.Layer)(mtextEntity(a))
}

// updateTextArgs are update_text's arguments; every field but handle is optional.
type updateTextArgs struct {
	Handle      string
	Value       *string
	Raw         bool
	Point       *Point
	SecondPoint *Point `json:"second_point"`
	Height      *float64
	Rotation    *float64
	Justify     *string
	Style       *string
	WidthFactor *float64 `json:"width_factor"`
	Width       *float64
	Attachment  *string
}

// textUpdate is a validated update_text: the changes to make, checked against the entity when
// applied.
type textUpdate struct {
	a          updateTextArgs
	h          doc.TextHAlign
	v          doc.TextVAlign
	attachment int
}

func updateTextEdit(a updateTextArgs) (editOp, error) {
	u := &textUpdate{a: a}
	n := 0
	for _, set := range []bool{a.Value != nil, a.Point != nil, a.SecondPoint != nil, a.Height != nil, a.Rotation != nil, a.Justify != nil, a.Style != nil, a.WidthFactor != nil, a.Width != nil, a.Attachment != nil} {
		if set {
			n++
		}
	}
	if n == 0 {
		return editOp{}, errors.New("give at least one field to change")
	}
	if a.Raw && a.Value == nil {
		return editOp{}, errors.New("raw applies to value")
	}
	if a.Height != nil {
		if err := checkHeight(*a.Height); err != nil {
			return editOp{}, err
		}
	}
	if a.Rotation != nil {
		if err := inRange(*a.Rotation); err != nil {
			return editOp{}, err
		}
	}
	if a.WidthFactor != nil {
		if err := checkWidthFactor(*a.WidthFactor); err != nil {
			return editOp{}, err
		}
	}
	if a.Width != nil {
		if err := inRange(*a.Width); err != nil || *a.Width < 0 {
			return editOp{}, errors.Join(err, errors.New("width must be >= 0 (0 does not wrap)"))
		}
	}
	if a.Justify != nil {
		var ok bool
		if u.h, u.v, ok = doc.ParseJustify(*a.Justify); !ok {
			return editOp{}, fmt.Errorf("justify %q is not an AutoCAD justification (Left, Center, Right, Align, Middle, Fit, TL, TC, TR, ML, MC, MR, BL, BC, BR)", *a.Justify)
		}
	}
	if a.Attachment != nil {
		var err error
		if u.attachment, err = parseAttachment(*a.Attachment); err != nil {
			return editOp{}, err
		}
	}
	return editOp{text: u}, nil
}

// apply changes e, which must be a TEXT or an MTEXT, and reports a field that does not apply to
// it. It changes nothing when it reports an error.
func (u *textUpdate) apply(e doc.Entity) error {
	a := u.a
	switch t := e.(type) {
	case *doc.Text:
		switch {
		case a.Width != nil || a.Attachment != nil:
			return errors.New("width and attachment apply to MTEXT; this is TEXT (use justify)")
		case a.Raw:
			return errors.New("raw applies to MTEXT")
		}
		h, v := t.HAlign, t.VAlign
		if a.Justify != nil {
			h, v = u.h, u.v
		}
		pair := isPair(h)
		switch {
		case a.SecondPoint != nil && !pair:
			return errors.New("second_point is only for justify Align and Fit")
		case pair && a.Rotation != nil:
			return errors.New("Align and Fit take the rotation from their two points")
		case h == doc.TextAligned && a.Height != nil:
			return errors.New("Align derives the height from its two points")
		case h == doc.TextFit && a.WidthFactor != nil:
			return errors.New("Fit derives the width factor from its two points")
		}
		if a.Value != nil {
			if err := checkTextValue(*a.Value, false); err != nil {
				return err
			}
			if len(*a.Value) > MaxTextLen {
				return fmt.Errorf("value too long (max %d bytes)", MaxTextLen)
			}
		}
		c := t.Clone().(*doc.Text)
		if a.Value != nil {
			c.Value = *a.Value
		}
		if a.Style != nil {
			c.Style = *a.Style
		}
		if a.Height != nil {
			c.Height = *a.Height
		}
		if a.WidthFactor != nil {
			c.WidthFactor = *a.WidthFactor
		}
		if a.Rotation != nil {
			c.Rotation = radians(*a.Rotation)
		}
		if a.Justify != nil {
			c.SetJustify(h, v)
		}
		if a.Point != nil {
			c.SetAnchor(a.Point.vec())
		}
		if a.SecondPoint != nil {
			if a.SecondPoint.vec().Dist(c.Insert) == 0 {
				return errors.New("second_point must differ from the first point")
			}
			c.Align = a.SecondPoint.vec()
		}
		*t = *c
	case *doc.MText:
		switch {
		case a.Justify != nil || a.SecondPoint != nil || a.WidthFactor != nil:
			return errors.New("justify, second_point and width_factor apply to TEXT; this is MTEXT (use attachment)")
		}
		value := t.Value
		if a.Value != nil {
			if err := checkTextValue(*a.Value, true); err != nil {
				return err
			}
			if value = mtextValue(*a.Value, a.Raw); len(value) > MaxMTextLen {
				return fmt.Errorf("value too long (max %d bytes after escaping)", MaxMTextLen)
			}
		}
		t.Value = value
		if a.Style != nil {
			t.Style = *a.Style
		}
		if a.Height != nil {
			t.Height = *a.Height
		}
		if a.Width != nil {
			t.Width = *a.Width
		}
		if a.Rotation != nil {
			t.Rotation = radians(*a.Rotation)
		}
		if a.Attachment != nil {
			t.Attachment = u.attachment
		}
		if a.Point != nil {
			t.Insert = a.Point.vec()
		}
	default:
		return fmt.Errorf("entity %s is a %s; update_text needs TEXT or MTEXT", e.Handle(), e.Class())
	}
	return nil
}

// styleResolver resolves style names with doc.FindTextStyle, remembering each answer so a batch
// naming one style many times scans the drawing once.
type styleResolver struct {
	d    *doc.Drawing
	seen map[string]string
}

// resolve returns the drawing's spelling of name ("" is the default style).
func (r *styleResolver) resolve(name string) (string, error) {
	if name == "" {
		return doc.DefaultTextStyle, nil
	}
	if n, ok := r.seen[name]; ok {
		return n, nil
	}
	n, ok := doc.FindTextStyle(r.d, name)
	if !ok {
		return "", fmt.Errorf("text style %q does not exist (known: %s)", name, strings.Join(doc.TextStyles(r.d), ", "))
	}
	if r.seen == nil {
		r.seen = map[string]string{}
	}
	r.seen[name] = n
	return n, nil
}

// resolveStyles fixes the style name of a new text entity, or of an update, to the drawing's
// spelling, refusing an unknown style.
func resolveStyles(r *styleResolver, e doc.Entity, op editOp) error {
	var err error
	switch t := e.(type) {
	case *doc.Text:
		t.Style, err = r.resolve(t.Style)
	case *doc.MText:
		t.Style, err = r.resolve(t.Style)
	}
	if err == nil && op.text != nil && op.text.a.Style != nil {
		var n string
		if n, err = r.resolve(*op.text.a.Style); err == nil {
			op.text.a.Style = &n
		}
	}
	return err
}

// updateText is the single update_text tool: it checks the change on a copy first, so a refused
// update changes nothing, then applies it in one transaction.
func updateText(s *cmdsys.Session, a updateTextArgs) (any, error) {
	op, err := updateTextEdit(a)
	if err != nil {
		return nil, err
	}
	if err := resolveStyles(&styleResolver{d: s.Drawing()}, nil, op); err != nil {
		return nil, err
	}
	hs, err := editable(s, []string{a.Handle})
	if err != nil {
		return nil, err
	}
	if err := op.text.apply(s.Drawing().Entity(hs[0]).Clone().(doc.Entity)); err != nil {
		return nil, err
	}
	if err := withTx(s, func(tx *doc.Txn) error {
		if err := op.apply(tx, hs); err != nil {
			return err
		}
		return checkBounds(s.Drawing(), hs...)
	}); err != nil {
		return nil, err
	}
	return entityInfo(s.Drawing().Entity(hs[0])), nil
}

// textInfo adds the TEXT and MTEXT fields to an entity description.
func textInfo(m map[string]any, e doc.Entity) {
	switch t := e.(type) {
	case *doc.Text:
		lay := t.Layout()
		m["value"], m["height"], m["rotation"] = t.Value, lay.Height, degrees(lay.Rotation)
		m["justify"], m["point"], m["style"], m["width_factor"] = t.Justify(), xy(t.Anchor()), styleName(t.Style), lay.WidthFactor
		if isPair(t.HAlign) {
			m["point"], m["second_point"] = xy(t.Insert), xy(t.Align)
		}
		m["box"] = corners(t.Box())
	case *doc.MText:
		m["value"], m["raw"], m["height"], m["rotation"] = doc.MTextPlain(t.Value), t.Value, t.Height, degrees(t.Rotation)
		m["attachment"], m["point"], m["width"], m["style"] = attachmentName(t.Attachment), xy(t.Insert), t.Width, styleName(t.Style)
		m["box"] = corners(t.Box())
	}
}

func styleName(s string) string {
	if s == "" {
		return doc.DefaultTextStyle
	}
	return s
}

func corners(c [4]geom.Vec2) [][2]float64 {
	out := make([][2]float64, 4)
	for i, p := range c {
		out[i] = xy(p)
	}
	return out
}

// plainText is the text a TEXT or MTEXT shows, for searching; ok is false for other entities.
func plainText(e doc.Entity) (string, bool) {
	switch t := e.(type) {
	case *doc.Text:
		return t.Value, true
	case *doc.MText:
		return doc.MTextPlain(t.Value), true
	}
	return "", false
}
