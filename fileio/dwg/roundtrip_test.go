package dwg

import (
	"bytes"
	"testing"

	"cad-lite/fileio/dxf"
)

// TestRoundTripThroughDXF reads each sample DWG, writes it with the DXF writer, reads that DXF
// back and expects the same entities and layers: the DWG reader fills the document the way the
// writer needs it.
func TestRoundTripThroughDXF(t *testing.T) {
	for _, fx := range fixtures {
		t.Run(fx.file, func(t *testing.T) {
			src := mustRead(t, "testdata/"+fx.file, Codec{})
			var buf bytes.Buffer
			if _, err := (dxf.Codec{}).Write(&buf, src); err != nil {
				t.Fatal(err)
			}
			back, _, err := (dxf.Codec{}).Read(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := classCounts(back), classCounts(src); len(got) != len(want) {
				t.Errorf("classes after the round trip: %v, want %v", got, want)
			} else {
				for c, n := range want {
					if got[c] != n {
						t.Errorf("%s: %d after the round trip, want %d", c, got[c], n)
					}
				}
			}
			if back.Len() != src.Len() || back.Len() != 56 {
				t.Errorf("%d entities after the round trip, %d before", back.Len(), src.Len())
			}
			if len(back.Layers()) != len(src.Layers()) {
				t.Errorf("%d layers after the round trip, %d before", len(back.Layers()), len(src.Layers()))
			}
			for _, h := range src.Handles() {
				a, b := src.Entity(h), back.Entity(h)
				if b == nil {
					t.Errorf("handle %s lost", h)
					continue
				}
				if a.Common().Layer != b.Common().Layer || a.Common().Color != b.Common().Color {
					t.Errorf("%s %s: layer/colour %q/%d became %q/%d", a.Class(), h, a.Common().Layer, a.Common().Color, b.Common().Layer, b.Common().Color)
				}
				if !sameGeometry(a, b) {
					t.Errorf("%s %s: geometry changed:\n %+v\n %+v", a.Class(), h, a, b)
				}
			}
			// The R12 writer carries no linetype definitions, so a layer's linetype name does not
			// survive, and it writes no $INSUNITS.
			for i, l := range src.Layers() {
				m := back.Layers()[i]
				want := *l
				want.Linetype = ""
				if want != *m {
					t.Errorf("layer %q changed: %+v -> %+v", l.Name, *l, *m)
				}
			}
		})
	}
}
