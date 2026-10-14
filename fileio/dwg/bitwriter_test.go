package dwg

import (
	"encoding/binary"
	"math"
)

// bitWriter writes the bit-coded values of a DWG stream, the inverse of bitReader, so tests can
// build records without a DWG file.
type bitWriter struct {
	buf []byte
	n   int // bits written
}

func (w *bitWriter) bit(b bool) {
	if w.n%8 == 0 {
		w.buf = append(w.buf, 0)
	}
	if b {
		w.buf[w.n/8] |= 0x80 >> uint(w.n%8)
	}
	w.n++
}

func (w *bitWriter) bits(v uint, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bit(v>>uint(i)&1 == 1)
	}
}

func (w *bitWriter) rc(b byte) { w.bits(uint(b), 8) }

func (w *bitWriter) rs(v int16) { w.rc(byte(v)); w.rc(byte(v >> 8)) }

func (w *bitWriter) rl(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	for _, x := range b {
		w.rc(x)
	}
}

func (w *bitWriter) rd(v float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	for _, x := range b {
		w.rc(x)
	}
}

func (w *bitWriter) bb(v byte) { w.bits(uint(v), 2) }

func (w *bitWriter) bs(v int16) {
	switch {
	case v == 0:
		w.bb(2)
	case v == 256:
		w.bb(3)
	case v > 0 && v < 256:
		w.bb(1)
		w.rc(byte(v))
	default:
		w.bb(0)
		w.rs(v)
	}
}

func (w *bitWriter) bl(v int32) {
	switch {
	case v == 0:
		w.bb(2)
	case v > 0 && v < 256:
		w.bb(1)
		w.rc(byte(v))
	default:
		w.bb(0)
		w.rl(v)
	}
}

func (w *bitWriter) bll(v uint64) {
	n := 0
	for x := v; x != 0; x >>= 8 {
		n++
	}
	w.bits(uint(n), 3)
	for i := 0; i < n; i++ {
		w.rc(byte(v >> (8 * uint(i))))
	}
}

func (w *bitWriter) bd(v float64) {
	switch v {
	case 0:
		w.bb(2)
	case 1:
		w.bb(1)
	default:
		w.bb(0)
		w.rd(v)
	}
}

// dd writes v against the default def, using the full double.
func (w *bitWriter) dd(v, def float64) {
	if v == def {
		w.bb(0)
		return
	}
	w.bb(3)
	w.rd(v)
}

func (w *bitWriter) mc(v uint64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			w.rc(b | 0x80)
			continue
		}
		w.rc(b)
		return
	}
}

func (w *bitWriter) smc(v int64) {
	neg := v < 0
	if neg {
		v = -v
	}
	for v >= 0x40 {
		w.rc(byte(v&0x7f) | 0x80)
		v >>= 7
	}
	b := byte(v)
	if neg {
		b |= 0x40
	}
	w.rc(b)
}

func (w *bitWriter) ms(v int) {
	for {
		lo := v & 0x7fff
		v >>= 15
		w.rc(byte(lo))
		hi := byte(lo >> 8)
		if v != 0 {
			w.rc(hi | 0x80)
			continue
		}
		w.rc(hi)
		return
	}
}

// handle writes an absolute handle with code (2 to 5) and the fewest bytes.
func (w *bitWriter) handle(code byte, v uint64) {
	n := 0
	for x := v; x != 0; x >>= 8 {
		n++
	}
	w.rc(code<<4 | byte(n))
	for i := n - 1; i >= 0; i-- {
		w.rc(byte(v >> (8 * uint(i))))
	}
}

func (w *bitWriter) tv(s string) {
	w.bs(int16(len(s) + 1))
	for i := 0; i < len(s); i++ {
		w.rc(s[i])
	}
	w.rc(0)
}

func (w *bitWriter) tu(s string) {
	u := []rune(s)
	w.bs(int16(len(u) + 1))
	for _, r := range u {
		w.rc(byte(r))
		w.rc(byte(r >> 8))
	}
	w.rc(0)
	w.rc(0)
}

// pad fills the last byte with zero bits.
func (w *bitWriter) pad() { w.n = (w.n + 7) &^ 7 }

func (w *bitWriter) bytes() []byte { return w.buf }
