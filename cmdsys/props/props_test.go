package props_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/cmdsys/props"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func newSession(t testing.TB) *cmdsys.Session {
	t.Helper()
	v := render.DefaultView(800, 600)
	return cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
}

// add commits es as one step and returns their handles.
func add(t testing.TB, s *cmdsys.Session, es ...doc.Entity) []doc.Handle {
	t.Helper()
	tx, err := s.Drawing().Begin()
	if err != nil {
		t.Fatal(err)
	}
	var hs []doc.Handle
	for _, e := range es {
		hs = append(hs, tx.AddCopy(e))
	}
	tx.Commit()
	return hs
}

func sel(hs ...doc.Handle) props.Target {
	return props.Target{Kind: props.TargetSelection, Handles: hs}
}

func prop(t *testing.T, sh props.Sheet, id string) props.Property {
	t.Helper()
	p, ok := sh.Find(id)
	if !ok {
		t.Fatalf("sheet has no %q: %+v", id, sh.Categories)
	}
	return p
}

func apply(t *testing.T, s *cmdsys.Session, tg props.Target, id string, v props.Value) props.Result {
	t.Helper()
	r, err := props.Apply(s, tg, id, v)
	if err != nil {
		t.Fatalf("Apply %s: %v", id, err)
	}
	return r
}

func line(x0, y0, x1, y1 float64) *doc.Line {
	return &doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer}, Start: geom.V(x0, y0), End: geom.V(x1, y1)}
}

func TestBuildLineRowsKindsAndInheritance(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("WALLS", 3)
	l := line(0, 0, 3, 4)
	l.Layer = "WALLS"
	hs := add(t, s, l)
	sh := props.Build(s, sel(hs...))
	if !sh.Editable || sh.Count != 1 || len(sh.Types) != 1 || sh.Types[0].Class != "LINE" || sh.Types[0].Label != "Line" {
		t.Fatalf("sheet header: %+v", sh)
	}
	want := map[string]struct {
		kind props.Kind
		ro   bool
	}{
		"color": {props.KindColor, false}, "layer": {props.KindLayer, false}, "linetype": {props.KindLinetype, false},
		"ltscale": {props.KindNumber, false}, "lineweight": {props.KindLineweight, false}, "handle": {props.KindText, true},
		"start_x": {props.KindLength, false}, "end_y": {props.KindLength, false}, "delta_x": {props.KindLength, true},
		"length": {props.KindLength, true}, "angle": {props.KindAngle, true},
	}
	for id, w := range want {
		p := prop(t, sh, id)
		if p.Kind != w.kind || p.ReadOnly != w.ro || p.Label != "ui.props.p."+id {
			t.Errorf("%s: kind %v ro %v label %q", id, p.Kind, p.ReadOnly, p.Label)
		}
	}
	if p := prop(t, sh, "length"); p.Value.Num != 5 || p.Display != "5.0000" {
		t.Errorf("length %+v", p)
	}
	c := prop(t, sh, "color")
	if c.Value.Int != doc.ByLayer || c.Source != props.SourceLayer || c.From != "WALLS" || c.Sample.ACI != 3 ||
		c.Display != "ByLayer" || c.Inherited != "From layer WALLS" || c.Clearable {
		t.Errorf("color %+v", c)
	}
	if len(c.Options) < 2 || c.Options[0].Sample.ACI != 3 || c.Options[0].Label != "ByLayer" || c.Options[1].Value.Int != doc.ByBlock {
		t.Errorf("colour options %+v", c.Options[:2])
	}
	lt := prop(t, sh, "linetype")
	if lt.Value.Str != doc.LinetypeByLayer || lt.Sample.Linetype != "CONTINUOUS" || lt.Source != props.SourceLayer {
		t.Errorf("linetype %+v", lt)
	}
	lib := 0
	for _, o := range lt.Options {
		if o.Load {
			lib++
		}
	}
	if lib == 0 {
		t.Error("library linetypes are not offered")
	}
	lw := prop(t, sh, "lineweight")
	if lw.Value.Int != 0 || lw.Sample.Lineweight != 25 || lw.Source != props.SourceLayer {
		t.Errorf("lineweight %+v", lw)
	}
	if h := prop(t, sh, "handle"); h.Value.Str != hs[0].String() {
		t.Errorf("handle %+v", h)
	}
}

func TestVariesAndClassFilter(t *testing.T) {
	s := newSession(t)
	a, b := line(0, 0, 1, 0), line(0, 0, 2, 0)
	b.Color = 1
	c := &doc.Circle{EntityCommon: doc.EntityCommon{Layer: "0", Color: 1}, Center: geom.V(5, 5), Radius: 2}
	hs := add(t, s, a, b, c)
	sh := props.Build(s, sel(hs...))
	if sh.Count != 3 || len(sh.Types) != 2 || sh.Types[0].Count != 2 || sh.Types[1].Count != 1 {
		t.Fatalf("types %+v", sh.Types)
	}
	if p := prop(t, sh, "color"); !p.Value.Varies || p.Display != "*VARIES*" {
		t.Fatalf("colour %+v", p)
	}
	if _, ok := sh.Find("radius"); ok {
		t.Fatal("mixed classes show geometry")
	}
	if p := prop(t, sh, "layer"); p.Value.Varies || p.Value.Str != "0" {
		t.Fatalf("layer %+v", p)
	}
	tg := sel(hs...)
	tg.Class = "CIRCLE"
	sh = props.Build(s, tg)
	if sh.Count != 1 || prop(t, sh, "radius").Value.Num != 2 || prop(t, sh, "color").Value.Int != 1 {
		t.Fatalf("filtered sheet %+v", sh)
	}
	tg.Class = "LINE"
	sh = props.Build(s, tg)
	if p := prop(t, sh, "length"); !p.Value.Varies {
		t.Fatalf("line lengths %+v", p)
	}
	// Apply with the filter changes the lines only.
	apply(t, s, tg, "color", props.IntValue(5))
	if s.Drawing().Entity(hs[2]).Common().Color != 1 || s.Drawing().Entity(hs[0]).Common().Color != 5 {
		t.Fatal("the class filter did not limit the edit")
	}
}

func TestApplyIsOneStepUndoRestoresNoOpRecordsNothing(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	hs := add(t, s, line(0, 0, 1, 0), line(0, 1, 1, 1))
	depth := d.UndoDepth()
	r := apply(t, s, sel(hs...), "color", props.IntValue(2))
	if r.Changed != 2 || !r.Step || d.UndoDepth() != depth+1 || d.UndoName() != "PROPERTIES" {
		t.Fatalf("result %+v depth %d name %q", r, d.UndoDepth(), d.UndoName())
	}
	r = apply(t, s, sel(hs...), "color", props.IntValue(2))
	if r.Changed != 0 || r.Step || d.UndoDepth() != depth+1 {
		t.Fatalf("no-op recorded: %+v", r)
	}
	d.Undo()
	for _, h := range hs {
		if d.Entity(h).Common().Color != doc.ByLayer {
			t.Fatal("undo did not restore the colour")
		}
	}
	// Clear: colour back to ByLayer, linetype scale back to 1.
	apply(t, s, sel(hs...), "ltscale", props.NumValue(3))
	if p := prop(t, props.Build(s, sel(hs...)), "ltscale"); !p.Clearable || p.Value.Num != 3 {
		t.Fatalf("ltscale %+v", p)
	}
	apply(t, s, sel(hs...), "ltscale", props.ClearValue())
	if d.Entity(hs[0]).Common().LtScale() != 1 {
		t.Fatal("clear did not reset the linetype scale")
	}
}

func TestSkipsLockedAndUnknownAndRefusesWhenBusy(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	d.AddLayer("LOCK", 1).Locked = true
	l := line(0, 0, 1, 0)
	l.Layer = "LOCK"
	hs := add(t, s, line(0, 0, 1, 1), l)
	u := &doc.Unknown{Type: "HATCH"}
	u.Layer = "0"
	d.Load(u)
	hs = append(hs, u.Handle(), 0xFFFFF)
	r := apply(t, s, sel(hs...), "color", props.IntValue(4))
	if r.Changed != 1 || r.Skipped != 3 {
		t.Fatalf("result %+v", r)
	}
	if d.Entity(hs[1]).Common().Color == 4 {
		t.Fatal("a locked object changed")
	}
	s.Run("LINE")
	if sh := props.Build(s, sel(hs[0])); sh.Editable {
		t.Fatal("sheet editable while a command runs")
	}
	if _, err := props.Apply(s, sel(hs[0]), "color", props.IntValue(1)); !errors.Is(err, props.ErrBusy) {
		t.Fatalf("busy: %v", err)
	}
	s.Escape()
	tx, _ := d.Begin()
	if _, err := props.Apply(s, sel(hs[0]), "color", props.IntValue(1)); !errors.Is(err, props.ErrBusy) {
		t.Fatalf("open transaction: %v", err)
	}
	tx.Abort()
	if _, err := props.Apply(s, sel(hs[0]), "length", props.NumValue(3)); !errors.Is(err, props.ErrReadOnly) {
		t.Fatalf("read-only: %v", err)
	}
	if _, err := props.Apply(s, sel(hs[0]), "nonsense", props.NumValue(3)); !errors.Is(err, props.ErrInvalid) {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestCircleDerivedEdits(t *testing.T) {
	s := newSession(t)
	hs := add(t, s, &doc.Circle{EntityCommon: doc.EntityCommon{Layer: "0"}, Center: geom.V(0, 0), Radius: 1})
	c := func() *doc.Circle { return s.Drawing().Entity(hs[0]).(*doc.Circle) }
	apply(t, s, sel(hs...), "diameter", props.NumValue(20))
	if c().Radius != 10 {
		t.Fatalf("diameter: r=%v", c().Radius)
	}
	apply(t, s, sel(hs...), "circumference", props.NumValue(2*math.Pi*3))
	if math.Abs(c().Radius-3) > 1e-12 {
		t.Fatalf("circumference: r=%v", c().Radius)
	}
	apply(t, s, sel(hs...), "area", props.NumValue(math.Pi*16))
	if math.Abs(c().Radius-4) > 1e-12 {
		t.Fatalf("area: r=%v", c().Radius)
	}
	if _, err := props.Apply(s, sel(hs...), "radius", props.NumValue(-1)); !errors.Is(err, props.ErrInvalid) {
		t.Fatalf("negative radius: %v", err)
	}
	sh := props.Build(s, sel(hs...))
	if p := prop(t, sh, "area"); p.ReadOnly || !p.Positive {
		t.Fatalf("area row %+v", p)
	}
}

func TestTextEditsMatchTheTextEditor(t *testing.T) {
	s := newSession(t)
	al := doc.NewText(geom.V(0, 0), 2, 0, "ABC")
	al.SetJustify(doc.TextAligned, doc.TextBaseline)
	al.Layer = "0"
	hs := add(t, s, al)
	txt := func() *doc.Text { return s.Drawing().Entity(hs[0]).(*doc.Text) }
	// The text editor's rule for an Aligned text's height: the second point moves so the width
	// follows the new height.
	want := txt().Clone().(*doc.Text)
	lay := want.Layout()
	want.Align = want.Insert.Add(lay.Dir().Scale(doc.TextAdvance(want.Value) * 5 * lay.WidthFactor))
	want.Height = 5
	apply(t, s, sel(hs...), "height", props.NumValue(5))
	if txt().Align != want.Align || txt().Height != 5 {
		t.Fatalf("height: align %v want %v", txt().Align, want.Align)
	}
	apply(t, s, sel(hs...), "justify", props.StrValue("MC"))
	if txt().Justify() != "MC" {
		t.Fatalf("justify %q", txt().Justify())
	}
	apply(t, s, sel(hs...), "contents", props.StrValue("XY"))
	if txt().Value != "XY" {
		t.Fatal("contents")
	}
	if _, err := props.Apply(s, sel(hs...), "contents", props.StrValue("a\nb")); !errors.Is(err, props.ErrInvalid) {
		t.Fatalf("newline in TEXT: %v", err)
	}
	m := doc.NewMText(geom.V(0, 0), 2.5, 0, `one\Ptwo`)
	m.Layer = "0"
	mh := add(t, s, m)
	sh := props.Build(s, sel(mh...))
	if p := prop(t, sh, "contents"); p.Kind != props.KindMultiText || p.Value.Str != "one\ntwo" {
		t.Fatalf("mtext contents %+v", p)
	}
	apply(t, s, sel(mh...), "contents", props.StrValue("a\nb{c}"))
	if v := s.Drawing().Entity(mh[0]).(*doc.MText).Value; v != `a\Pb\{c\}` {
		t.Fatalf("mtext value %q", v)
	}
	apply(t, s, sel(mh...), "justify", props.StrValue("BR"))
	if a := s.Drawing().Entity(mh[0]).(*doc.MText).Attachment; a != doc.MTextBottomRight {
		t.Fatalf("attachment %d", a)
	}
}

func TestInsertAttributes(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	tx, _ := d.Begin()
	def := &doc.AttributeDefinition{Text: *doc.NewText(geom.V(0, 0), 1, 0, "?"), Tag: "ID"}
	if err := tx.DefineBlock(&doc.Block{Name: "TAG", Entities: []doc.Entity{&doc.Circle{Radius: 1}, def}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("TAG", geom.V(10, 10))
	in.Layer = "0"
	h := tx.Add(in)
	tx.Commit()
	if !d.Entity(h).(*doc.Insert).SetAttribute("ID", "A1") {
		t.Fatal("setup attribute")
	}
	sh := props.Build(s, sel(h))
	p := prop(t, sh, "attr:ID")
	if p.Name != "ID" || p.Value.Str != "A1" || p.Kind != props.KindText {
		t.Fatalf("attribute row %+v", p)
	}
	apply(t, s, sel(h), "attr:ID", props.StrValue("B2"))
	if v := d.Entity(h).(*doc.Insert).Attributes[0].Value; v != "B2" {
		t.Fatalf("attribute %q", v)
	}
	apply(t, s, sel(h), "position_x", props.NumValue(3))
	if d.Entity(h).(*doc.Insert).Position.X != 3 {
		t.Fatal("position")
	}
	if _, err := props.Apply(s, sel(h), "scale_x", props.NumValue(0)); !errors.Is(err, props.ErrInvalid) {
		t.Fatalf("zero scale: %v", err)
	}
	if n := prop(t, sh, "name"); n.Value.Str != "TAG" || len(n.Options) != 1 {
		t.Fatalf("name row %+v", n)
	}
}

func dimension(s *cmdsys.Session) *doc.Dimension {
	dm := &doc.Dimension{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer}, Kind: doc.DimRotated,
		Ext1: geom.V(0, 0), Ext2: geom.V(100, 0), DimLine: geom.V(100, -20)}
	dm.SetStyle(s.Drawing(), s.Drawing().ActiveDimStyle())
	dm.Recompute()
	return dm
}

func TestDimensionOverrideClearAndStyleEdit(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	hs := add(t, s, dimension(s), dimension(s))
	dm := func(i int) *doc.Dimension { return d.Entity(hs[i]).(*doc.Dimension) }
	sh := props.Build(s, sel(hs...))
	p := prop(t, sh, "dimasz")
	if p.Source != props.SourceStyle || p.From != "ISO-25" || p.Overridden || p.Clearable || p.Var != "DIMASZ" || p.Value.Num != 2.5 {
		t.Fatalf("following row %+v", p)
	}
	// An override on the first dimension.
	apply(t, s, sel(hs[0]), "dimasz", props.NumValue(5))
	if dm(0).Overrides.Set != doc.DimVarArrowSize || dm(0).Style().ArrowSize != 5 || dm(1).Overrides.Set != 0 {
		t.Fatalf("override: %+v", dm(0).Overrides)
	}
	sh = props.Build(s, sel(hs...))
	if p := prop(t, sh, "dimasz"); !p.Value.Varies || !p.Overridden || !p.Clearable {
		t.Fatalf("mixed row %+v", p)
	}
	// Editing the style through Defaults changes the dimension that follows it, not the override.
	r := apply(t, s, props.Target{Kind: props.TargetDefaults}, "dimasz", props.NumValue(4))
	if !r.Step || d.ActiveDimStyle().ArrowSize != 4 || s.Vars().Real("DIMASZ") != 4 {
		t.Fatalf("style edit %+v", r)
	}
	if dm(1).Style().ArrowSize != 4 || dm(0).Style().ArrowSize != 5 {
		t.Fatalf("after the style edit: %v %v", dm(0).Style().ArrowSize, dm(1).Style().ArrowSize)
	}
	// Clearing returns the first to the style.
	apply(t, s, sel(hs[0]), "dimasz", props.ClearValue())
	if dm(0).Overrides.Set != 0 || dm(0).Style().ArrowSize != 4 {
		t.Fatalf("clear: %+v", dm(0).Overrides)
	}
	// A value equal to the style's is no override.
	apply(t, s, sel(hs[0]), "dimasz", props.NumValue(4))
	if dm(0).Overrides.Set != 0 {
		t.Fatal("setting the style's value made an override")
	}
	// Arrow 1 makes the arrows separate; setting it back joins them.
	apply(t, s, sel(hs[0]), "arrow1", props.StrValue("_ArchTick"))
	if a, b := dm(0).Style().ArrowBlocks(); a != "_ArchTick" || b != "" || !dm(0).Style().SeparateArrows {
		t.Fatalf("arrow1: %q %q", a, b)
	}
	apply(t, s, sel(hs[0]), "arrow1", props.StrValue(""))
	if dm(0).Style().SeparateArrows {
		t.Fatal("equal arrows stay separate")
	}
	if p := prop(t, props.Build(s, sel(hs[0])), "arrow1"); !p.Sample.HasArrow || len(p.Options) != int(doc.ArrowUser) {
		t.Fatalf("arrow row %+v", p)
	}
	d.Undo()
	if !dm(0).Style().SeparateArrows {
		t.Fatal("undo of the arrow edit")
	}
}

func TestLayerEditsUndoableAndFreezeCurrentRefused(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	d.AddLayer("A", 1)
	tg := props.Target{Kind: props.TargetLayer, Layer: "A"}
	sh := props.Build(s, tg)
	if sh.Title != "Layer: A" || prop(t, sh, "name").ReadOnly != true || prop(t, sh, "color").Value.Int != 1 {
		t.Fatalf("layer sheet %+v", sh)
	}
	r := apply(t, s, tg, "linetype", props.StrValue("dashed"))
	if !r.Step || d.Layer("A").Linetype != "DASHED" {
		t.Fatalf("linetype: %+v %q", r, d.Layer("A").Linetype)
	}
	if _, ok := d.Linetype("DASHED"); !ok {
		t.Fatal("library linetype not loaded")
	}
	apply(t, s, tg, "frozen", props.BoolValue(true))
	apply(t, s, tg, "color", props.IntValue(5))
	if l := d.Layer("A"); !l.Frozen || l.Color != 5 {
		t.Fatal("frozen/colour")
	}
	d.Undo()
	d.Undo()
	if l := d.Layer("A"); l.Frozen || l.Color != 1 {
		t.Fatalf("undo: %+v", *l)
	}
	d.Undo()
	if _, ok := d.Linetype("DASHED"); ok || d.Layer("A").Linetype != "" {
		t.Fatal("undo did not unload the linetype with the layer edit")
	}
	cur := props.Target{Kind: props.TargetLayer, Layer: "0"}
	if p := prop(t, props.Build(s, cur), "frozen"); !p.ReadOnly {
		t.Fatal("current layer's frozen row editable")
	}
	if _, err := props.Apply(s, cur, "frozen", props.BoolValue(true)); !errors.Is(err, props.ErrRefused) {
		t.Fatalf("freeze current: %v", err)
	}
	if _, err := props.Apply(s, props.Target{Kind: props.TargetLayer, Layer: "GONE"}, "on", props.BoolValue(false)); !errors.Is(err, props.ErrStale) {
		t.Fatalf("missing layer: %v", err)
	}
	if r := apply(t, s, tg, "lineweight", props.IntValue(int(doc.LineweightDefault))); r.Step {
		t.Fatal("no-op layer edit recorded a step")
	}
}

func TestDefaultsAreUndoableAndAgreeWithSETVAR(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	def := props.Target{Kind: props.TargetDefaults}
	sh := props.Build(s, def)
	if p := prop(t, sh, "color"); p.Var != "CECOLOR" || p.Value.Int != doc.ByLayer || p.NotUndoable {
		t.Fatalf("color %+v", p)
	}
	if p := prop(t, sh, "textsize"); !p.NotUndoable {
		t.Fatalf("textsize %+v", p)
	}
	apply(t, s, def, "color", props.IntValue(doc.ByBlock))
	if s.Vars().Text("CECOLOR") != "BYBLOCK" || d.CurrentColor != doc.ByBlock {
		t.Fatal("CECOLOR")
	}
	apply(t, s, def, "lineweight", props.IntValue(30))
	if s.Vars().Int("CELWEIGHT") != 30 {
		t.Fatal("CELWEIGHT")
	}
	apply(t, s, def, "linetype", props.StrValue("HIDDEN"))
	if s.Vars().Text("CELTYPE") != "HIDDEN" {
		t.Fatalf("CELTYPE %q", s.Vars().Text("CELTYPE"))
	}
	apply(t, s, def, "global_ltscale", props.NumValue(2))
	if s.Vars().Real("LTSCALE") != 2 || d.LTScale != 2 {
		t.Fatal("LTSCALE")
	}
	d.Undo()
	d.Undo()
	if d.LTScaleValue() != 1 || s.Vars().Text("CELTYPE") != "BYLAYER" {
		t.Fatal("undo of LTSCALE and CELTYPE")
	}
	// SETVAR and the sheet show one value.
	s.Script("CECOLOR 3\n")
	if p := prop(t, props.Build(s, def), "color"); p.Value.Int != 3 {
		t.Fatalf("after SETVAR: %+v", p)
	}
	d.Undo()
	if d.CurrentColor != doc.ByBlock {
		t.Fatalf("SETVAR is undoable: %d", d.CurrentColor)
	}
	if p := prop(t, sh, "insunits"); p.NotUndoable {
		t.Fatalf("insunits %+v", p)
	}
	if r := apply(t, s, def, "insunits", props.IntValue(6)); !r.Step || r.NotUndoable || d.Header.InsUnits != 6 {
		t.Fatalf("insunits apply: %+v header %d", r, d.Header.InsUnits)
	}
	d.Undo()
	if d.Header.InsUnits != 4 || s.Vars().Int("INSUNITS") != 4 {
		t.Fatalf("undo of INSUNITS: %d", d.Header.InsUnits)
	}
	r := apply(t, s, def, "textsize", props.NumValue(5))
	if !r.NotUndoable || s.Vars().Real("TEXTSIZE") != 5 {
		t.Fatalf("textsize %+v", r)
	}
	if r := apply(t, s, def, "color", props.IntValue(doc.ByBlock)); r.Step {
		t.Fatal("no-op default recorded a step")
	}
	if _, err := props.Apply(s, def, "layer", props.StrValue("NOPE")); !errors.Is(err, props.ErrInvalid) {
		t.Fatalf("missing layer: %v", err)
	}
}

func TestByBlockSurvives(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	s.Script("CECOLOR 1\nCELWEIGHT 50\n")
	hs := add(t, s, line(0, 0, 1, 0))
	apply(t, s, sel(hs...), "color", props.IntValue(doc.ByBlock))
	apply(t, s, sel(hs...), "lineweight", props.IntValue(int(doc.LineweightByBlock)))
	apply(t, s, sel(hs...), "linetype", props.StrValue("BYBLOCK"))
	c := d.Entity(hs[0]).Common()
	if c.Color != doc.ByBlock || c.Lineweight != doc.LineweightByBlock || c.Linetype != doc.LinetypeByBlock {
		t.Fatalf("ByBlock lost: %+v", *c)
	}
	apply(t, s, sel(hs...), "ltscale", props.NumValue(2))
	if c := d.Entity(hs[0]).Common(); c.Color != doc.ByBlock || c.Lineweight != doc.LineweightByBlock {
		t.Fatalf("another edit resolved ByBlock: %+v", *c)
	}
	p := prop(t, props.Build(s, sel(hs...)), "color")
	if p.Source != props.SourceBlock || p.Display != "ByBlock" || p.Sample.ACI != 7 {
		t.Fatalf("ByBlock row %+v", p)
	}
	// ApplyTx (the engine of cadmcp's planned update_entity) keeps it too.
	tx, _ := d.Begin()
	if _, err := props.ApplyTx(tx, d, hs, "layer", props.StrValue("0")); err != nil {
		t.Fatal(err)
	}
	if _, err := props.ApplyTx(tx, d, hs, "start_x", props.NumValue(-1)); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if c := d.Entity(hs[0]).Common(); c.Color != doc.ByBlock || d.Entity(hs[0]).(*doc.Line).Start.X != -1 {
		t.Fatal("ApplyTx")
	}
}

func TestParseUnitsAndDecimalComma(t *testing.T) {
	s := newSession(t)
	length := props.Property{Kind: props.KindLength}
	for in, want := range map[string]float64{"2,5": 2.5, "2.5": 2.5, " -3 ": -3} {
		if v, err := props.Parse(s, length, in); err != nil || v.Num != want {
			t.Errorf("%q: %v %v", in, v, err)
		}
	}
	s.Script("LUNITS 4\n")
	for in, want := range map[string]float64{`1'-3 1/2"`: 15.5, `2'`: 24, `3-1/2"`: 3.5, `15 1/2`: 15.5} {
		if v, err := props.Parse(s, length, in); err != nil || v.Num != want {
			t.Errorf("%q: %v %v", in, v, err)
		}
	}
	angle := props.Property{Kind: props.KindAngle}
	if v, err := props.Parse(s, angle, "45,5"); err != nil || math.Abs(v.Num-45.5*math.Pi/180) > 1e-12 {
		t.Errorf("angle: %v %v", v, err)
	}
	if _, err := props.Parse(s, props.Property{Kind: props.KindNumber, Positive: true}, "0"); !errors.Is(err, props.ErrInvalid) {
		t.Errorf("positive: %v", err)
	}
	if v, err := props.Parse(s, props.Property{Kind: props.KindColor}, "bybLock"); err != nil || v.Int != doc.ByBlock {
		t.Errorf("colour: %v %v", v, err)
	}
	if v, err := props.Parse(s, props.Property{Kind: props.KindLineweight}, "0,30 mm"); err != nil || v.Int != 30 {
		t.Errorf("lineweight: %v %v", v, err)
	}
	if v, err := props.Parse(s, props.Property{Kind: props.KindInt, Selector: true}, "3"); err != nil || v.Int != 2 {
		t.Errorf("vertex: %v %v", v, err)
	}
	if _, err := props.Parse(s, props.Property{Kind: props.KindLength}, "abc"); err == nil || !strings.Contains(err.Error(), "number") {
		t.Errorf("garbage: %v", err)
	}
}

func TestPolylineVertexRows(t *testing.T) {
	s := newSession(t)
	p := &doc.LWPolyline{EntityCommon: doc.EntityCommon{Layer: "0"}, Points: []geom.Vec2{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}}
	hs := add(t, s, p)
	tg := sel(hs...)
	tg.Vertex = 9
	sh := props.Build(s, tg)
	if sh.Vertices != 3 || sh.Target.Vertex != 2 || prop(t, sh, "vertex").Display != "3" || prop(t, sh, "vertex_y").Value.Num != 10 {
		t.Fatalf("vertex rows %+v", sh)
	}
	tg.Vertex = 1
	apply(t, s, tg, "vertex_x", props.NumValue(7))
	apply(t, s, tg, "global_width", props.NumValue(0.5))
	got := s.Drawing().Entity(hs[0]).(*doc.LWPolyline)
	if got.Points[1].X != 7 || len(got.Widths) != 3 || got.Widths[2] != [2]float64{0.5, 0.5} {
		t.Fatalf("polyline %+v", got)
	}
	if p := prop(t, props.Build(s, tg), "length"); !p.ReadOnly || p.Value.Num <= 0 {
		t.Fatalf("length %+v", p)
	}
}

func BenchmarkBuild100k(b *testing.B) {
	s := newSession(b)
	es := make([]doc.Entity, 100000)
	for i := range es {
		es[i] = line(float64(i), 0, float64(i), 1)
	}
	hs := add(b, s, es...)
	tg := sel(hs...)
	b.ResetTimer()
	for b.Loop() {
		props.Build(s, tg)
	}
}

// A selection large enough to be read in parallel parts aggregates as a small one does: a value
// that differs only in the last part varies, and a part's own value makes the row clearable.
func TestLargeSelectionMergesParts(t *testing.T) {
	s := newSession(t)
	es := make([]doc.Entity, 40000)
	for i := range es {
		es[i] = line(0, 0, 1, 0)
	}
	es[len(es)-1].Common().Color = 1
	es[len(es)-1].(*doc.Line).End = geom.V(2, 0)
	hs := add(t, s, es...)
	sh := props.Build(s, sel(hs...))
	if sh.Count != 40000 || sh.Types[0].Count != 40000 {
		t.Fatalf("counts %+v", sh.Types)
	}
	if p := prop(t, sh, "color"); !p.Value.Varies || !p.Clearable {
		t.Fatalf("colour %+v", p)
	}
	if p := prop(t, sh, "length"); !p.Value.Varies {
		t.Fatalf("length %+v", p)
	}
	if p := prop(t, sh, "start_y"); p.Value.Varies || p.Value.Num != 0 {
		t.Fatalf("start_y %+v", p)
	}
	if p := prop(t, sh, "layer"); p.Value.Varies || p.Value.Str != "0" {
		t.Fatalf("layer %+v", p)
	}
}

// TestDefaultsLWDEFAULTRow checks the Defaults sheet edits LWDEFAULT, with a lineweight sample and
// only widths on offer.
func TestDefaultsLWDEFAULTRow(t *testing.T) {
	s := newSession(t)
	def := props.Target{Kind: props.TargetDefaults}
	p := prop(t, props.Build(s, def), "lwdefault")
	if p.Var != "LWDEFAULT" || p.Kind != props.KindLineweight || p.Value.Int != 25 || p.Sample.Lineweight != 25 || p.Label != "ui.props.p.lwdefault" {
		t.Fatalf("lwdefault %+v", p)
	}
	for _, o := range p.Options {
		if w := doc.Lineweight(o.Value.Int); w == doc.LineweightByLayer || w == doc.LineweightByBlock || w == doc.LineweightDefault {
			t.Fatalf("option %+v", o)
		}
	}
	apply(t, s, def, "lwdefault", props.IntValue(70))
	if s.Drawing().LWDefaultValue() != 70 || s.Vars().Int("LWDEFAULT") != 70 {
		t.Fatalf("LWDEFAULT %d", s.Vars().Int("LWDEFAULT"))
	}
	if _, err := props.Apply(s, def, "lwdefault", props.IntValue(int(doc.LineweightDefault))); err == nil {
		t.Fatal("Default accepted as LWDEFAULT")
	}
	s.Drawing().Undo()
	if s.Drawing().LWDefaultValue() != 25 {
		t.Fatal("undo")
	}
}

// TestLayerLineweightZero checks the layer sheet offers 0.00 mm and stores it as
// LineweightZero.LayerValue(), apart from Default.
func TestLayerLineweightZero(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("A", 1)
	tg := props.Target{Kind: props.TargetLayer, Layer: "A"}
	p := prop(t, props.Build(s, tg), "lineweight")
	offered := false
	for _, o := range p.Options {
		offered = offered || doc.Lineweight(o.Value.Int) == doc.LineweightZero
	}
	if !offered {
		t.Fatal("0.00 mm not offered")
	}
	apply(t, s, tg, "lineweight", props.IntValue(int(doc.LineweightZero)))
	if got := s.Drawing().Layer("A").Lineweight; got != doc.LineweightZero.LayerValue() {
		t.Fatalf("layer lineweight %d", got)
	}
	if p := prop(t, props.Build(s, tg), "lineweight"); doc.Lineweight(p.Value.Int) != doc.LineweightZero || p.Display == "Default" {
		t.Fatalf("row %+v", p)
	}
	apply(t, s, tg, "lineweight", props.IntValue(int(doc.LineweightDefault)))
	if s.Drawing().Layer("A").Lineweight != 0 {
		t.Fatal("Default")
	}
}
