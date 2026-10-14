package spatial_test

import (
	"math/rand/v2"
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/spatial"
)

// brute is the reference answer: every entity of d whose box meets box, in
// ascending handle order.
func brute(d *doc.Drawing, box geom.Box2) []doc.Handle {
	var out []doc.Handle
	for _, h := range d.Handles() {
		if d.Entity(h).Bounds().Intersects(box) {
			out = append(out, h)
		}
	}
	return out
}

func randLine(rng *rand.Rand, side float64) *doc.Line {
	a := geom.V(rng.Float64()*side, rng.Float64()*side)
	b := a.Add(geom.V(rng.Float64()*20-10, rng.Float64()*20-10))
	return &doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer}, Start: a, End: b}
}

func randBox(rng *rand.Rand, lo, hi float64) geom.Box2 {
	c := geom.V(lo+rng.Float64()*(hi-lo), lo+rng.Float64()*(hi-lo))
	return geom.Box2{Min: c, Max: c}.Grow(rng.Float64() * (hi - lo) / 4)
}

// check compares Query and Near with the brute-force answer on boxes that
// reach beyond the drawing on every side.
func check(t *testing.T, rng *rand.Rand, d *doc.Drawing, ix *spatial.Index, side float64) {
	t.Helper()
	if ix.Len() != d.Len() {
		t.Fatalf("Len = %d, drawing has %d", ix.Len(), d.Len())
	}
	for range 60 {
		box := randBox(rng, -side/2, side*1.5)
		if got, want := ix.Query(box), brute(d, box); !slices.Equal(got, want) {
			t.Fatalf("Query(%v): got %d handles, want %d\n got %v\nwant %v", box, len(got), len(want), got, want)
		}
		p := geom.V(rng.Float64()*side, rng.Float64()*side)
		want := brute(d, geom.Box2{Min: p, Max: p}.Grow(3))
		if got := ix.Near(p, 3); !slices.Equal(got, want) {
			t.Fatalf("Near(%v): got %v, want %v", p, got, want)
		}
	}
	all := geom.Box2{Min: geom.V(-1e9, -1e9), Max: geom.V(1e9, 1e9)}
	if got, want := ix.Query(all), brute(d, all); !slices.Equal(got, want) {
		t.Fatalf("Query(everything): got %d handles, want %d", len(got), len(want))
	}
}

func commit(t *testing.T, d *doc.Drawing, ix *spatial.Index, fn func(*doc.Txn)) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	fn(tx)
	ix.Apply(d, tx.Commit())
}

// The grid answers exactly what a linear scan answers, in ascending handle
// order, through builds, additions, moves, erasures, growth far beyond the
// original extent and shrinking back below the grid threshold.
func TestQueryMatchesBruteForceThroughEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	const side = 1000.0
	d := doc.New()
	for range 3000 {
		d.Load(randLine(rng, side))
	}
	// Large entities that span many cells, or the whole drawing.
	d.Load(&doc.Circle{EntityCommon: doc.EntityCommon{Layer: "0"}, Center: geom.V(500, 500), Radius: 700})
	d.Load(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, Start: geom.V(0, 0), End: geom.V(side, side)})
	d.Load(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, Start: geom.V(0, 400), End: geom.V(side, 401)})
	ix := spatial.New()
	ix.Build(d)
	check(t, rng, d, ix, side)

	for round := range 8 {
		hs := slices.Clone(d.Handles())
		commit(t, d, ix, func(tx *doc.Txn) {
			for range 200 {
				tx.Add(randLine(rng, side))
			}
			// Spanning additions, some far outside the built extent.
			tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"},
				Start: geom.V(-5000, rng.Float64()*side), End: geom.V(5000, rng.Float64()*side)})
			tx.Add(randLine(rng, side*float64(round+3)))
			for i := range 150 {
				h := hs[rng.IntN(len(hs))]
				tx.Modify(h, func(e doc.Entity) {
					dx := rng.Float64()*60 - 30
					if i%10 == 0 {
						dx = side * 3 // move far away, then maybe back
					}
					e.Transform(geom.Translate(geom.V(dx, rng.Float64()*60-30)))
				})
			}
			for range 100 {
				tx.Erase(hs[rng.IntN(len(hs))])
			}
		})
		check(t, rng, d, ix, side)
		// A small change set takes the entry-by-entry erase path.
		hs = slices.Clone(d.Handles())
		commit(t, d, ix, func(tx *doc.Txn) {
			tx.Erase(hs[0])
			tx.Erase(hs[len(hs)/2])
			tx.Modify(hs[len(hs)-1], func(e doc.Entity) { e.Transform(geom.Translate(geom.V(1, 1))) })
		})
		check(t, rng, d, ix, side)
	}

	// Shrink to a handful of entities: the index falls back to a scan.
	hs := slices.Clone(d.Handles())
	commit(t, d, ix, func(tx *doc.Txn) {
		for _, h := range hs[10:] {
			tx.Erase(h)
		}
	})
	check(t, rng, d, ix, side)
	// And grow back past the grid threshold through Apply alone.
	commit(t, d, ix, func(tx *doc.Txn) {
		for range 500 {
			tx.Add(randLine(rng, side))
		}
	})
	check(t, rng, d, ix, side)
}

// Handles that arrive out of order (a file loaded with its own handles,
// then edited) still come back ascending.
func TestQueryOrderWithOutOfOrderHandles(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))
	d := doc.New()
	for i := range 300 {
		l := randLine(rng, 100)
		l.H = doc.Handle(0x10000 - i*7)
		d.Load(l)
	}
	ix := spatial.New()
	ix.Build(d)
	check(t, rng, d, ix, 100)
	commit(t, d, ix, func(tx *doc.Txn) {
		for range 100 {
			tx.Add(randLine(rng, 100))
		}
	})
	check(t, rng, d, ix, 100)
}

// A degenerate drawing, every entity at one point or on one line, still
// gets a usable grid, and an entity with an empty box is counted but never
// returned.
func TestQueryDegenerateExtent(t *testing.T) {
	rng := rand.New(rand.NewPCG(9, 9))
	d := doc.New()
	for i := range 200 {
		x := float64(i)
		d.Load(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, Start: geom.V(x, 5), End: geom.V(x+1, 5)})
		d.Load(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, Start: geom.V(3, 3), End: geom.V(3, 3)})
	}
	// An unknown entity with no points has an empty box: indexed, never found.
	d.Load(&doc.Unknown{EntityCommon: doc.EntityCommon{Layer: "0"}, Type: "SPLINE"})
	ix := spatial.New()
	ix.Build(d)
	check(t, rng, d, ix, 200)
	if got := ix.Query(geom.EmptyBox()); got != nil {
		t.Fatalf("empty query returned %v", got)
	}
}

func TestEmptyIndex(t *testing.T) {
	ix := spatial.New()
	ix.Build(doc.New())
	if got := ix.Query(geom.BoxOf(geom.V(-1, -1), geom.V(1, 1))); len(got) != 0 {
		t.Fatalf("empty index returned %v", got)
	}
}
