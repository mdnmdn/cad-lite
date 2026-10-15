package doc_test

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"fmt"
	"math"
	"testing"
)

func TestMetadataUndoAbortGrouped(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(1, 0)})
	tx.Commit()
	d.BeginGroup()
	tx, _ = d.Begin()
	g, err := tx.AddGroup(&doc.Group{Name: "desk", Selectable: true, Members: []doc.Handle{h}})
	if err != nil {
		t.Fatal(err)
	}
	cs := tx.Commit()
	if len(cs.Groups) != 1 {
		t.Fatal(cs)
	}
	tx, _ = d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "chair", Entities: []doc.Entity{&doc.Circle{Radius: 2}}}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	d.EndGroup()
	if d.UndoDepth() != 2 {
		t.Fatal(d.UndoDepth())
	}
	d.Undo()
	if len(d.Groups()) != 0 || d.Block("chair") != nil {
		t.Fatal("metadata undo")
	}
	d.Redo()
	if d.Group("DESK") == nil || d.Block("CHAIR") == nil {
		t.Fatal("metadata redo")
	}
	tx, _ = d.Begin()
	tx.Erase(h)
	if len(d.Group("desk").Members) != 0 {
		t.Fatal("membership not removed")
	}
	tx.Abort()
	if len(d.Group("desk").Members) != 1 {
		t.Fatal("membership not restored")
	}
	copy := d.Group("desk")
	copy.Name = "mutated"
	if d.Group("desk") == nil {
		t.Fatal("mutable read")
	}
	tx, _ = d.Begin()
	tx.EraseGroup(g)
	tx.Abort()
	if d.Group("desk") == nil {
		t.Fatal("abort group")
	}
}
func TestBlockReferenceDefinitionRestoration(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	err := tx.DefineBlock(&doc.Block{Name: "chair", Entities: []doc.Entity{&doc.Circle{Radius: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("chair", geom.V(10, 0))
	in.ScaleX = 3
	h := tx.Add(in)
	tx.Commit()
	if in.Bounds().Max.X != 16 {
		t.Fatal(in.Bounds())
	}
	tx, _ = d.Begin()
	err = tx.DefineBlock(&doc.Block{Name: "chair", Entities: []doc.Entity{&doc.Circle{Radius: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	if hs := tx.TakeEdits(); len(hs) != 1 || hs[0] != h {
		t.Fatal(hs)
	}
	cs := tx.Commit()
	if len(cs.Modified) != 1 || cs.Modified[0] != h {
		t.Fatal(cs)
	}
	d.Undo()
	if d.Entity(h).Bounds().Max.X != 16 {
		t.Fatal(d.Entity(h).Bounds())
	}
	d.Redo()
	if d.Entity(h).Bounds().Max.X != 22 {
		t.Fatal("redo")
	}
	tx, _ = d.Begin()
	_ = tx.DefineBlock(&doc.Block{Name: "chair", Entities: []doc.Entity{&doc.Circle{Radius: 1}}})
	tx.Abort()
	if d.Entity(h).Bounds().Max.X != 22 {
		t.Fatal("abort")
	}
}
func TestBlockCyclesAndCopies(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "loop", Entities: []doc.Entity{doc.NewInsert("loop", geom.V(0, 0))}}); err == nil {
		t.Fatal("accepted cycle")
	}
	if err := tx.DefineBlock(&doc.Block{Name: "bad", Base: geom.V(math.Inf(1), 0)}); err == nil {
		t.Fatal("accepted nonfinite")
	}
	tx.Abort()
}
func TestAffineNearestAndNestedAttributes(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	a := &doc.AttributeDefinition{Text: *doc.NewText(geom.V(0, 0), 1, 0, "default"), Tag: "ID"}
	if err := tx.DefineBlock(&doc.Block{Name: "a", Entities: []doc.Entity{&doc.Circle{Radius: 2}, a}}); err != nil {
		t.Fatal(err)
	}
	nested := doc.NewInsert("a", geom.V(1, 0))
	nested.Attributes = []doc.Attribute{{Tag: "ID", Value: "nested"}}
	if err := tx.DefineBlock(&doc.Block{Name: "b", Entities: []doc.Entity{nested}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("b", geom.V(0, 0))
	in.ScaleX = 3
	tx.Add(in)
	tx.Commit()
	c := in.Geometry()[0]
	p := geom.Closest(c, geom.V(10, 0))
	if p.Dist(geom.V(9, 0)) > 1e-7 {
		t.Fatal(p)
	}
	if math.Abs(c.PointAt(math.Pi/2).Y-2) > 1e-9 {
		t.Fatal("nonuniform scale lost")
	}
}

func TestAffineIntersectionsAndTightRotatedBounds(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "e", Entities: []doc.Entity{&doc.Circle{Radius: 2}}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("e", geom.V(0, 0))
	in.ScaleX = 3
	tx.Add(in)
	tx.Commit()
	c := in.Geometry()[0]
	for _, tc := range []struct {
		other geom.Curve
		want  int
	}{{geom.Line{A: geom.V(-10, 0), B: geom.V(10, 0)}, 2}, {geom.Line{A: geom.V(6, -10), B: geom.V(6, 10)}, 1}, {geom.Circle{R: 5}, 4}} {
		ps := geom.Intersect(c, tc.other, 1e-8)
		if len(ps) != tc.want {
			t.Fatal(tc.want, ps)
		}
		for _, p := range ps {
			if math.Abs(p.X*p.X/36+p.Y*p.Y/4-1) > 1e-6 {
				t.Fatal(p)
			}
		}
	}
	tx, _ = d.Begin()
	tx.Modify(in.Handle(), func(e doc.Entity) { e.(*doc.Insert).Rotation = math.Pi / 4 })
	tx.Commit()
	b := in.Bounds()
	if math.Abs(b.Max.X-math.Sqrt(20)) > 1e-7 || math.Abs(b.Max.Y-math.Sqrt(20)) > 1e-7 {
		t.Fatal("loose rotated bounds", b)
	}
}
func TestInvalidInsertCannotEnterDocument(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if h := tx.Add(doc.NewInsert("missing", geom.V(0, 0))); h != 0 {
		t.Fatal(h)
	}
	_ = tx.DefineBlock(&doc.Block{Name: "a", Entities: []doc.Entity{&doc.Circle{Radius: 1}}})
	in := doc.NewInsert("a", geom.V(math.Inf(1), 0))
	if h := tx.Add(in); h != 0 {
		t.Fatal("nonfinite inserted", h)
	}
	tx.Abort()
}

func TestImportBlockSupportUndoPreservesUnrelatedLayerEdits(t *testing.T) {
	d := doc.New()
	a := d.AddLayer("A", 1)
	src := doc.New()
	src.AddLayer("B", 3)
	src.Preserved.Tables = map[string][]doc.RawRecord{"STYLE": {{Pairs: []doc.Pair{{Code: 0, Value: "STYLE"}, {Code: 5, Value: "100"}, {Code: 2, Value: "NOTES"}, {Code: 3, Value: "romans.shx"}}}}, "LTYPE": {{Pairs: []doc.Pair{{Code: 0, Value: "LTYPE"}, {Code: 5, Value: "101"}, {Code: 2, Value: "DASH"}, {Code: 340, Value: "100"}}}}}
	tx, _ := d.Begin()
	tx.ImportBlockSupport(src)
	cs := tx.Commit()
	if !cs.Support || d.Layer("B") == nil {
		t.Fatal(cs)
	}
	style := d.Preserved.Tables["STYLE"][0]
	var styleHandle string
	for _, p := range style.Pairs {
		if p.Code == 5 {
			styleHandle = p.Value
		}
	}
	if styleHandle == "100" && d.Header.HandSeed <= 0x101 {
		t.Fatal("handle remap failed")
	}
	for _, p := range d.Preserved.Tables["LTYPE"][0].Pairs {
		if p.Code == 340 && p.Value != styleHandle {
			t.Fatal("style pointer not remapped")
		}
	}
	a.Off = true
	d.Undo()
	if d.Layer("B") != nil || !d.Layer("A").Off || len(d.Preserved.Tables["STYLE"]) != 0 {
		t.Fatal("undo support overwrote unrelated layer state")
	}
	d.Redo()
	if d.Layer("B") == nil || !d.Layer("A").Off {
		t.Fatal("redo support")
	}
	tx, _ = d.Begin()
	tx.ImportBlockSupport(src)
	tx.Abort()
	if d.Layer("B") == nil || !d.Layer("A").Off {
		t.Fatal("abort support")
	}
}
func TestBlockExpansionBudgetRejectsDepthAndBreadth(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "leaf", Entities: []doc.Entity{&doc.Circle{Radius: 1}}}); err != nil {
		t.Fatal(err)
	}
	last := "leaf"
	for i := 0; i < doc.MaxBlockDepth+2; i++ {
		name := fmt.Sprintf("level%d", i)
		err := tx.DefineBlock(&doc.Block{Name: name, Entities: []doc.Entity{doc.NewInsert(last, geom.V(0, 0))}})
		if i < doc.MaxBlockDepth-1 && err != nil {
			t.Fatal(i, err)
		}
		if i >= doc.MaxBlockDepth-1 {
			if err == nil {
				t.Fatal("deep graph accepted")
			}
			break
		}
		last = name
	}
	tx.Abort()
	d = doc.New()
	tx, _ = d.Begin()
	_ = tx.DefineBlock(&doc.Block{Name: "leaf", Entities: []doc.Entity{&doc.Circle{Radius: 1}}})
	last = "leaf"
	rejected := false
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("wide%d", i)
		err := tx.DefineBlock(&doc.Block{Name: name, Entities: []doc.Entity{doc.NewInsert(last, geom.V(0, 0)), doc.NewInsert(last, geom.V(1, 0))}})
		if err != nil {
			rejected = true
			break
		}
		last = name
	}
	if !rejected {
		t.Fatal("exponential graph accepted")
	}
	tx.Abort()
}

func TestExplodePreservesImportedAttributePlacementAndColor(t *testing.T) {
	d := doc.New()
	d.AddLayer("root", 1)
	d.AddLayer("child", 5)
	tx, _ := d.Begin()
	a := &doc.AttributeDefinition{Text: *doc.NewText(geom.V(0, 3), .75, 0, "default"), Tag: "ID"}
	if err := tx.DefineBlock(&doc.Block{Name: "b", Entities: []doc.Entity{&doc.Circle{EntityCommon: doc.EntityCommon{Layer: "child", Color: doc.ByBlock}, Radius: 2}, a}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("b", geom.V(20, 10))
	in.Layer = "root"
	in.Rotation = math.Pi / 6
	in.ScaleX, in.ScaleY = 2, 2
	placed := doc.NewText(geom.V(123, 456), 1.125, .3, "C42")
	in.Attributes = []doc.Attribute{{Tag: "ID", Value: "C42", Text: placed}}
	tx.Add(in)
	tx.Commit()
	parts, err := in.Explode()
	if err != nil || len(parts) != 2 {
		t.Fatal(parts, err)
	}
	if parts[0].Common().Color != 1 {
		t.Fatal("ByBlock changed to child layer color")
	}
	text := parts[1].(*doc.Text)
	if text.Insert != placed.Insert || text.Height != placed.Height || text.Rotation != placed.Rotation || text.Value != "C42" {
		t.Fatal("imported attribute moved", text)
	}
}

func TestEmptyReferenceDAGExpansionBudget(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "empty0"}); err != nil {
		t.Fatal(err)
	}
	last := "empty0"
	rejected := false
	for level := 1; level <= 20; level++ {
		name := fmt.Sprintf("empty%d", level)
		err := tx.DefineBlock(&doc.Block{Name: name, Entities: []doc.Entity{doc.NewInsert(last, geom.V(0, 0)), doc.NewInsert(last, geom.V(0, 0))}})
		if err != nil {
			rejected = true
			break
		}
		last = name
	}
	if !rejected {
		t.Fatal("accepted over two million empty reference visits")
	}
	tx.Abort()
	loaded := doc.New()
	loaded.LoadBlock(&doc.Block{Name: "empty0"})
	last = "empty0"
	for level := 1; level <= 20; level++ {
		name := fmt.Sprintf("empty%d", level)
		loaded.LoadBlock(&doc.Block{Name: name, Entities: []doc.Entity{doc.NewInsert(last, geom.V(0, 0)), doc.NewInsert(last, geom.V(0, 0))}})
		last = name
	}
	if loaded.BlockError(last) == nil {
		t.Fatal("loaded empty DAG bypasses shared validation")
	}
}
func TestAffineClassificationAcrossMagnitudes(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		radius, sx, sy, rx, ry float64
	}{{"tiny unequal", 1e12, 1e-12, 2e-12, 1, 2}, {"tiny uniform", 1e200, 1e-200, 1e-200, 1, 1}, {"huge unequal", 1e-200, 1e200, 2e200, 1, 2}, {"huge uniform", 1e-200, 1e200, 1e200, 1, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			d := doc.New()
			tx, _ := d.Begin()
			if err := tx.DefineBlock(&doc.Block{Name: "c", Entities: []doc.Entity{&doc.Circle{Radius: tc.radius}}}); err != nil {
				t.Fatal(err)
			}
			in := doc.NewInsert("c", geom.V(0, 0))
			in.ScaleX, in.ScaleY = tc.sx, tc.sy
			if tx.Add(in) == 0 {
				t.Fatal("finite resolved reference rejected")
			}
			tx.Commit()
			b := in.Bounds()
			if math.Abs(b.Max.X-tc.rx) > 1e-9 || math.Abs(b.Max.Y-tc.ry) > 1e-9 || math.Abs(b.Min.X+tc.rx) > 1e-9 || math.Abs(b.Min.Y+tc.ry) > 1e-9 {
				t.Fatal("anisotropy/scale lost", b)
			}
			if points := in.Geometry()[0].Tessellate(.01, nil); len(points) > 1000 {
				t.Fatal("scale normalization causes excessive tessellation", len(points))
			}
		})
	}
}
func TestImportUndoKeepsCurrentLayerValid(t *testing.T) {
	src := doc.New()
	src.AddLayer("Imported", 3)
	d := doc.New()
	d.AddLayer("A", 1)
	tx, _ := d.Begin()
	tx.ImportBlockSupport(src)
	tx.Commit()
	d.CurrentLayer = "Imported"
	d.Layer("A").Off = true
	d.Undo()
	if d.Layer(d.CurrentLayer) == nil {
		t.Fatalf("current layer %q no longer exists", d.CurrentLayer)
	}
	if d.Layer("Imported") != nil || !d.Layer("A").Off {
		t.Fatal("support undo changed unrelated metadata")
	}
	d.Redo()
	if d.Layer(d.CurrentLayer) == nil || d.Layer("Imported") == nil || !d.Layer("A").Off {
		t.Fatal("redo invalid metadata")
	}
}

func TestRootAttributeExpansionBudget(t *testing.T) {
	for _, attributes := range []int{doc.MaxBlockEntities - 1, doc.MaxBlockEntities, doc.MaxBlockEntities + 1} {
		d := doc.New()
		tx, _ := d.Begin()
		if err := tx.DefineBlock(&doc.Block{Name: "empty"}); err != nil {
			t.Fatal(err)
		}
		in := doc.NewInsert("empty", geom.V(0, 0))
		in.Attributes = make([]doc.Attribute, attributes)
		h := tx.Add(in)
		if (attributes < doc.MaxBlockEntities) != (h != 0) {
			t.Fatalf("root plus %d attributes: Add=%s", attributes, h)
		}
		tx.Abort()
	}
	d := doc.New()
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "one", Entities: []doc.Entity{&doc.Circle{Radius: 1}}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("one", geom.V(0, 0))
	in.Attributes = make([]doc.Attribute, doc.MaxBlockEntities-2)
	h := tx.Add(in)
	if h == 0 {
		t.Fatal("exact combined threshold rejected")
	}
	tx.Commit()
	tx, _ = d.Begin()
	tx.Modify(h, func(e doc.Entity) { i := e.(*doc.Insert); i.Attributes = append(i.Attributes, doc.Attribute{}) })
	tx.Commit()
	if n := len(d.Entity(h).(*doc.Insert).Attributes); n != doc.MaxBlockEntities-2 {
		t.Fatal("over-budget Modify was retained", n)
	}
	loaded := doc.New()
	loaded.LoadBlock(&doc.Block{Name: "empty"})
	over := doc.NewInsert("empty", geom.V(0, 0))
	over.Attributes = make([]doc.Attribute, doc.MaxBlockEntities)
	loaded.Load(over)
	if !over.Bounds().IsEmpty() || len(over.Parts()) != 0 {
		t.Fatal("defensive geometry expanded over-budget root")
	}
}

func TestDefinitionReplacementHonorsRootAttributeBudget(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	_ = tx.DefineBlock(&doc.Block{Name: "empty"})
	in := doc.NewInsert("empty", geom.V(0, 0))
	in.Attributes = make([]doc.Attribute, doc.MaxBlockEntities-1)
	if tx.Add(in) == 0 {
		t.Fatal("threshold rejected")
	}
	tx.Commit()
	tx, _ = d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "empty", Entities: []doc.Entity{&doc.Circle{Radius: 1}}}); err == nil {
		t.Fatal("replacement exceeded live root attribute budget")
	}
	tx.Abort()
}
