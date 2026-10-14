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
