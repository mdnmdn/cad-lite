package cmdsys_test

import (
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/spatial"
)

// Unknown entities are indexed by their point groups but must never be
// picked, boxed or snapped to.
func TestUnknownEntitiesAreNotSelectableOrSnappable(t *testing.T) {
	d := doc.New()
	d.Load(&doc.Unknown{
		EntityCommon: doc.EntityCommon{H: 0x40, Layer: "0", Color: doc.ByLayer},
		Type:         "SPLINE",
		Raw:          []doc.Pair{{Code: 10, Value: "5"}, {Code: 20, Value: "5"}, {Code: 11, Value: "6"}, {Code: 21, Value: "6"}},
	})
	ix := spatial.New()
	ix.Build(d)
	if got := ix.Query(geom.BoxOf(geom.V(0, 0), geom.V(10, 10))); len(got) != 1 {
		t.Fatalf("index holds %v", got)
	}
	if h, ok := cmdsys.Pick(d, ix, geom.V(5, 5), 1); ok {
		t.Fatalf("picked unknown %v", h)
	}
	if hs := cmdsys.SelectBox(d, ix, geom.V(0, 0), geom.V(10, 10), 0.01); len(hs) != 0 {
		t.Fatalf("window selected %v", hs)
	}
	if hs := cmdsys.SelectBox(d, ix, geom.V(10, 10), geom.V(0, 0), 0.01); len(hs) != 0 {
		t.Fatalf("crossing selected %v", hs)
	}
	if hit, ok := cmdsys.FindSnap(d, ix, geom.V(5, 5), 1, cmdsys.SnapEndpoint|cmdsys.SnapNearest); ok {
		t.Fatalf("snapped to unknown: %+v", hit)
	}
}
