package dwg

import (
	"encoding/binary"
	"sort"
	"strings"
	"testing"
)

func TestOpenFileR2000(t *testing.T) {
	f, err := openFile(mustBytes(t, "testdata/sample_AC1015.dwg"))
	if err != nil {
		t.Fatal(err)
	}
	if f.magic != "AC1015" || f.ver != verR15 || f.codepg != 30 {
		t.Errorf("magic %q version %d code page %d", f.magic, f.ver, f.codepg)
	}
	// The six locator records: header, classes, object map, free space, template, aux header.
	for _, rec := range []int{0, 1, 2, 3, 4, 5} {
		if _, ok := f.records[rec]; !ok {
			t.Errorf("no section locator %d", rec)
		}
	}
	for name, min := range map[string]int{"AcDb:Header": 100, "AcDb:Classes": 100, "AcDb:Handles": 100} {
		b, err := f.section(name)
		if err != nil || len(b) < min {
			t.Errorf("section %s: %d bytes, %v", name, len(b), err)
		}
	}
	if b, _ := f.section("AcDb:NoSuchSection"); b != nil {
		t.Error("a section that does not exist was returned")
	}
}

func TestSectionMapR2004(t *testing.T) {
	for _, file := range []string{"sample_AC1018.dwg", "sample_AC1024.dwg", "sample_AC1027.dwg", "sample_AC1032.dwg", "empty_AC1032.dwg"} {
		f, err := openFile(mustBytes(t, "testdata/"+file))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if len(f.warns) != 0 {
			t.Errorf("%s: file header warnings %v", file, f.warns)
		}
		for _, name := range []string{"AcDb:Header", "AcDb:Classes", "AcDb:Handles", "AcDb:AcDbObjects"} {
			s := f.secs[name]
			if s == nil {
				t.Errorf("%s: no section %s (have %v)", file, name, sectionNames(f))
				continue
			}
			b, err := f.section(name)
			if err != nil {
				t.Errorf("%s: section %s: %v", file, name, err)
			}
			if len(b) != s.size && !(name == "AcDb:AcDbObjects") {
				// A section's pages add up to its declared size, except the last page may be
				// padded.
				if len(b) < s.size-s.pageMax || len(b) > s.size+s.pageMax {
					t.Errorf("%s: section %s has %d bytes, the map says %d", file, name, len(b), s.size)
				}
			}
			if !s.compressed {
				t.Errorf("%s: section %s is not marked compressed", file, name)
			}
		}
		if s := f.secs["AcDb:AcDbObjects"]; s.pageMax != 0x7400 || len(s.pages) < 1 {
			t.Errorf("%s: objects section page size %#x, %d pages", file, s.pageMax, len(s.pages))
		}
	}
}

func TestSectionMapR2007(t *testing.T) {
	f, err := openFile(mustBytes(t, "testdata/sample_AC1021.dwg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AcDb:Header", "AcDb:Classes", "AcDb:Handles", "AcDb:AcDbObjects"} {
		b, err := f.section(name)
		if err != nil || len(b) == 0 {
			t.Errorf("section %s: %d bytes, %v", name, len(b), err)
		}
	}
	if f.maint == 0 && f.codepg != 30 {
		t.Errorf("code page %d", f.codepg)
	}
}

func TestParseSectionMap(t *testing.T) {
	le := binary.LittleEndian
	desc := func(name string, size uint64, pages [][3]int) []byte {
		b := make([]byte, 96)
		le.PutUint64(b[0:], size)
		le.PutUint32(b[8:], uint32(len(pages)))
		le.PutUint32(b[12:], 0x7400)
		le.PutUint32(b[20:], 2) // compressed
		copy(b[32:], name)
		for _, p := range pages {
			e := make([]byte, 16)
			le.PutUint32(e[0:], uint32(p[0]))
			le.PutUint32(e[4:], uint32(p[1]))
			le.PutUint64(e[8:], uint64(p[2]))
			b = append(b, e...)
		}
		return b
	}
	head := make([]byte, 20)
	le.PutUint32(head, 2)
	m := append(head, desc("AcDb:Header", 700, [][3]int{{5, 400, 0}})...)
	m = append(m, desc("AcDb:Objects", 60000, [][3]int{{6, 300, 0}, {7, 200, 0x7400}})...)
	f := &file{data: make([]byte, 1<<16)}
	if err := f.parseSectionMap(m); err != nil {
		t.Fatal(err)
	}
	h := f.secs["AcDb:Header"]
	if h == nil || h.size != 700 || !h.compressed || h.pageMax != 0x7400 || len(h.pages) != 1 || h.pages[0] != (sectionPage{5, 400, 0}) {
		t.Errorf("header section = %+v", h)
	}
	o := f.secs["AcDb:Objects"]
	if o == nil || len(o.pages) != 2 || o.pages[1].offset != 0x7400 {
		t.Errorf("objects section = %+v", o)
	}
	// Damage: a count that does not fit, a size past the limit, and a truncated entry.
	for name, mod := range map[string]func([]byte){
		"count": func(b []byte) { le.PutUint32(b[20+8:], 1<<30) },
		"size":  func(b []byte) { le.PutUint64(b[20:], 1<<50) },
		"names": func(b []byte) { le.PutUint32(b[0:], 1000) },
	} {
		bad := append([]byte(nil), m...)
		mod(bad)
		if err := (&file{data: make([]byte, 1<<16)}).parseSectionMap(bad); err == nil {
			t.Errorf("damaged map (%s) was accepted", name)
		}
	}
	if err := (&file{data: make([]byte, 16)}).parseSectionMap(m[:30]); err == nil {
		t.Error("a truncated map was accepted")
	}
}

func TestObjectMap(t *testing.T) {
	// Two chunks: handles 1, 3 and 0x103 with offsets 100, 40 (a step of -60) and 5000; then the
	// empty terminating chunk of size 2.
	chunk := func(pairs ...int64) []byte {
		w := &bitWriter{}
		for i := 0; i < len(pairs); i += 2 {
			w.mc(uint64(pairs[i]))
			w.smc(pairs[i+1])
		}
		body := w.bytes()
		out := []byte{byte((len(body) + 2) >> 8), byte(len(body) + 2)}
		out = append(out, body...)
		return append(out, 0xAA, 0xBB) // CRC
	}
	var sec []byte
	sec = append(sec, chunk(1, 100, 2, -60, 0x100, 4960)...)
	sec = append(sec, chunk(5, 7)...)
	sec = append(sec, 0x00, 0x02, 0xFF, 0xFF)
	f := &file{ver: verR15, records: map[int]locator{2: {0, len(sec)}}, data: sec, cache: map[string][]byte{}}
	m, err := f.readObjectMap()
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint64]int{1: 100, 3: 40, 0x103: 5000, 0x108: 5007 - 0}
	// The second chunk starts again from handle 0 and offset 0.
	want = map[uint64]int{1: 100, 3: 40, 0x103: 5000, 5: 7}
	if len(m) != len(want) {
		t.Errorf("map = %v, want %v", m, want)
	}
	for h, off := range want {
		if m[h] != off {
			t.Errorf("handle %#x at %d, want %d", h, m[h], off)
		}
	}
	// A truncated section keeps what was read.
	f.cache = map[string][]byte{}
	f.data = sec[:len(sec)-12]
	f.records[2] = locator{0, len(f.data)}
	if m, _ := f.readObjectMap(); len(m) == 0 {
		t.Error("a truncated object map lost every entry")
	}
	// A section with no data at all is an error.
	f.records = map[int]locator{}
	f.cache = map[string][]byte{}
	if _, err := f.readObjectMap(); err == nil {
		t.Error("a missing object map was accepted")
	}
}

func TestClassesSection(t *testing.T) {
	w := &bitWriter{}
	for _, c := range []struct {
		num    int16
		dxf    string
		entity bool
	}{{500, "LWPOLYLINE", true}, {501, "DICTIONARYVAR", false}, {502, "HATCH", true}} {
		w.bs(c.num)
		w.bs(0)
		w.tv("ObjectDBX Classes")
		w.tv("AcDbX" + c.dxf)
		w.tv(c.dxf)
		w.bit(false)
		if c.entity {
			w.bs(0x1F2)
		} else {
			w.bs(0x1F3)
		}
	}
	w.pad()
	sec := make([]byte, 16)
	sec = binary.LittleEndian.AppendUint32(sec, uint32(len(w.bytes())))
	sec = append(sec, w.bytes()...)
	sec = append(sec, make([]byte, 18)...) // CRC and end sentinel
	f := &file{ver: verR15, codepg: 30, records: map[int]locator{1: {0, len(sec)}}, data: sec, cache: map[string][]byte{}}
	classes, err := f.readClasses()
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) != 3 || classes[500].dxfName != "LWPOLYLINE" || !classes[500].entity || classes[501].entity || classes[502].dxfName != "HATCH" || classes[500].cppName != "AcDbXLWPOLYLINE" {
		t.Errorf("classes = %+v", classes)
	}
}

func TestVersionDetection(t *testing.T) {
	cases := map[string]string{
		"MC0.0": "1.0", "AC1.50": "2.0", "AC1002": "2.5", "AC1006": "R10", "AC1009": "R11/R12",
	}
	for magic, release := range cases {
		data := append([]byte(magic), make([]byte, 200)...)
		_, err := openFile(data)
		var u *unsupportedError
		if err == nil || !asUnsupported(err, &u) || u.magic != magic {
			t.Errorf("%s: got %v, want an unsupported version error", magic, err)
			continue
		}
		if releaseNames[magic] != release {
			t.Errorf("%s: release %q, want %q", magic, releaseNames[magic], release)
		}
	}
	for _, bad := range [][]byte{nil, []byte("AC"), []byte("PK\x03\x04 not a drawing"), []byte("AC9999" + strings.Repeat("\x00", 100))} {
		if _, err := openFile(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	// A recognised version with a file too short to hold its header is an error, not a panic.
	for _, magic := range []string{"AC1012", "AC1014", "AC1015", "AC1018", "AC1021", "AC1024", "AC1027", "AC1032"} {
		if _, err := openFile([]byte(magic + "\x00\x00\x00")); err == nil {
			t.Errorf("%s: a six-byte file was accepted", magic)
		}
	}
}

func asUnsupported(err error, target **unsupportedError) bool {
	u, ok := err.(*unsupportedError)
	if ok {
		*target = u
	}
	return ok
}

// sectionNames lists the sections of an R2004-or-later file, sorted.
func sectionNames(f *file) []string {
	var n []string
	for k := range f.secs {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
