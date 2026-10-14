package dwg

import "errors"

var errLZ77 = errors.New("dwg: corrupt compressed section page")

// decompressAC18 expands the LZ77 variant of R2004 and later (spec chapter 4.7). src is the
// compressed page; the result is at most max bytes long, and the call fails when the stream
// would expand past it. A stream normally ends with opcode 0x11; the end of src also ends it.
func decompressAC18(src []byte, max int) (out []byte, err error) {
	d := &lz{src: src, max: max, out: make([]byte, 0, min(max, 8*len(src)+64))}
	defer func() {
		if r := recover(); r != nil {
			if r == errLZ77 || r == errTruncated {
				out, err = nil, errLZ77
				return
			}
			panic(r)
		}
	}()
	d.run()
	return d.out, nil
}

type lz struct {
	src []byte
	pos int
	out []byte
	max int
}

func (d *lz) next() int {
	if d.pos >= len(d.src) {
		panic(errLZ77)
	}
	b := d.src[d.pos]
	d.pos++
	return int(b)
}

// literals copies n bytes of the source to the output and returns the next opcode.
func (d *lz) literals(n int) int {
	if n > len(d.src)-d.pos || len(d.out)+n > d.max {
		panic(errLZ77)
	}
	d.out = append(d.out, d.src[d.pos:d.pos+n]...)
	d.pos += n
	return d.next()
}

// literalCount decodes a literal run length from the low nibble of op: a zero nibble continues
// in following bytes, each zero adding 0xFF and the first non-zero byte ending the sum.
func (d *lz) literalCount(op int) int {
	n := op & 0x0F
	if n == 0 {
		b := d.next()
		for b == 0 {
			n += 0xFF
			b = d.next()
		}
		n += 0x0F + b
	}
	return n
}

func (d *lz) matchLength(op, validBits int) int {
	n := op & validBits
	if n == 0 {
		b := d.next()
		for b == 0 {
			n += 0xFF
			b = d.next()
		}
		n += b + validBits
	}
	return n + 2
}

func (d *lz) twoByteOffset(off, added int) (offset, first int) {
	first = d.next()
	off |= first >> 2
	off |= d.next() << 6
	return off + added, first
}

func (d *lz) run() {
	op := d.next()
	if op > 0x11 {
		op = d.literals(op - 17)
	}
	if op&0xF0 == 0 {
		op = d.literals(d.literalCount(op) + 3)
	}
	for op != 0x11 {
		var n, off int
		switch {
		case op < 0x10 || op >= 0x40:
			n = (op >> 4) - 1
			op2 := d.next()
			off = (op>>2&3 | op2<<2) + 1
		case op < 0x20:
			n = d.matchLength(op, 0b0111)
			off = (op & 8) << 11
			off, op = d.twoByteOffset(off, 0x4000)
		default:
			n = d.matchLength(op, 0b00011111)
			off, op = d.twoByteOffset(0, 1)
		}
		if n < 0 || off <= 0 || off > len(d.out) || len(d.out)+n > d.max {
			panic(errLZ77)
		}
		for range n {
			d.out = append(d.out, d.out[len(d.out)-off])
		}
		lit := op & 3
		if lit == 0 {
			op = d.next()
			if op&0xF0 == 0 {
				lit = d.literalCount(op) + 3
			}
		}
		if lit > 0 {
			op = d.literals(lit)
		}
	}
}
