package dwg

// decompressAC21 expands the LZ77 variant of R2007 (spec chapter 5.9): src[off:off+length] is
// the compressed data and dst the buffer of the exact decompressed size. Unlike the later
// variant, literal runs are copied in an odd order: in blocks of 32 bytes, each block as four
// 8-byte words in reverse order, and a shorter run as its largest words last (see copyRun).
func decompressAC21(src []byte, off, length int, dst []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if r == errLZ77 || r == errTruncated {
				err = errLZ77
				return
			}
			panic(r)
		}
	}()
	d := &lz21{src: src, dst: dst}
	d.run(off, length)
	return nil
}

type lz21 struct {
	src, dst  []byte
	si, di    int // source and destination positions
	opcode    int
	length    int
	srcOffset int
}

func (d *lz21) next() int {
	if d.si >= len(d.src) {
		panic(errLZ77)
	}
	b := d.src[d.si]
	d.si++
	return int(b)
}

func (d *lz21) run(off, length int) {
	if off < 0 || length < 0 || off > len(d.src)-length || length == 0 {
		panic(errLZ77)
	}
	d.si = off
	end := off + length
	d.opcode = d.next()
	if d.si >= end {
		return
	}
	if d.opcode&0xF0 == 0x20 {
		d.si += 3
		if d.si > len(d.src) {
			panic(errLZ77)
		}
		d.length = int(d.src[d.si-1]) & 7
	}
	for d.si < end {
		d.literals()
		if d.si >= end {
			break
		}
		d.chunks(end)
	}
}

// literals copies a literal run to the output.
func (d *lz21) literals() {
	if d.length == 0 {
		d.literalLength()
	}
	d.copyRun(d.length)
	d.si += d.length
	d.di += d.length
}

func (d *lz21) literalLength() {
	d.length = d.opcode + 8
	if d.length == 0x17 {
		n := d.next()
		d.length += n
		if n == 0xFF {
			for {
				n = d.next()
				n |= d.next() << 8
				d.length += n
				if n != 0xFFFF {
					break
				}
			}
		}
	}
}

// chunks decodes back-references until a literal run follows.
func (d *lz21) chunks(end int) {
	d.length = 0
	d.opcode = d.next()
	d.instructions()
	for {
		d.backRef()
		d.di += d.length
		d.length = d.opcode & 7
		if d.length != 0 || d.si >= end {
			break
		}
		d.opcode = d.next()
		if d.opcode>>4 == 0 {
			break
		}
		if d.opcode>>4 == 15 {
			d.opcode &= 15
		}
		d.instructions()
	}
}

// backRef copies length bytes from srcOffset bytes back in the output (overlapping allowed).
func (d *lz21) backRef() {
	from := d.di - d.srcOffset
	if d.length < 0 || from < 0 || d.di+d.length > len(d.dst) {
		panic(errLZ77)
	}
	for i := range d.length {
		d.dst[d.di+i] = d.dst[from+i]
	}
}

func (d *lz21) instructions() {
	switch d.opcode >> 4 {
	case 0:
		d.length = d.opcode&0xF + 0x13
		d.srcOffset = d.next()
		d.opcode = d.next()
		d.length = d.opcode>>3&0x10 + d.length
		d.srcOffset = (d.opcode&0x78)<<5 + 1 + d.srcOffset
	case 1:
		d.length = d.opcode&0xF + 3
		d.srcOffset = d.next()
		d.opcode = d.next()
		d.srcOffset = (d.opcode&248)<<5 + 1 + d.srcOffset
	case 2:
		d.srcOffset = d.next()
		d.srcOffset |= d.next() << 8 & 0xFF00
		d.length = d.opcode & 7
		if d.opcode&8 == 0 {
			d.opcode = d.next()
			d.length = d.opcode&0xF8 + d.length
		} else {
			d.srcOffset++
			d.length = d.next()<<3 + d.length
			d.opcode = d.next()
			d.length = (d.opcode&0xF8)<<8 + d.length + 0x100
		}
	default:
		d.length = d.opcode >> 4
		d.srcOffset = d.opcode & 15
		d.opcode = d.next()
		d.srcOffset = (d.opcode&0xF8)<<1 + d.srcOffset + 1
	}
}

// copyRun copies n literal bytes from src[si:] to dst[di:] in the odd order of the format.
func (d *lz21) copyRun(n int) {
	if n < 0 || n > len(d.src)-d.si || n > len(d.dst)-d.di {
		panic(errLZ77)
	}
	s, t := d.src[d.si:], d.dst[d.di:]
	for ; n >= 32; n -= 32 {
		copy(t[0:8], s[24:32])
		copy(t[8:16], s[16:24])
		copy(t[16:24], s[8:16])
		copy(t[24:32], s[0:8])
		s, t = s[32:], t[32:]
	}
	if n == 0 {
		return
	}
	for _, p := range runTail[n] {
		p.apply(s, t)
	}
}

// piece is one step of copying a short run: size bytes from src offset so to dst offset do, plain
// for sizes 4 and 8, byte-reversed for sizes 1 to 3 (the reversal of 1 byte is the byte) and
// word-swapped for 16.
type piece struct{ size, so, do int }

func (p piece) apply(s, t []byte) {
	switch p.size {
	case 1:
		t[p.do] = s[p.so]
	case 2:
		t[p.do], t[p.do+1] = s[p.so+1], s[p.so]
	case 3:
		t[p.do], t[p.do+1], t[p.do+2] = s[p.so+2], s[p.so+1], s[p.so]
	case 16: // two 8-byte words, swapped
		copy(t[p.do:p.do+8], s[p.so+8:p.so+16])
		copy(t[p.do+8:p.do+16], s[p.so:p.so+8])
	default:
		copy(t[p.do:p.do+p.size], s[p.so:p.so+p.size])
	}
}

// runTail lists, for each run length 1 to 31, the pieces that copy it.
var runTail = [32][]piece{
	1:  {{1, 0, 0}},
	2:  {{2, 0, 0}},
	3:  {{3, 0, 0}},
	4:  {{4, 0, 0}},
	5:  {{1, 4, 0}, {4, 0, 1}},
	6:  {{1, 5, 0}, {4, 1, 1}, {1, 0, 5}},
	7:  {{2, 5, 0}, {4, 1, 2}, {1, 0, 6}},
	8:  {{8, 0, 0}},
	9:  {{1, 8, 0}, {8, 0, 1}},
	10: {{1, 9, 0}, {8, 1, 1}, {1, 0, 9}},
	11: {{2, 9, 0}, {8, 1, 2}, {1, 0, 10}},
	12: {{4, 8, 0}, {8, 0, 4}},
	13: {{1, 12, 0}, {4, 8, 1}, {8, 0, 5}},
	14: {{1, 13, 0}, {4, 9, 1}, {8, 1, 5}, {1, 0, 13}},
	15: {{2, 13, 0}, {4, 9, 2}, {8, 1, 6}, {1, 0, 14}},
	16: {{16, 0, 0}},
	17: {{8, 9, 0}, {1, 8, 8}, {8, 0, 9}},
	18: {{1, 17, 0}, {16, 1, 1}, {1, 0, 17}},
	19: {{3, 16, 0}, {16, 0, 3}},
	20: {{4, 16, 0}, {8, 8, 4}, {8, 0, 12}},
	21: {{1, 20, 0}, {4, 16, 1}, {8, 8, 5}, {8, 0, 13}},
	22: {{2, 20, 0}, {4, 16, 2}, {8, 8, 6}, {8, 0, 14}},
	23: {{3, 20, 0}, {4, 16, 3}, {8, 8, 7}, {8, 0, 15}},
	24: {{8, 16, 0}, {16, 0, 8}},
	25: {{8, 17, 0}, {1, 16, 8}, {16, 0, 9}},
	26: {{1, 25, 0}, {8, 17, 1}, {1, 16, 9}, {16, 0, 10}},
	27: {{2, 25, 0}, {8, 17, 2}, {1, 16, 10}, {16, 0, 11}},
	28: {{4, 24, 0}, {8, 16, 4}, {8, 8, 12}, {8, 0, 20}},
	29: {{1, 28, 0}, {4, 24, 1}, {8, 16, 5}, {8, 8, 13}, {8, 0, 21}},
	30: {{2, 28, 0}, {4, 24, 2}, {8, 16, 6}, {8, 8, 14}, {8, 0, 22}},
	31: {{1, 30, 0}, {4, 26, 1}, {8, 18, 5}, {8, 10, 13}, {8, 2, 21}, {2, 0, 29}},
}
