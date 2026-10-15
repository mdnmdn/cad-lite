package dxf_test

import (
	"bytes"
	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestSymbolsAndGroupsRepeatedRoundTrip(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		d := doc.New()
		tx, _ := d.Begin()
		a := &doc.AttributeDefinition{Text: *doc.NewText(geom.V(0, 3), .75, 0, "C1"), Tag: "ID", Prompt: "Number"}
		if err := tx.DefineBlock(&doc.Block{Name: "Chair", Base: geom.V(1, 1), Entities: []doc.Entity{&doc.Circle{Radius: 2}, a}}); err != nil {
			t.Fatal(err)
		}
		in := doc.NewInsert("chair", geom.V(20, 10))
		in.Rotation = math.Pi / 6
		in.ScaleX, in.ScaleY = 2, 2
		in.Attributes = []doc.Attribute{{Tag: "ID", Value: "C42"}}
		h := tx.Add(in)
		if grouped {
			if _, err := tx.AddGroup(&doc.Group{Name: "Desk", Selectable: true, Members: []doc.Handle{h}}); err != nil {
				t.Fatal(err)
			}
		}
		tx.Commit()
		want := d.Extents()
		for round := 0; round < 3; round++ {
			var out bytes.Buffer
			warns, err := (dxf.Codec{}).Write(&out, d)
			if err != nil || len(warns) > 0 {
				t.Fatal(warns, err)
			}
			version := "AC1009"
			if grouped {
				version = "AC1015"
			}
			if !strings.Contains(out.String(), version) {
				t.Fatal("wrong version")
			}
			d, warns, err = (dxf.Codec{}).Read(bytes.NewReader(out.Bytes()))
			if err != nil || len(warns) > 0 {
				t.Fatal(warns, err)
			}
			if len(d.Blocks()) != 1 || d.Len() != 1 {
				t.Fatal("symbols lost")
			}
			if grouped && (len(d.Groups()) != 1 || d.Groups()[0].Members[0] != h) {
				t.Fatal("group lost")
			}
			ref := d.Entity(h).(*doc.Insert)
			if len(ref.Attributes) != 1 || ref.Attributes[0].Tag != "ID" || ref.Attributes[0].Value != "C42" {
				t.Fatal(ref.Attributes)
			}
			if ref.Attributes[0].Text == nil || math.Abs(ref.Attributes[0].Text.Height-1.5) > 1e-9 {
				t.Fatal("attribute placement lost")
			}
			if !d.Extents().Min.Near(want.Min, 1e-7) || !d.Extents().Max.Near(want.Max, 1e-7) {
				t.Fatal("bounds changed", want, d.Extents())
			}
			want = d.Extents()
		}
	}
}
func TestUnsupportedInsertAndUnknownBlockContentPreserved(t *testing.T) {
	src := richFile(strings.Replace(insEnt, " 30\n0\n", " 30\n2\n", 1))
	d, w, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil || len(w) != 1 {
		t.Fatal(w, err)
	}
	before := unknownRaw(d)[0x31]
	var out bytes.Buffer
	if _, err = (dxf.Codec{}).Write(&out, d); err != nil {
		t.Fatal(err)
	}
	back, _, err := (dxf.Codec{}).Read(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	after := unknownRaw(back)[0x31]
	if len(after) != len(r12Groups(before)) {
		t.Fatal("unsupported record lost", before, after)
	}
}

func TestForeignOwnerCollisionAndObjectsRepeatedRoundTrip(t *testing.T) {
	src := group("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "BLOCK_RECORD", "0", "BLOCK_RECORD", "5", "10", "2", "*Model_Space", "0", "BLOCK_RECORD", "5", "20", "2", "Chair", "0", "ENDTAB", "0", "ENDSEC") + group("0", "SECTION", "2", "BLOCKS", "0", "BLOCK", "5", "21", "330", "20", "2", "Chair", "10", "0", "20", "0", "0", "CIRCLE", "5", "22", "8", "0", "10", "0", "20", "0", "40", "2", "0", "ENDBLK", "5", "23", "330", "20", "0", "ENDSEC") + group("0", "SECTION", "2", "ENTITIES", "0", "INSERT", "5", "30", "330", "10", "8", "0", "2", "Chair", "10", "20", "20", "10", "41", "2", "42", "1.5", "50", "30", "0", "ENDSEC") + group("0", "SECTION", "2", "OBJECTS", "0", "DICTIONARY", "5", "40", "330", "0", "100", "AcDbDictionary", "3", "ACAD_GROUP", "350", "41", "3", "CUSTOM_N11", "350", "1F", "0", "DICTIONARY", "5", "41", "330", "40", "100", "AcDbDictionary", "3", "Desk", "350", "50", "0", "GROUP", "5", "50", "330", "41", "100", "AcDbGroup", "70", "0", "71", "1", "340", "30", "0", "XRECORD", "5", "1F", "330", "40", "100", "AcDbXrecord", "280", "1", "1", "keep-me", "0", "ENDSEC", "0", "EOF")
	d, w, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil || len(w) != 0 {
		t.Fatal(w, err)
	}
	for n := 0; n < 3; n++ {
		var out bytes.Buffer
		if w, err = (dxf.Codec{}).Write(&out, d); err != nil || len(w) != 0 {
			t.Fatal(w, err)
		}
		d, w, err = (dxf.Codec{}).Read(bytes.NewReader(out.Bytes()))
		if err != nil || len(w) != 0 {
			t.Fatal(w, err)
		}
		if len(d.Groups()) != 1 || d.Entity(0x30) == nil {
			t.Fatal("foreign references lost")
		}
		record := false
		custom := false
		for _, r := range d.Preserved.Objects {
			if r.Type() == "XRECORD" {
				for _, p := range r.Pairs {
					if p.Code == 1 && p.Value == "keep-me" {
						record = true
					}
				}
			}
			for _, p := range r.Pairs {
				if p.Code == 3 && p.Value == "CUSTOM_N11" {
					custom = true
				}
			}
		}
		if !record || !custom {
			t.Fatal("custom dictionary data lost")
		}
		in := d.Entity(0x30).(*doc.Insert)
		if in.ScaleX != 2 || in.ScaleY != 1.5 {
			t.Fatal("anisotropy lost")
		}
	}
}

func TestPreservedExternalBlockRecordOwnerRoundTrip(t *testing.T) {
	input := group("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "BLOCK_RECORD", "0", "BLOCK_RECORD", "5", "20", "2", "external", "0", "ENDTAB", "0", "ENDSEC") + group("0", "SECTION", "2", "BLOCKS", "0", "BLOCK", "5", "21", "330", "20", "8", "0", "2", "external", "70", "4", "10", "0", "20", "0", "1", "external.dxf", "0", "ENDBLK", "5", "22", "330", "20", "8", "0", "0", "ENDSEC") + group("0", "SECTION", "2", "ENTITIES", "0", "LINE", "5", "30", "8", "0", "10", "0", "20", "0", "11", "1", "21", "1", "0", "ENDSEC", "0", "EOF")
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := d.Begin()
	if _, err := tx.AddGroup(&doc.Group{Name: "g", Selectable: true, Members: []doc.Handle{0x30}}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	for round := 0; round < 3; round++ {
		var out bytes.Buffer
		if _, err := (dxf.Codec{}).Write(&out, d); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		var typ, name, handle, owner string
		var want string
		var owners []string
		flush := func() {
			if typ == "BLOCK_RECORD" && name == "external" {
				want = handle
			}
			if typ == "BLOCK" && name == "external" || typ == "ENDBLK" && handle == "22" {
				owners = append(owners, owner)
			}
		}
		for i := 0; i+1 < len(lines); i += 2 {
			code := strings.TrimSpace(lines[i])
			value := strings.TrimSpace(lines[i+1])
			if code == "0" {
				flush()
				typ, name, handle, owner = value, "", "", ""
			}
			switch code {
			case "2":
				name = value
			case "5":
				handle = value
			case "330":
				owner = value
			}
		}
		flush()
		if want == "" || len(owners) != 2 {
			t.Fatal("missing block structures", want, owners)
		}
		for _, owner := range owners {
			if owner != want {
				t.Fatalf("BLOCK/ENDBLK owner %s differs from BLOCK_RECORD %s", owner, want)
			}
		}
		d, _, err = (dxf.Codec{}).Read(bytes.NewReader(out.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestFileQuarantinesEmptyReferenceExpansion(t *testing.T) {
	var src strings.Builder
	src.WriteString(group("0", "SECTION", "2", "BLOCKS"))
	for level := 0; level <= 20; level++ {
		name := fmt.Sprintf("e%d", level)
		src.WriteString(group("0", "BLOCK", "2", name, "70", "0", "10", "0", "20", "0"))
		if level > 0 {
			for child := 0; child < 2; child++ {
				src.WriteString(group("0", "INSERT", "2", fmt.Sprintf("e%d", level-1), "10", "0", "20", "0"))
			}
		}
		src.WriteString(group("0", "ENDBLK"))
	}
	src.WriteString(group("0", "ENDSEC", "0", "SECTION", "2", "ENTITIES", "0", "INSERT", "5", "F00", "2", "e20", "10", "0", "20", "0", "0", "ENDSEC", "0", "EOF"))
	d, w, err := (dxf.Codec{}).Read(strings.NewReader(src.String()))
	if err != nil {
		t.Fatal(err)
	}
	u, ok := d.Entity(0xF00).(*doc.Unknown)
	if !ok || u.Type != "INSERT" || len(w) != 1 || !strings.Contains(w[0], "block expansion limit exceeded") {
		t.Fatal("invalid loaded graph became editable", d.Entity(0xF00), w)
	}
	if len(u.Raw) == 0 {
		t.Fatal("original insertion not preserved")
	}
}

func TestFileQuarantinesCombinedRootAttributeBudget(t *testing.T) {
	var src strings.Builder
	src.WriteString(group("0", "SECTION", "2", "BLOCKS"))
	for level := 0; level <= 15; level++ {
		src.WriteString(group("0", "BLOCK", "2", fmt.Sprintf("e%d", level), "70", "0", "10", "0", "20", "0"))
		if level > 0 {
			for i := 0; i < 2; i++ {
				src.WriteString(group("0", "INSERT", "2", fmt.Sprintf("e%d", level-1), "10", "0", "20", "0"))
			}
		}
		src.WriteString(group("0", "ENDBLK"))
	}
	src.WriteString(group("0", "ENDSEC", "0", "SECTION", "2", "ENTITIES", "0", "INSERT", "5", "F00", "2", "e15", "66", "1", "10", "0", "20", "0"))
	for i := 0; i < doc.MaxBlockEntities-65535+1; i++ {
		src.WriteString(group("0", "ATTRIB", "2", fmt.Sprintf("T%d", i), "1", "value", "10", "0", "20", "0", "40", "1", "70", "1"))
	}
	src.WriteString(group("0", "SEQEND", "0", "ENDSEC", "0", "EOF"))
	d, w, err := (dxf.Codec{}).Read(strings.NewReader(src.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Entity(0xF00).(*doc.Unknown); !ok || len(w) != 1 || !strings.Contains(w[0], "block expansion limit exceeded") {
		t.Fatal("over-budget root attributes not quarantined", w)
	}
	if len(d.Entity(0xF00).(*doc.Unknown).Raw) < doc.MaxBlockEntities-65535 {
		t.Fatal("source attributes lost")
	}
}
