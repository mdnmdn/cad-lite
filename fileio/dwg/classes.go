package dwg

import "runtime"

// class is one entry of the class table: the objects whose type code is the class number
// (500 and up) are of this class.
type class struct {
	num     int
	dxfName string
	cppName string
	entity  bool // the class produces entities (item class id 0x1F2), not objects
}

// classTypeBase is the first type code a class number can take.
const classTypeBase = 500

// readClasses parses the AcDb:Classes section.
func (f *file) readClasses() (map[int]class, error) {
	data, err := f.section("AcDb:Classes")
	if err != nil || data == nil {
		return map[int]class{}, err
	}
	out := map[int]class{}
	err = safe(func() {
		r := newBitReader(data)
		r.skipBytes(16) // start sentinel
		size := int(r.rl())
		if size < 0 || size > len(data) {
			panic(errCorrupt)
		}
		end := r.tell()/8 + size
		if f.ver >= verR24 && f.maint > 3 || f.ver > verR27 {
			r.rl() // the high half of a 64-bit size
		}
		text := r
		inBits := false
		endBit := end * 8
		if f.ver >= verR21 {
			// Strings live in a separate stream that ends where the data starts: its
			// position comes from the flag bit at the end of the main part.
			inBits = true
			flagPos := r.tell() + int(r.rl()) - 1
			saved := r.tell()
			var start int
			text, start = stringStream(r, flagPos)
			endBit = start
			r.seekBit(saved)
			r.bl() // 0
			r.bit()
		} else if f.ver == verR18 {
			r.bs() // maximum class number
			r.rc()
			r.rc()
			r.bit()
		}
		for count := 0; ; count++ {
			if inBits && r.tell() >= endBit || !inBits && (r.tell()+7)/8 >= end || count > 1<<16 {
				break
			}
			var c class
			c.num = int(r.bs())
			r.bs() // proxy flags
			if f.ver >= verR21 {
				text.tu() // application name
				c.cppName = text.tu()
				c.dxfName = text.tu()
			} else {
				r.tv(f.codepg)
				c.cppName = r.tv(f.codepg)
				c.dxfName = r.tv(f.codepg)
			}
			r.bit() // was a zombie
			c.entity = r.bs() == 0x1F2
			if f.ver >= verR18 {
				for range 5 {
					r.bl()
				}
			}
			out[c.num] = c
		}
	})
	if err != nil && len(out) == 0 {
		return out, err
	}
	return out, nil
}

// stringStream returns a reader over the string stream of an R2007+ record whose "string
// stream present" flag is the bit at flagPos, and the bit offset where the stream starts. When
// the flag is clear the reader is empty (its reads fail). The stream ends with a size: a 16-bit
// count of bits just before the flag, or, when its 0x8000 bit is set, a 31-bit count made of two
// such words.
func stringStream(base *bitReader, flagPos int) (*bitReader, int) {
	t := base.clone()
	t.seekBit(flagPos)
	if !t.bit() {
		t.pos = t.end
		return t, t.end
	}
	end := flagPos - 16
	t.seekBit(end)
	n := int(t.rus())
	if n&0x8000 != 0 {
		end -= 16
		t.seekBit(end)
		n &= 0x7FFF
		n += int(t.rus()) << 15
	}
	start := end - n
	if start < 0 {
		panic(errTruncated)
	}
	t.seekBit(start)
	t.end = end
	return t, start
}

// debugPanics makes safe re-panic runtime errors (an index out of range, a nil pointer) instead of
// reporting them as corrupt data, so the fuzz target finds parser bugs that a damaged file would
// otherwise hide.
var debugPanics bool

// safe runs f and turns the panics a bit reader raises, and the runtime errors a damaged file can
// provoke, into an error, so one bad object does not end the import.
func safe(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case runtime.Error:
				if debugPanics {
					panic(r)
				}
				err = errCorrupt
			case error:
				if e != errTruncated && e != errCorrupt && e != errLZ77 {
					panic(r)
				}
				err = e
			default:
				panic(r)
			}
		}
	}()
	f()
	return nil
}
