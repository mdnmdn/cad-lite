package dwg

// readObjectMap parses the AcDb:Handles section: the map from object handle to the offset of
// the object (absolute in the file up to R2000, within the AcDbObjects section after). The
// section is a series of chunks, each a big-endian size, pairs of (handle step, offset step) as
// modular chars, and a CRC, ended by a chunk of size 2.
func (f *file) readObjectMap() (map[uint64]int, error) {
	data, err := f.section("AcDb:Handles")
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, errCorrupt
	}
	out := map[uint64]int{}
	err = safe(func() {
		r := newBitReader(data)
		for r.bitsLeft() >= 16 {
			size := r.rsBE()
			if size <= 2 {
				return
			}
			chunkEnd := r.tell()/8 + min(size, 2032) - 2
			if chunkEnd > len(data) {
				chunkEnd = len(data)
			}
			var handle uint64
			var loc int64
			for r.tell()/8 < chunkEnd {
				step := r.mc()
				handle += step
				loc += r.smc()
				if step > 0 && loc >= 0 && loc < maxSectionSize {
					out[handle] = int(loc)
				}
			}
			r.rc() // CRC
			r.rc()
		}
	})
	if len(out) == 0 && err != nil {
		return nil, err
	}
	return out, nil
}
