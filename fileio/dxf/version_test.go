package dxf_test

import (
	"bytes"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
)

// grouped is sample() with a group and an MTEXT, content R12 cannot hold.
func grouped(t *testing.T) *doc.Drawing {
	t.Helper()
	d := sample()
	tx, _ := d.Begin()
	tx.Add(doc.NewMText(geom.V(0, 20), 2, 30, "first\\Psecond"))
	tx.Add(doc.NewMText(geom.V(0, 40), 2, 30, "third"))
	if _, err := tx.AddGroup(&doc.Group{Name: "G", Selectable: true, Members: d.Handles()[:2]}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	return d
}

func writeVersion(t *testing.T, d *doc.Drawing, v string) (string, []string) {
	t.Helper()
	var out bytes.Buffer
	warns, err := dxf.Codec{}.WriteVersion(&out, d, v)
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), warns
}

func TestWriteVersionAutomaticForcedAndUnknown(t *testing.T) {
	plain := sample()
	if v := (dxf.Codec{}).AutoVersion(plain); v != "AC1009" {
		t.Fatalf("plain drawing auto version %s", v)
	}
	if v := (dxf.Codec{}).AutoVersion(grouped(t)); v != "AC1015" {
		t.Fatalf("grouped drawing auto version %s", v)
	}
	auto, _ := writeVersion(t, plain, "")
	var w bytes.Buffer
	if _, err := (dxf.Codec{}).Write(&w, plain); err != nil || w.String() != auto {
		t.Fatalf("Write differs from WriteVersion(\"\"): %v", err)
	}
	// AC1015 forced on a plain drawing takes the AutoCAD 2000 path and reads back the same.
	modern, warns := writeVersion(t, plain, "AC1015")
	if !strings.Contains(modern, "AC1015") || !strings.Contains(modern, "LWPOLYLINE") || len(warns) > 0 {
		t.Fatalf("forced 2000: warnings %v", warns)
	}
	back, _, err := (dxf.Codec{}).Read(strings.NewReader(modern))
	if err != nil || back.Len() != plain.Len() || back.Header.Version != "AC1015" {
		t.Fatalf("2000 read back: %v, %d entities, version %q", err, back.Len(), back.Header.Version)
	}
	if _, err := (dxf.Codec{}).WriteVersion(&w, plain, "AC1018"); err == nil || !strings.Contains(err.Error(), "AC1018") {
		t.Fatalf("unwritable version: %v", err)
	}
}

// R12 forced on a drawing that needs AutoCAD 2000 drops the groups and writes MTEXT as TEXT, with
// one warning for each kind of loss.
func TestWriteVersionR12WarnsForWhatItCannotHold(t *testing.T) {
	d := grouped(t)
	out, warns := writeVersion(t, d, "AC1009")
	if !strings.Contains(out, "AC1009") || strings.Contains(out, "GROUP") || strings.Contains(out, "MTEXT") || strings.Contains(out, "AcDb") {
		t.Fatal("forced R12 output holds AutoCAD 2000 records")
	}
	joined := strings.Join(warns, "\n")
	if len(warns) != 2 || !strings.Contains(joined, "groups not written") || !strings.Contains(joined, "MTEXT written as TEXT") || !strings.Contains(joined, ": 2 objects") {
		t.Fatalf("warnings %q", warns)
	}
	back, _, err := (dxf.Codec{}).Read(strings.NewReader(out))
	if err != nil || len(back.Groups()) != 0 || back.Header.Version != "" {
		t.Fatalf("R12 read back: %v, groups %d, version %q", err, len(back.Groups()), back.Header.Version)
	}
	// With symbols but no groups, R12 forced keeps the blocks (R12 has them).
	sym := sample()
	tx, _ := sym.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "B", Entities: []doc.Entity{&doc.Circle{Radius: 1}}}); err != nil {
		t.Fatal(err)
	}
	tx.Add(doc.NewInsert("B", geom.V(5, 5)))
	tx.Commit()
	out, warns = writeVersion(t, sym, "AC1009")
	if !strings.Contains(out, "AC1009") || len(warns) > 0 {
		t.Fatalf("R12 with blocks: warnings %v", warns)
	}
	if back, _, _ := (dxf.Codec{}).Read(strings.NewReader(out)); len(back.Blocks()) != 1 {
		t.Fatal("block lost in R12")
	}
}

func TestReadKeepsTheVersion(t *testing.T) {
	for in, want := range map[string]string{"AC1009": "", "AC1006": "", "AC1014": "AC1014", "AC1015": "AC1015", "ac1032": "AC1032"} {
		src := "  0\nSECTION\n  2\nHEADER\n  9\n$ACADVER\n  1\n" + in + "\n  0\nENDSEC\n  0\nEOF\n"
		d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
		if err != nil || d.Header.Version != want {
			t.Errorf("$ACADVER %s: version %q, want %q (%v)", in, d.Header.Version, want, err)
		}
	}
}
