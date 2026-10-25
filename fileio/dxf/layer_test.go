package dxf_test

import (
	"bytes"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
)

func TestLayerFlagsRoundTrip(t *testing.T) {
	src := doc.New()
	f := src.AddLayer("FROZEN", 2)
	f.Frozen, f.Locked = true, true
	p := src.AddLayer("PLAN", 3)
	p.NoPlot, p.Lineweight, p.Description = true, 35, "Floor plan, level 1"
	var buf bytes.Buffer
	if _, err := (dxf.Codec{}).Write(&buf, src); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "CADLITE_LAYER") {
		t.Fatal("extended data application not registered")
	}
	got, _, err := (dxf.Codec{}).Read(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if l := got.Layer("FROZEN"); l == nil || !l.Frozen || !l.Locked || l.Off {
		t.Fatalf("FROZEN = %+v", l)
	}
	if l := got.Layer("PLAN"); l == nil || !l.NoPlot || l.Lineweight != 35 || l.Description != "Floor plan, level 1" || l.Frozen {
		t.Fatalf("PLAN = %+v", l)
	}
	// A second round trip must not duplicate the application record.
	var again bytes.Buffer
	if _, err := (dxf.Codec{}).Write(&again, got); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(again.String(), "CADLITE_LAYER"); n != strings.Count(text, "CADLITE_LAYER") {
		t.Fatalf("CADLITE_LAYER appears %d times after a second save, want %d", n, strings.Count(text, "CADLITE_LAYER"))
	}
}

func TestLayerNativeR2000Fields(t *testing.T) {
	in := "0\nSECTION\n2\nTABLES\n0\nTABLE\n2\nLAYER\n70\n1\n0\nLAYER\n2\nA\n70\n1\n62\n5\n6\nDASHED\n290\n0\n370\n50\n0\nENDTAB\n0\nENDSEC\n0\nEOF\n"
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	l := d.Layer("A")
	if l == nil || !l.Frozen || !l.NoPlot || l.Lineweight != 50 || l.Linetype != "DASHED" || l.Color != 5 {
		t.Fatalf("A = %+v", l)
	}
}

func TestLayerLegacyExtendedDataApp(t *testing.T) {
	in := "0\nSECTION\n2\nTABLES\n0\nTABLE\n2\nAPPID\n70\n1\n0\nAPPID\n2\nGODRAW_LAYER\n70\n0\n0\nENDTAB\n" +
		"0\nTABLE\n2\nLAYER\n70\n1\n0\nLAYER\n2\nPLAN\n70\n0\n62\n3\n6\nCONTINUOUS\n" +
		"1001\nGODRAW_LAYER\n1070\n1\n1070\n35\n1000\nOld plan\n0\nENDTAB\n0\nENDSEC\n0\nEOF\n"
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if l := d.Layer("PLAN"); l == nil || !l.NoPlot || l.Lineweight != 35 || l.Description != "Old plan" {
		t.Fatalf("PLAN = %+v", l)
	}
	var buf bytes.Buffer
	if _, err := (dxf.Codec{}).Write(&buf, d); err != nil {
		t.Fatal(err)
	}
	if text := buf.String(); strings.Contains(text, "GODRAW_LAYER") || !strings.Contains(text, "CADLITE_LAYER") {
		t.Fatal("a file saved after reading the legacy application name must carry only CADLITE_LAYER")
	}
}

// layerRecord returns the groups of the named LAYER record in a written file, up to the next
// record.
func layerRecord(t *testing.T, text, name string) string {
	t.Helper()
	i := strings.Index(text, "AcDbLayerTableRecord\n  2\n"+name+"\n")
	if i < 0 {
		i = strings.Index(text, "LAYER\n  2\n"+name+"\n")
	}
	if i < 0 {
		t.Fatalf("no LAYER record %q", name)
	}
	rest := text[i:]
	if j := strings.Index(rest, "\n  0\n"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

// TestLayerLineweightDefaultAndZero checks that a layer's Default lineweight is written as DXF
// 370 -3, not 0 (0.00 mm to AutoCAD), and that a 0.00 mm layer survives a round trip in R2000
// and R12 (through CADLITE_LAYER extended data).
func TestLayerLineweightDefaultAndZero(t *testing.T) {
	src := doc.New()
	src.AddLayer("DEF", 1)
	src.AddLayer("ZERO", 2).Lineweight = doc.LineweightZero.LayerValue()
	src.AddLayer("W50", 3).Lineweight = 50
	for _, ver := range []string{"AC1015", "AC1009"} {
		var buf bytes.Buffer
		if _, err := (dxf.Codec{}).WriteVersion(&buf, src, ver); err != nil {
			t.Fatal(err)
		}
		text := buf.String()
		if ver == "AC1015" {
			if rec := layerRecord(t, text, "DEF"); !strings.Contains(rec, "\n370\n-3\n") {
				t.Errorf("Default layer lineweight not written as 370 -3:\n%s", rec)
			}
			if rec := layerRecord(t, text, "ZERO"); !strings.Contains(rec, "\n370\n0\n") {
				t.Errorf("0.00 mm layer lineweight not written as 370 0:\n%s", rec)
			}
		}
		got, _, err := (dxf.Codec{}).Read(strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]doc.Lineweight{"DEF": doc.LineweightDefault, "ZERO": doc.LineweightZero, "W50": 50} {
			if l := got.Layer(name); l == nil || doc.LineweightFromLayer(l.Lineweight) != want {
				t.Errorf("%s: layer %s lineweight %+v, want %v", ver, name, l, want)
			}
		}
	}
}

// TestLayerNativeLineweightZero reads AutoCAD's 370 0 (0.00 mm) and 370 -3 (Default) on layers.
func TestLayerNativeLineweightZero(t *testing.T) {
	in := "0\nSECTION\n2\nTABLES\n0\nTABLE\n2\nLAYER\n70\n2\n" +
		"0\nLAYER\n2\nZ\n70\n0\n62\n5\n6\nCONTINUOUS\n370\n0\n" +
		"0\nLAYER\n2\nD\n70\n0\n62\n5\n6\nCONTINUOUS\n370\n-3\n" +
		"0\nENDTAB\n0\nENDSEC\n0\nEOF\n"
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if l := d.Layer("Z"); l == nil || doc.LineweightFromLayer(l.Lineweight) != doc.LineweightZero {
		t.Errorf("Z = %+v, want 0.00 mm", l)
	}
	if l := d.Layer("D"); l == nil || l.Lineweight != 0 {
		t.Errorf("D = %+v, want Default", l)
	}
}
