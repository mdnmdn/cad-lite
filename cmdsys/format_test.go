package cmdsys_test

import (
	"math"
	"testing"

	"cad-lite/cmdsys"
)

func TestFormatLength(t *testing.T) {
	for _, c := range []struct {
		v      float64
		lunits int
		prec   int
		want   string
	}{
		{5, 2, 4, "5.0000"},
		{-0.00001, 2, 4, "0.0000"},
		{15.5, 2, 0, "16"},
		{15.5, 1, 2, "1.55E+01"},
		{15.5, 3, 2, `1'-3.50"`},
		{11.999, 3, 2, `1'-0.00"`},
		{-27.25, 3, 2, `-2'-3.25"`},
		{15.5, 4, 4, `1'-3 1/2"`},
		{12, 4, 4, `1'-0"`},
		{3.4375, 4, 4, `0'-3 7/16"`},
		{15.5, 5, 4, "15 1/2"},
		{0.25, 5, 3, "1/4"},
		{0, 5, 3, "0"},
	} {
		if got := cmdsys.FormatLength(c.v, c.lunits, c.prec); got != c.want {
			t.Errorf("FormatLength(%v, %d, %d) = %q, want %q", c.v, c.lunits, c.prec, got, c.want)
		}
	}
}

func TestFormatAngle(t *testing.T) {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	for _, c := range []struct {
		deg    float64
		aunits int
		prec   int
		want   string
	}{
		{53.1301, 0, 0, "53"},
		{53.1301, 0, 2, "53.13"},
		{359.9, 0, 0, "0"},
		{53.1301, 1, 0, "53d"},
		{53.1301, 1, 2, "53d8'"},
		{53.1301, 1, 3, `53d7'48"`},
		{53.1301, 1, 4, `53d7'48.4"`},
		{90, 2, 1, "100.0g"},
		{180, 3, 4, "3.1416r"},
		{45, 4, 0, "N 45d E"},
		{135, 4, 0, "N 45d W"},
		{0, 4, 0, "E"},
		{270, 4, 0, "S"},
	} {
		if got := cmdsys.FormatAngle(rad(c.deg), c.aunits, c.prec); got != c.want {
			t.Errorf("FormatAngle(%v, %d, %d) = %q, want %q", c.deg, c.aunits, c.prec, got, c.want)
		}
	}
}

func TestParseAngleText(t *testing.T) {
	for _, c := range []struct {
		text   string
		aunits int
		deg    float64
		ok     bool
	}{
		{"45", 0, 45, true},
		{"-30.5", 0, -30.5, true},
		{"12d30'", 0, 12.5, true},
		{`10d30'36"`, 1, 10.51, true},
		{"100g", 0, 90, true},
		{"100", 2, 90, true},
		{"1r", 0, 180 / math.Pi, true},
		{"3.141592653589793", 3, 180, true},
		{"N 45d E", 4, 45, true},
		{"S 45d W", 4, 225, true},
		{"N 10d W", 4, 100, true},
		{"1,2", 0, 0, false},
		{"abc", 0, 0, false},
		{"", 0, 0, false},
	} {
		got, ok := cmdsys.ParseAngleText(c.text, c.aunits)
		if ok != c.ok || (ok && math.Abs(got*180/math.Pi-c.deg) > 1e-6) {
			t.Errorf("ParseAngleText(%q, %d) = %v, %v; want %v, %v", c.text, c.aunits, got*180/math.Pi, ok, c.deg, c.ok)
		}
	}
}

func TestUnitName(t *testing.T) {
	for code, want := range map[int]string{0: "", 1: "in", 4: "mm", 5: "cm", 6: "m", 24: "US mi", 25: "", -1: ""} {
		if got := cmdsys.UnitName(code); got != want {
			t.Errorf("UnitName(%d) = %q, want %q", code, got, want)
		}
	}
}
