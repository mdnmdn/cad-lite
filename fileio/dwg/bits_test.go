package dwg

import (
	"math"
	"testing"
)

// roundTrip writes with fn at every bit offset 0 to 7, then reads the same bits back with check.
func atEveryShift(t *testing.T, name string, write func(*bitWriter), read func(*bitReader) bool) {
	t.Helper()
	for shift := 0; shift < 8; shift++ {
		w := &bitWriter{}
		w.bits(0, shift)
		write(w)
		w.bits(0b1011, 4) // a marker after the value
		r := newBitReader(w.bytes())
		r.seekBit(shift)
		if !read(r) {
			t.Errorf("%s at bit shift %d: wrong value", name, shift)
			continue
		}
		if got := r.bits(4); got != 0b1011 {
			t.Errorf("%s at bit shift %d: read the wrong number of bits (marker %04b)", name, shift, got)
		}
	}
}

func TestBitShort(t *testing.T) {
	for _, v := range []int16{0, 1, 7, 255, 256, 257, -1, 1000, -32768, 32767} {
		atEveryShift(t, "BS", func(w *bitWriter) { w.bs(v) }, func(r *bitReader) bool { return r.bs() == v })
	}
}

func TestBitShortCodes(t *testing.T) {
	// The four codes: 00 short follows, 01 byte follows, 10 zero, 11 256.
	for _, c := range []struct {
		bits  string
		bytes []byte
		want  int16
	}{
		{"00", []byte{0x34, 0x12}, 0x1234},
		{"01", []byte{0x7f}, 0x7f},
		{"10", nil, 0},
		{"11", nil, 256},
	} {
		w := &bitWriter{}
		for _, ch := range c.bits {
			w.bit(ch == '1')
		}
		for _, b := range c.bytes {
			w.rc(b)
		}
		if got := newBitReader(w.bytes()).bs(); got != c.want {
			t.Errorf("BS code %s: got %d, want %d", c.bits, got, c.want)
		}
	}
}

func TestBitLong(t *testing.T) {
	for _, v := range []int32{0, 1, 255, 256, -1, 70000, math.MinInt32, math.MaxInt32} {
		atEveryShift(t, "BL", func(w *bitWriter) { w.bl(v) }, func(r *bitReader) bool { return r.bl() == v })
	}
}

func TestBitLongInvalidCode(t *testing.T) {
	if err := safe(func() { newBitReader([]byte{0xC0}).bl() }); err != errCorrupt {
		t.Errorf("BL code 11 is not used: got %v", err)
	}
}

func TestBitLongLong(t *testing.T) {
	for _, v := range []uint64{0, 1, 0xff, 0x100, 0x123456789a, 1<<56 - 1} {
		atEveryShift(t, "BLL", func(w *bitWriter) { w.bll(v) }, func(r *bitReader) bool { return r.bll() == v })
	}
}

func TestBitDouble(t *testing.T) {
	for _, v := range []float64{0, 1, -1, 3.5, math.Pi, -1e300, math.SmallestNonzeroFloat64} {
		atEveryShift(t, "BD", func(w *bitWriter) { w.bd(v) }, func(r *bitReader) bool { return r.bd() == v })
	}
}

func TestBitDoubleWithDefault(t *testing.T) {
	const def = 10.5
	// 00: the default.
	atEveryShift(t, "DD 00", func(w *bitWriter) { w.dd(def, def) }, func(r *bitReader) bool { return r.dd(def) == def })
	// 11: a full double.
	atEveryShift(t, "DD 11", func(w *bitWriter) { w.dd(-77.25, def) }, func(r *bitReader) bool { return r.dd(def) == -77.25 })
	// 01: the first four bytes of the default replaced.
	patched := math.Float64frombits(math.Float64bits(def)&^0xFFFFFFFF | 0xDEADBEEF)
	atEveryShift(t, "DD 01", func(w *bitWriter) {
		w.bb(1)
		for _, b := range []byte{0xEF, 0xBE, 0xAD, 0xDE} {
			w.rc(b)
		}
	}, func(r *bitReader) bool { return r.dd(def) == patched })
	// 10: bytes 4 and 5 replaced first, then bytes 0 to 3.
	bits := math.Float64bits(def)&^0xFFFF00000000 | 0x1234<<32
	bits = bits&^0xFFFFFFFF | 0xDEADBEEF
	atEveryShift(t, "DD 10", func(w *bitWriter) {
		w.bb(2)
		for _, b := range []byte{0x34, 0x12, 0xEF, 0xBE, 0xAD, 0xDE} {
			w.rc(b)
		}
	}, func(r *bitReader) bool { return math.Float64bits(r.dd(def)) == bits })
}

func TestModularChar(t *testing.T) {
	for _, v := range []uint64{0, 1, 0x7f, 0x80, 0x3fff, 0x4000, 1 << 40, math.MaxUint64 >> 1} {
		atEveryShift(t, "MC", func(w *bitWriter) { w.mc(v) }, func(r *bitReader) bool { return r.mc() == v })
	}
	// A known encoding: 0x80 is two bytes, 0x80 0x01.
	if got := newBitReader([]byte{0x80, 0x01}).mc(); got != 0x80 {
		t.Errorf("MC 80 01 = %#x, want 0x80", got)
	}
}

func TestSignedModularChar(t *testing.T) {
	for _, v := range []int64{0, 1, -1, 63, -63, 64, -64, 1000, -1000, 1 << 30, -(1 << 30)} {
		atEveryShift(t, "SMC", func(w *bitWriter) { w.smc(v) }, func(r *bitReader) bool { return r.smc() == v })
	}
	// 0x41 is the one-byte form of -1: sign bit 0x40 set, value 1.
	if got := newBitReader([]byte{0x41}).smc(); got != -1 {
		t.Errorf("SMC 41 = %d, want -1", got)
	}
}

func TestModularShort(t *testing.T) {
	for _, v := range []int{0, 1, 0x7fff, 0x8000, 0x12345, 1 << 29} {
		atEveryShift(t, "MS", func(w *bitWriter) { w.ms(v) }, func(r *bitReader) bool { return r.ms() == v })
	}
	// 0x34 0x12 is the single word 0x1234.
	if got := newBitReader([]byte{0x34, 0x12}).ms(); got != 0x1234 {
		t.Errorf("MS 34 12 = %#x", got)
	}
}

func TestModularOverlong(t *testing.T) {
	long := make([]byte, 40)
	for i := range long {
		long[i] = 0xFF
	}
	for name, f := range map[string]func(*bitReader){
		"MC":  func(r *bitReader) { r.mc() },
		"SMC": func(r *bitReader) { r.smc() },
		"MS":  func(r *bitReader) { r.ms() },
	} {
		if err := safe(func() { f(newBitReader(long)) }); err != errCorrupt {
			t.Errorf("%s of an endless run: got %v, want errCorrupt", name, err)
		}
	}
}

func TestHandle(t *testing.T) {
	for _, v := range []uint64{0, 1, 0x1F, 0x100, 0xABCDEF, 1 << 40} {
		atEveryShift(t, "H", func(w *bitWriter) { w.handle(5, v) }, func(r *bitReader) bool { return r.handle(0) == v })
	}
	// Relative forms: 6 is +1, 8 is -1, 0xA adds the stored offset, 0xC subtracts it.
	rel := func(form byte, stored ...byte) uint64 {
		return newBitReader(append([]byte{form}, stored...)).handle(100)
	}
	for _, c := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"+1", rel(0x60), 101}, {"-1", rel(0x80), 99},
		{"+off", rel(0xA1, 5), 105}, {"-off", rel(0xC2, 0x00, 0x0A), 90},
		{"abs", rel(0x42, 0x01, 0x02), 0x0102},
	} {
		if c.got != c.want {
			t.Errorf("handle %s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if err := safe(func() { newBitReader([]byte{0xF1, 0x00}).handle(0) }); err != errCorrupt {
		t.Errorf("handle code 0xF is invalid: got %v", err)
	}
	if err := safe(func() { newBitReader([]byte{0x59, 0, 0, 0, 0, 0, 0, 0, 0, 0}).handle(0) }); err != errCorrupt {
		t.Errorf("handle longer than 8 bytes: got %v", err)
	}
}

func TestText(t *testing.T) {
	atEveryShift(t, "TV", func(w *bitWriter) { w.tv("Layer_1") }, func(r *bitReader) bool { return r.tv(30) == "Layer_1" })
	atEveryShift(t, "TU", func(w *bitWriter) { w.tu("Schön €") }, func(r *bitReader) bool { return r.tu() == "Schön €" })
	atEveryShift(t, "empty TV", func(w *bitWriter) { w.bs(0) }, func(r *bitReader) bool { return r.tv(30) == "" })
	// Windows-1252 maps 0x80 to the euro sign and 0xE9 to e acute; the same bytes in ISO 8859-1
	// are a control character and e acute.
	w := &bitWriter{}
	w.bs(4)
	w.rc(0x80)
	w.rc(0xE9)
	w.rc('x')
	w.rc(0)
	if got := newBitReader(w.bytes()).tv(30); got != "€éx" {
		t.Errorf("TV in code page 30 = %q", got)
	}
	// Text stops at the first NUL.
	w = &bitWriter{}
	w.bs(5)
	for _, b := range []byte{'a', 'b', 0, 'c', 0} {
		w.rc(b)
	}
	if got := newBitReader(w.bytes()).tv(30); got != "ab" {
		t.Errorf("TV with an embedded NUL = %q, want ab", got)
	}
}

func TestBitExtrusionAndThickness(t *testing.T) {
	// R2000: one bit says (0,0,1) / thickness 0; otherwise the value follows.
	w := &bitWriter{}
	w.bit(true)
	w.bit(false)
	w.bd(0.5)
	w.bit(true)
	w.bit(false)
	w.bd(0)
	w.bd(0)
	w.bd(-1)
	r := newBitReader(w.bytes())
	if x, y, z := r.bitExtrusion(); x != 0 || y != 0 || z != 1 {
		t.Errorf("default extrusion = %v %v %v", x, y, z)
	}
	if got := r.bitThickness(); got != 0.5 {
		t.Errorf("thickness = %v", got)
	}
	if got := r.bitThickness(); got != 0 {
		t.Errorf("default thickness = %v", got)
	}
	if x, y, z := r.bitExtrusion(); x != 0 || y != 0 || z != -1 {
		t.Errorf("explicit extrusion = %v %v %v", x, y, z)
	}
	// R13 and R14 (plain): the values are always present.
	w = &bitWriter{}
	w.bd(2)
	w.bd(0)
	w.bd(0)
	w.bd(1)
	w.bd(2)
	r = newBitReader(w.bytes())
	r.plain = true
	if got := r.bitThickness(); got != 2 {
		t.Errorf("plain thickness = %v", got)
	}
	if x, y, z := r.bitExtrusion(); x != 0 || y != 0 || z != 1 {
		t.Errorf("plain extrusion = %v %v %v", x, y, z)
	}
}

func TestBitReaderBounds(t *testing.T) {
	r := newBitReader([]byte{0xAA})
	r.bits(8)
	for name, f := range map[string]func(){
		"bit":   func() { r.bit() },
		"rc":    func() { r.rc() },
		"rd":    func() { r.rd() },
		"bd":    func() { r.bd() },
		"bytes": func() { r.bytes(1) },
	} {
		if err := safe(f); err != errTruncated {
			t.Errorf("%s past the end: got %v, want errTruncated", name, err)
		}
	}
	// A huge byte count must fail before allocating.
	if err := safe(func() { newBitReader(make([]byte, 4)).bytes(1 << 40) }); err != errTruncated {
		t.Errorf("bytes(1<<40) = %v", err)
	}
	if err := safe(func() { newBitReader(make([]byte, 4)).bytes(-1) }); err != errTruncated {
		t.Errorf("bytes(-1) = %v", err)
	}
	// A reader bounded below the data stops there.
	r = newBitReader([]byte{1, 2, 3, 4})
	r.end = 16
	r.rc()
	r.rc()
	if err := safe(func() { r.rc() }); err != errTruncated {
		t.Errorf("read past the bound: %v", err)
	}
}

func TestStringStream(t *testing.T) {
	// A record whose strings sit at its end: [main data][strings][size of strings (16 bits)][flag].
	var w bitWriter
	w.bits(0b1010, 4) // some main data
	strStart := w.n
	w.tu("abc")
	size := w.n - strStart
	w.rs(int16(size))
	flagPos := w.n
	w.bit(true)
	w.pad()
	main := newBitReader(w.bytes())
	text, start := stringStream(main, flagPos)
	if start != strStart {
		t.Fatalf("string stream starts at bit %d, want %d", start, strStart)
	}
	if got := text.tu(); got != "abc" {
		t.Errorf("string = %q", got)
	}
	// With the flag clear the stream is empty.
	var w2 bitWriter
	w2.bits(0, 20)
	w2.bit(false)
	text, _ = stringStream(newBitReader(w2.bytes()), 20)
	if err := safe(func() { text.tu() }); err != errTruncated {
		t.Errorf("empty string stream: %v", err)
	}
	// A string size that reaches before the start of the data is corrupt.
	var w3 bitWriter
	w3.rs(0x7FFF)
	w3.bit(true)
	if err := safe(func() { stringStream(newBitReader(w3.bytes()), 16) }); err == nil {
		t.Error("an oversized string stream was accepted")
	}
}
