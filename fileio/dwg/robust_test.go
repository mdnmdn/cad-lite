package dwg

import (
	"bytes"
	"math/rand"
	"testing"
	"time"
)

// TestMutatedFiles feeds damaged copies of each sample to the reader: single bit flips, random
// spans, and truncations, in the headers, the section map and the data. Every one must come back
// as a drawing or an error, quickly, and with debugPanics on a parser bug (an index out of range)
// fails the test instead of being reported as corrupt data.
func TestMutatedFiles(t *testing.T) {
	debugPanics = true
	defer func() { debugPanics = false }()
	rounds := 40
	if testing.Short() {
		rounds = 8
	}
	files := []string{"sample_AC1014.dwg", "sample_AC1015.dwg", "sample_AC1018.dwg", "sample_AC1021.dwg", "sample_AC1024.dwg", "empty_AC1032.dwg"}
	for _, file := range files {
		data := mustBytes(t, "testdata/"+file)
		rng := rand.New(rand.NewSource(int64(len(data))))
		for i := 0; i < rounds; i++ {
			m := append([]byte(nil), data...)
			// Aim a third of the damage at the first kilobyte (file header, locators) and the last
			// 4 KB (page and section maps of the later versions), where it matters most.
			pick := func() int {
				switch i % 3 {
				case 0:
					return rng.Intn(min(len(m), 1024))
				case 1:
					return len(m) - 1 - rng.Intn(min(len(m), 4096))
				}
				return rng.Intn(len(m))
			}
			switch i % 4 {
			case 0, 1:
				for k := 0; k < 1+rng.Intn(16); k++ {
					m[pick()] ^= byte(1 << rng.Intn(8))
				}
			case 2:
				a := pick()
				for k := a; k < min(len(m), a+rng.Intn(2048)); k++ {
					m[k] = byte(rng.Intn(256))
				}
			case 3:
				m = m[:pick()+1]
			}
			start := time.Now()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s round %d: panic %v", file, i, r)
					}
				}()
				d, _, err := importDrawing(m)
				if err == nil && d == nil {
					t.Errorf("%s round %d: neither a drawing nor an error", file, i)
				}
			}()
			if took := time.Since(start); took > 5*time.Second {
				t.Errorf("%s round %d took %v", file, i, took)
			}
		}
	}
}

// TestHugeClaims checks that sizes a damaged file claims do not turn into allocations: a section
// that says it is a gigabyte, a page count in the billions.
func TestHugeClaims(t *testing.T) {
	data := mustBytes(t, "testdata/sample_AC1018.dwg")
	f, err := openFile(data)
	if err != nil {
		t.Fatal(err)
	}
	s := f.secs["AcDb:AcDbObjects"]
	s.size = 1 << 40
	for i := range s.pages {
		s.pages[i].offset = 1 << 40
	}
	delete(f.cache, "AcDb:AcDbObjects")
	if _, err := f.section("AcDb:AcDbObjects"); err == nil {
		t.Error("a page placed a terabyte into the section")
	}
	// The same through the front door: patch nothing, just read it under a memory cap by timing.
	start := time.Now()
	if _, _, err := (Codec{}).Read(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("a normal read is slow")
	}
}

// TestMutatedObjects damages the decoded object data of each sample, past the compression and the
// CRC checks, so the entity and table parsers meet records that are wrong in every way a bit flip
// can make them.
func TestMutatedObjects(t *testing.T) {
	debugPanics, ignoreCRC = true, true
	defer func() { debugPanics, ignoreCRC = false, false }()
	rounds := 60
	if testing.Short() {
		rounds = 10
	}
	for _, file := range []string{"sample_AC1014.dwg", "sample_AC1015.dwg", "sample_AC1018.dwg", "sample_AC1021.dwg", "sample_AC1024.dwg", "sample_AC1032.dwg"} {
		rng := rand.New(rand.NewSource(7))
		f0, err := openFile(mustBytes(t, "testdata/"+file))
		if err != nil {
			t.Fatal(err)
		}
		var objects []byte
		if f0.ver <= verR15 {
			objects = f0.data
		} else if objects, err = f0.section("AcDb:AcDbObjects"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < rounds; i++ {
			f, err := openFile(f0.data)
			if err != nil {
				t.Fatal(err)
			}
			m := append([]byte(nil), objects...)
			for k := 0; k < 1+rng.Intn(24); k++ {
				m[rng.Intn(len(m))] ^= byte(1 << rng.Intn(8))
			}
			if f.ver <= verR15 {
				f.data = m
			} else {
				f.cache["AcDb:AcDbObjects"] = m
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s round %d: panic %v", file, i, r)
					}
				}()
				d, _, err := importFile(f)
				if err == nil && d == nil {
					t.Errorf("%s round %d: neither a drawing nor an error", file, i)
				}
			}()
		}
	}
}
