package dwg

import (
	"encoding/binary"
	"errors"
	"math"
)

// errTruncated is the panic value of a bit reader asked for bits past its end. It is recovered
// where an object or a section is read, so one bad record does not end the whole file.
var errTruncated = errors.New("dwg: read past the end of the data")

// errCorrupt is the panic value of a bit reader that met a value no valid file holds.
var errCorrupt = errors.New("dwg: corrupt data")

// bitReader reads the bit-coded values of a DWG stream (spec chapter 2): most significant bit
// first, from a byte slice, with every read bounds-checked against end. A read past end panics
// with errTruncated; the callers recover.
type bitReader struct {
	data []byte
	pos  int // bit offset of the next bit
	end  int // bit offset one past the last readable bit
	// plain makes the extrusion and thickness readers expect the R13 and R14 coding, a plain 3BD
	// and BD, instead of the single-bit shortcut of R2000 and later.
	plain bool
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data, end: len(data) * 8}
}

// clone returns an independent reader over the same bytes, at the same position.
func (r *bitReader) clone() *bitReader { c := *r; return &c }

// seekBit moves to bit offset p.
func (r *bitReader) seekBit(p int) {
	if p < 0 || p > r.end {
		panic(errTruncated)
	}
	r.pos = p
}

// seekByte moves to byte offset p.
func (r *bitReader) seekByte(p int) {
	if p < 0 || p > len(r.data) {
		panic(errTruncated)
	}
	r.pos = p * 8
}

// tell returns the position in bits.
func (r *bitReader) tell() int { return r.pos }

// bitsLeft returns the number of unread bits.
func (r *bitReader) bitsLeft() int { return r.end - r.pos }

// bit reads one bit (B).
func (r *bitReader) bit() bool {
	if r.pos >= r.end {
		panic(errTruncated)
	}
	b := r.data[r.pos>>3] >> (7 - uint(r.pos&7)) & 1
	r.pos++
	return b == 1
}

// bits reads n (0..8) bits as the low bits of the result.
func (r *bitReader) bits(n int) byte {
	if r.pos+n > r.end {
		panic(errTruncated)
	}
	var v byte
	for i := 0; i < n; i++ {
		v = v<<1 | r.data[r.pos>>3]>>(7-uint(r.pos&7))&1
		r.pos++
	}
	return v
}

// rc reads a raw byte (RC).
func (r *bitReader) rc() byte {
	if r.pos+8 > r.end {
		panic(errTruncated)
	}
	i, s := r.pos>>3, uint(r.pos&7)
	r.pos += 8
	if s == 0 {
		return r.data[i]
	}
	return r.data[i]<<s | r.data[i+1]>>(8-s)
}

// bytes reads n raw bytes. It fails before allocating when fewer than n bytes remain.
func (r *bitReader) bytes(n int) []byte {
	if n < 0 || n > r.bitsLeft()/8 {
		panic(errTruncated)
	}
	out := make([]byte, n)
	if r.pos&7 == 0 {
		copy(out, r.data[r.pos>>3:])
		r.pos += n * 8
		return out
	}
	for i := range out {
		out[i] = r.rc()
	}
	return out
}

// skipBytes skips n raw bytes.
func (r *bitReader) skipBytes(n int) {
	if n < 0 || n > r.bitsLeft()/8 {
		panic(errTruncated)
	}
	r.pos += n * 8
}

// rs reads a raw little-endian 16-bit integer (RS).
func (r *bitReader) rs() int16 { return int16(r.rus()) }

func (r *bitReader) rus() uint16 {
	lo := uint16(r.rc())
	return lo | uint16(r.rc())<<8
}

// rl reads a raw little-endian 32-bit integer (RL).
func (r *bitReader) rl() int32 { return int32(r.rul()) }

func (r *bitReader) rul() uint32 {
	var b [4]byte
	for i := range b {
		b[i] = r.rc()
	}
	return binary.LittleEndian.Uint32(b[:])
}

// rd reads a raw IEEE double (RD).
func (r *bitReader) rd() float64 {
	var b [8]byte
	for i := range b {
		b[i] = r.rc()
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
}

// rsBE reads a big-endian 16-bit integer, as the object map stores section sizes.
func (r *bitReader) rsBE() int {
	hi := int(r.rc())
	return hi<<8 | int(r.rc())
}

// bb reads a two-bit code (BB).
func (r *bitReader) bb() byte { return r.bits(2) }

// bs reads a bit short (BS).
func (r *bitReader) bs() int16 {
	switch r.bb() {
	case 0:
		return r.rs()
	case 1:
		return int16(r.rc())
	case 2:
		return 0
	default:
		return 256
	}
}

// bl reads a bit long (BL).
func (r *bitReader) bl() int32 {
	switch r.bb() {
	case 0:
		return r.rl()
	case 1:
		return int32(r.rc())
	case 2:
		return 0
	default:
		panic(errCorrupt)
	}
}

// bll reads a bit long long (BLL): a 3-bit byte count and that many little-endian bytes.
func (r *bitReader) bll() uint64 {
	n := int(r.bits(3))
	var v uint64
	for i := 0; i < n; i++ {
		v |= uint64(r.rc()) << (8 * uint(i))
	}
	return v
}

// bd reads a bit double (BD).
func (r *bitReader) bd() float64 {
	switch r.bb() {
	case 0:
		return r.rd()
	case 1:
		return 1
	case 2:
		return 0
	default:
		panic(errCorrupt)
	}
}

// dd reads a bit double with default (DD): the default with 0, 4 or 6 bytes patched in, or a
// full double.
func (r *bitReader) dd(def float64) float64 {
	switch r.bb() {
	case 0:
		return def
	case 1:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(def))
		for i := 0; i < 4; i++ {
			b[i] = r.rc()
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
	case 2:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(def))
		b[4], b[5] = r.rc(), r.rc()
		for i := 0; i < 4; i++ {
			b[i] = r.rc()
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
	default:
		return r.rd()
	}
}

// mc reads an unsigned modular char (MC): seven bits per byte, low group first, the high bit
// of a byte saying another follows.
func (r *bitReader) mc() uint64 {
	var v uint64
	for shift := uint(0); ; shift += 7 {
		if shift > 63 {
			panic(errCorrupt)
		}
		b := r.rc()
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v
		}
	}
}

// smc reads a signed modular char: like mc, but bit 6 of the last byte is the sign and holds
// no value bit.
func (r *bitReader) smc() int64 {
	var v int64
	for shift := uint(0); ; shift += 7 {
		if shift > 56 {
			panic(errCorrupt)
		}
		b := r.rc()
		if b&0x80 == 0 {
			v |= int64(b&0x3f) << shift
			if b&0x40 != 0 {
				v = -v
			}
			return v
		}
		v |= int64(b&0x7f) << shift
	}
}

// ms reads a modular short (MS): 15 bits per 16-bit word, the high bit of a word saying
// another follows.
func (r *bitReader) ms() int {
	v := 0
	for shift := uint(0); ; shift += 15 {
		if shift > 30 {
			panic(errCorrupt)
		}
		lo := int(r.rc())
		hi := int(r.rc())
		v |= (lo | (hi&0x7f)<<8) << shift
		if hi&0x80 == 0 {
			return v
		}
	}
}

// handle reads a handle reference (H) relative to base: a nibble code, a nibble byte count and
// that many big-endian bytes. Codes 2 to 5 are absolute; 6 and 8 mean base+1 and base-1; 10 and
// 12 add and subtract the stored value.
func (r *bitReader) handle(base uint64) uint64 {
	form := r.rc()
	code, n := form>>4, int(form&0x0f)
	var v uint64
	if n > 8 {
		panic(errCorrupt)
	}
	switch code {
	case 0x6:
		return base + 1
	case 0x8:
		return base - 1
	}
	for i := 0; i < n; i++ {
		v = v<<8 | uint64(r.rc())
	}
	switch {
	case code <= 0x5:
		return v
	case code == 0xA:
		return base + v
	case code == 0xC:
		return base - v
	}
	panic(errCorrupt)
}

// point2 reads two raw doubles (2RD).
func (r *bitReader) point2() (x, y float64) { x = r.rd(); y = r.rd(); return }

// point3bd reads three bit doubles (3BD).
func (r *bitReader) point3bd() (x, y, z float64) { x = r.bd(); y = r.bd(); z = r.bd(); return }
