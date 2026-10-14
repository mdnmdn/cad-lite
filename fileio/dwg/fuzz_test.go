package dwg

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzRead feeds arbitrary bytes to the importer. The reader promises to return a drawing or an
// error, never to panic, hang or allocate without bound. It calls importDrawing directly, with
// debugPanics on, so a runtime error in the parser fails the run instead of being turned into an
// "unreadable" count; Codec.Read adds a final recover for what this finds late.
//
//	go test ./fileio/dwg -fuzz FuzzRead -fuzztime 60s
func FuzzRead(f *testing.F) {
	seeds, _ := filepath.Glob("testdata/*.dwg")
	for _, s := range seeds {
		if b, err := os.ReadFile(s); err == nil {
			f.Add(b)
			// The first and last parts of a drawing hold the headers and the maps: small seeds
			// that reach the container code quickly.
			f.Add(b[:min(len(b), 0x500)])
			f.Add(b[max(0, len(b)-0x2000):])
		}
	}
	f.Add([]byte("AC1015"))
	f.Add([]byte("AC1032\x00\x00\x00\x00\x00\x00"))
	f.Add([]byte{})
	debugPanics = true
	ignoreCRC = true
	f.Fuzz(func(t *testing.T, data []byte) {
		d, _, err := importDrawing(data)
		if err == nil && d == nil {
			t.Fatal("neither a drawing nor an error")
		}
	})
}
