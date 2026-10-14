package spatial_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/spatial"
)

// benchDrawing returns a drawing of n short lines scattered over a square
// whose side grows with n, so density stays roughly constant, plus a few
// long lines that cross the whole extent.
func benchDrawing(n int) (*doc.Drawing, float64) {
	rng := rand.New(rand.NewPCG(1, uint64(n)))
	side := 100 * float64(intSqrt(n))
	d := doc.New()
	for i := range n {
		var a, b geom.Vec2
		if i%1000 == 999 {
			a, b = geom.V(0, rng.Float64()*side), geom.V(side, rng.Float64()*side)
		} else {
			a = geom.V(rng.Float64()*side, rng.Float64()*side)
			b = a.Add(geom.V(rng.Float64()*40-20, rng.Float64()*40-20))
		}
		d.Load(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer}, Start: a, End: b})
	}
	return d, side
}

func intSqrt(n int) int {
	r := 1
	for r*r < n {
		r++
	}
	return r
}

// BenchmarkQuery measures one viewport query on drawings of 10k and 100k
// entities: a zoomed-in view (about 0.1% of the extent), a working view
// (about 4%) and the whole drawing (zoom extents).
func BenchmarkQuery(b *testing.B) {
	for _, n := range []int{10_000, 100_000} {
		d, side := benchDrawing(n)
		ix := spatial.New()
		ix.Build(d)
		c := geom.V(side/2, side/2)
		views := []struct {
			name string
			half float64
		}{
			{"zoomed", side * 0.016},
			{"working", side * 0.1},
			{"extents", side * 0.6},
		}
		for _, v := range views {
			box := geom.Box2{Min: c, Max: c}.Grow(v.half)
			b.Run(fmt.Sprintf("n=%d/%s", n, v.name), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_ = ix.Query(box)
				}
			})
		}
		b.Run(fmt.Sprintf("n=%d/near", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = ix.Near(c, 5)
			}
		})
	}
}
