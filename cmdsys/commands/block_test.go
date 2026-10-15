package commands_test

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"strings"
	"testing"
)

func TestGroupCommandsOverlapAndUndo(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(100, 0), geom.V(110, 0)), line(geom.V(200, 0), geom.V(210, 0)))
	hs := s.Drawing().Handles()
	s.Selection().Add(hs[0], hs[1])
	s.Run("GROUP")
	s.Submit("desk")
	if g := s.Drawing().Group("desk"); g == nil || !g.Selectable {
		t.Fatal(g)
	}
	s.Selection().Add(hs[1], hs[2])
	s.Run("GROUP")
	s.Submit("row")
	s.Click(geom.V(5, 0), false)
	if s.Selection().Len() != 3 {
		t.Fatal(s.Selection().Handles())
	}
	s.Click(geom.V(5, 0), true)
	if s.Selection().Len() != 0 {
		t.Fatal("group removal")
	}
	s.Run("-GROUP")
	s.Submit("Selectable")
	s.Submit("desk")
	s.Click(geom.V(5, 0), false)
	if s.Selection().Len() != 1 {
		t.Fatal("selectability")
	}
	s.Selection().Clear()
	s.Run("GROUP")
	s.Submit("Rename")
	s.Submit("row")
	s.Submit("chairs")
	if s.Drawing().Group("chairs") == nil {
		t.Fatal("rename")
	}
	s.Drawing().Undo()
	if s.Drawing().Group("row") == nil {
		t.Fatal("undo rename")
	}
}
func TestBlockAttributeCommandsAndRedefinition(t *testing.T) {
	s := newSession(t)
	s.Run("ATTDEF")
	for _, in := range []string{"ID", "Number", "C1", "0,2", "1"} {
		s.Submit(in)
	}
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 4})
	s.Selection().Add(s.Drawing().Handles()...)
	s.Run("BLOCK")
	s.Submit("Chair")
	s.Submit("0,0")
	if s.Drawing().Len() != 1 || s.Drawing().Block("chair") == nil {
		t.Fatal("block creation")
	}
	s.Run("INSERT")
	for _, in := range []string{"Chair", "20,0", "2", "90", "C2"} {
		s.Submit(in)
	}
	refs := entities[*doc.Insert](s)
	if len(refs) != 2 || refs[1].Attributes[0].Value != "C2" {
		t.Fatal(refs)
	}
	s.Run("ATTEDIT")
	for _, in := range []string{"20,8", "ID", "C3"} {
		s.Submit(in)
	}
	if refs[1].Attributes[0].Value != "C3" {
		t.Fatal("attribute edit", s.History())
	}
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 8})
	s.Selection().Add(s.Drawing().Handles()[len(s.Drawing().Handles())-1])
	s.Run("BLOCKREDEFINE")
	s.Submit("Chair")
	s.Submit("0,0")
	if refs[1].Bounds().Max.Y != 16 {
		t.Fatal(refs[1].Bounds())
	}
	if refs[1].Attributes[0].Value != "C3" {
		t.Fatal("attribute value lost")
	}
	s.Drawing().Undo()
	if s.Drawing().Entity(refs[1].Handle()).Bounds().Max.Y != 8 {
		t.Fatal("definition undo")
	}
	s.Selection().Clear()
	s.Selection().Add(refs[0].Handle())
	s.Run("EXPLODE")
	if len(entities[*doc.Insert](s)) != 1 {
		t.Fatal("explode", s.History())
	}
}
func TestItalianGroupAndBlockCommands(t *testing.T) {
	s := newSession(t)
	tr := i18n.For(i18n.Italian)
	s.SetTranslator(&tr)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)))
	s.Selection().Add(s.Drawing().Handles()...)
	s.Run("GRUPPO")
	s.Submit("tavolo")
	s.Run("GRUPPO")
	s.Submit("rinomina")
	s.Submit("tavolo")
	s.Submit("sedie")
	if s.Drawing().Group("sedie") == nil {
		t.Fatal(strings.Join(s.History(), "\n"))
	}
	s.Selection().Add(s.Drawing().Handles()...)
	s.Run("BLOCCO")
	s.Submit("Sedia")
	s.Submit("0,0")
	if s.Drawing().Block("Sedia") == nil {
		t.Fatal(s.History())
	}
}

func TestGroupAllRetainsOffLayerAndFiltersExpandedMembers(t *testing.T) {
	s := newSession(t)
	off := s.Drawing().AddLayer("off", 1)
	off.Off = true
	s.Drawing().AddLayer("locked", 1).Locked = true
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), &doc.Line{EntityCommon: doc.EntityCommon{Layer: "off"}, Start: geom.V(100, 0), End: geom.V(110, 0)}, &doc.Line{EntityCommon: doc.EntityCommon{Layer: "locked"}, Start: geom.V(200, 0), End: geom.V(210, 0)})
	for pass := 0; pass < 2; pass++ {
		s.Run("SELECT")
		s.Submit("ALL")
		if s.Selection().Len() != 2 {
			t.Fatal("ALL dropped off-layer seed", s.Selection().Handles())
		}
		s.Escape()
		s.Selection().Clear()
		if pass == 0 {
			tx, _ := s.Drawing().Begin()
			_, err := tx.AddGroup(&doc.Group{Name: "mixed", Selectable: true, Members: s.Drawing().Handles()})
			if err != nil {
				t.Fatal(err)
			}
			tx.Commit()
		}
	}
	s.Click(geom.V(5, 0), false)
	if s.Selection().Len() != 1 {
		t.Fatal("group added hidden or locked member", s.Selection().Handles())
	}
}
func TestAnisotropicBlockPickingSnapAndWindow(t *testing.T) {
	s := newSession(t)
	tx, _ := s.Drawing().Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "ellipse", Entities: []doc.Entity{&doc.Circle{Radius: 2}}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("ellipse", geom.V(10, 0))
	in.ScaleX = 3
	h := tx.Add(in)
	tx.Commit()
	if got, ok := cmdsys.Pick(s.Drawing(), s.Index(), geom.V(16.1, 0), .2); !ok || got != h {
		t.Fatal("ellipse pick", got, ok)
	}
	hit, ok := cmdsys.FindSnap(s.Drawing(), s.Index(), geom.V(16.1, 0), .2, cmdsys.SnapNearest)
	if !ok || hit.Point.Dist(geom.V(16, 0)) > 1e-7 {
		t.Fatal(hit, ok)
	}
	s.SelectBox(geom.V(3, -3), geom.V(17, 3), false)
	if !s.Selection().Has(h) {
		t.Fatal("window missed ellipse")
	}
	s.Selection().Clear()
	s.SelectBox(geom.V(5, -3), geom.V(15, 3), false)
	if s.Selection().Len() != 0 {
		t.Fatal("window accepted partial ellipse")
	}
}

type symbolFiles struct {
	source, saved *doc.Drawing
	opens, saves  int
}

func (f *symbolFiles) Open(string) (*doc.Drawing, []string, error) {
	f.opens++
	return f.source, nil, nil
}
func (f *symbolFiles) Save(_ string, d *doc.Drawing) ([]string, error) {
	f.saves++
	f.saved = d
	return nil, nil
}
func TestLibraryImportCancelUndoAndConflicts(t *testing.T) {
	s := newSession(t)
	src := doc.New()
	src.AddLayer("furniture", 3)
	tx, _ := src.Begin()
	tx.Add(&doc.Circle{EntityCommon: doc.EntityCommon{Layer: "furniture"}, Radius: 2})
	tx.Commit()
	files := &symbolFiles{source: src}
	s.SetFiles(files)
	s.Run("INSERT")
	s.Submit("chair.dxf")
	s.Submit("10,0")
	s.Escape()
	if s.Drawing().Layer("furniture") != nil || len(s.Drawing().Blocks()) != 0 {
		t.Fatal("cancel changed destination")
	}
	s.Run("INSERT")
	for _, a := range []string{"chair.dxf", "10,0", "1", "0"} {
		s.Submit(a)
	}
	if s.Drawing().Len() != 1 || s.Drawing().Block("chair") == nil || s.Drawing().Layer("furniture") == nil {
		t.Fatal("library import", s.History())
	}
	s.Drawing().Undo()
	if s.Drawing().Len() != 0 || s.Drawing().Block("chair") != nil || s.Drawing().Layer("furniture") != nil {
		t.Fatal("library undo not atomic")
	}
	s.Drawing().Redo()
	s.Run("WBLOCK")
	s.Submit("chair")
	s.Submit("chair.dxf")
	if files.saved == nil || files.saved.Block("chair") == nil || files.saved.Layer("furniture") == nil {
		t.Fatal("library export")
	}
	source := files.saved
	other := newSession(t)
	other.SetFiles(&symbolFiles{source: source})
	other.Run("INSERT")
	for _, a := range []string{"chair.dxf", "0,0", "1", "0"} {
		other.Submit(a)
	}
	if other.Drawing().Len() != 1 || other.Drawing().Block("chair") == nil {
		t.Fatal("own WBLOCK cannot be inserted", other.History())
	}
}

func TestBlockVisibleExtentsExcludeHiddenChildren(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("hidden", 1).Off = true
	tx, _ := s.Drawing().Begin()
	err := tx.DefineBlock(&doc.Block{Name: "mixed", Entities: []doc.Entity{line(geom.V(-2, -2), geom.V(2, 2)), &doc.Line{EntityCommon: doc.EntityCommon{Layer: "hidden"}, Start: geom.V(10000, 0), End: geom.V(10010, 0)}}})
	if err != nil {
		t.Fatal(err)
	}
	h := tx.Add(doc.NewInsert("mixed", geom.V(0, 0)))
	tx.Commit()
	if s.Drawing().Entity(h).Bounds().Max.X != 10010 {
		t.Fatal("index bounds lost hidden child")
	}
	if s.Drawing().Extents().Max.X != 2 {
		t.Fatal("display extents include hidden child")
	}
	s.SelectBox(geom.V(-3, -3), geom.V(3, 3), false)
	if !s.Selection().Has(h) {
		t.Fatal("visible window selection failed")
	}
	s.Selection().Clear()
	s.Run("ZOOM")
	s.Submit("Extents")
	if s.View().Center.Dist(geom.V(0, 0)) > 1e-9 {
		t.Fatal("zoom includes hidden child", s.View().Center)
	}
}
func TestImportedGroupDoesNotSelectUnsupportedMembers(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)))
	lineHandle := s.Drawing().Handles()[0]
	u := &doc.Unknown{EntityCommon: doc.EntityCommon{H: 0x500, Layer: "0"}, Type: "POINT"}
	s.Drawing().Load(u)
	s.Drawing().LoadGroup(&doc.Group{Name: "mixed", Selectable: true, Members: []doc.Handle{lineHandle, u.Handle()}})
	s.Click(geom.V(5, 0), false)
	if s.Selection().Len() != 1 || !s.Selection().Has(lineHandle) {
		t.Fatal("unknown selected through group", s.Selection().Handles())
	}
}
