package main

import (
	"strings"
	"testing"
)

func TestParseScenario(t *testing.T) {
	sc, err := ParseScenario(strings.NewReader("# c\napp -demo -smoke 5\nsize 800 600\nclick right 10 20 # x\ntype \"l\\n a\"\nquit\n"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Width != 800 || len(sc.Steps) != 3 || sc.Steps[1].Args[0] != "l\n a" {
		t.Fatalf("%+v", sc)
	}
	if got := strings.Join(sc.AppFlags, " "); got != "-demo -smoke 5 -width 800 -height 600" {
		t.Fatal(got)
	}
	for _, bad := range []string{"bogus 1", "click", "type \"x", "size 1"} {
		if _, err := ParseScenario(strings.NewReader(bad)); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestCountDXF(t *testing.T) {
	dxf := "0\nSECTION\n2\nHEADER\n0\nENDSEC\n0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n0\nLINE\n0\nCIRCLE\n0\nENDSEC\n0\nEOF\n"
	c := CountDXF(dxf)
	if c["LINE"] != 2 || c["CIRCLE"] != 1 || c["SECTION"] != 0 {
		t.Fatal(c)
	}
}
