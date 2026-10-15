package dxf_test

import (
	"bytes"
	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
	"slices"
	"strings"
	"testing"
)

func TestForeignLayerTableOrderIncludesZeroInPlace(t *testing.T) {
	src := group("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "LAYER", "0", "LAYER", "2", "Walls", "62", "1", "0", "LAYER", "2", "0", "62", "7", "0", "LAYER", "2", "Furniture", "62", "3", "0", "ENDTAB", "0", "ENDSEC", "0", "EOF")
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Walls", "0", "Furniture"}
	for i, l := range d.Layers() {
		if l.Name != want[i] {
			t.Fatalf("table row%d=%s, want%s", i, l.Name, want[i])
		}
	}
}

func TestLayerOrderRepeatedRoundTripR12AndGroups(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		d := doc.New()
		d.AddLayer("Walls", 1)
		d.AddLayer("Furniture", 3).Locked = true
		tx, _ := d.Begin()
		h := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Walls"}, End: geom.V(1, 0)})
		if grouped {
			if _, err := tx.AddGroup(&doc.Group{Name: "g", Selectable: true, Members: []doc.Handle{h}}); err != nil {
				t.Fatal(err)
			}
		}
		tx.Commit()
		for _, order := range [][]string{{"Walls", "0", "Furniture"}, {"Furniture", "Walls", "0"}} {
			tx, _ := d.Begin()
			if err := tx.SetLayerOrder(order); err != nil {
				t.Fatal(err)
			}
			tx.Commit()
			for round := 0; round < 3; round++ {
				var out bytes.Buffer
				warnings, err := (dxf.Codec{}).Write(&out, d)
				if err != nil || len(warnings) != 0 {
					t.Fatal(warnings, err)
				}
				d, warnings, err = (dxf.Codec{}).Read(bytes.NewReader(out.Bytes()))
				if err != nil || len(warnings) != 0 {
					t.Fatal(warnings, err)
				}
				var names []string
				for _, l := range d.Layers() {
					names = append(names, l.Name)
				}
				if !slices.Equal(names, order) || d.CurrentLayer != "0" || !d.Layer("Furniture").Locked {
					t.Fatal("layer state/order lost", names, d.CurrentLayer)
				}
				if grouped && len(d.Groups()) != 1 {
					t.Fatal("group lost")
				}
			}
		}
	}
}
func TestLayerTableDuplicatesAndMissingZeroKeepAllLayers(t *testing.T) {
	src := group("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "LAYER", "0", "LAYER", "2", "Walls", "62", "1", "0", "LAYER", "2", "Furniture", "62", "3", "0", "LAYER", "2", "Walls", "62", "5", "0", "ENDTAB", "0", "ENDSEC", "0", "EOF")
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range d.Layers() {
		got = append(got, l.Name)
	}
	if !slices.Equal(got, []string{"Walls", "Furniture", "0"}) || d.Layer("Walls").Color != 5 {
		t.Fatal("duplicate/default layer handling", got)
	}
}
