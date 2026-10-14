package dwg

import "encoding/binary"

// reedSolomonDecode undoes the interleaving of the Reed-Solomon encoding R2007 puts on its file
// header and pages: the encoded data is factor interleaved blocks, each blockSize data bytes
// followed by parity bytes. Like ACadSharp, this reader takes the data bytes and does not use the
// parity to correct errors. out receives factor*blockSize bytes at most.
func reedSolomonDecode(encoded, out []byte, factor, blockSize int) {
	index, length := 0, len(out)
	for i := 0; i < factor; i++ {
		if i >= len(encoded) {
			break
		}
		c := i
		size := min(length, blockSize)
		length -= size
		for end := index + size; index < end; index++ {
			if c >= len(encoded) {
				return
			}
			out[index] = encoded[c]
			c += factor
		}
	}
}

// r21Meta is the compressed metadata of the R2007 file header (spec 5.2) that locates the page map
// and the section map.
type r21Meta struct {
	pagesMapOffset, pagesMapSizeCompressed, pagesMapSizeUncompressed, pagesMapCorrection         uint64
	sectionsMapID, sectionsMapSizeCompressed, sectionsMapSizeUncompressed, sectionsMapCorrection uint64
}

// readHeaderR21 reads the file header of R2007: a Reed-Solomon encoded block at 0x80 holding an
// LZ77 compressed table that locates the page map and the section map, both stored as encoded,
// compressed pages.
func (f *file) readHeaderR21() error {
	if len(f.data) < 0x480 {
		return errTruncated
	}
	f.maint = f.data[0x0B]
	f.codepg = int(int16(binary.LittleEndian.Uint16(f.data[0x13:])))
	decoded := make([]byte, 3*239)
	reedSolomonDecode(f.data[0x80:0x480], decoded, 3, 239)
	comprLen := int(int32(binary.LittleEndian.Uint32(decoded[24:])))
	meta := make([]byte, 0x110)
	if comprLen < 0 {
		// Stored uncompressed.
		if -comprLen > len(decoded)-32 {
			return errCorrupt
		}
		copy(meta, decoded[32:32+min(-comprLen, len(meta))])
	} else if err := decompressAC21(decoded, 32, comprLen, meta); err != nil {
		return err
	}
	u := func(off int) uint64 { return binary.LittleEndian.Uint64(meta[off:]) }
	m := r21Meta{
		pagesMapOffset: u(0x38), pagesMapSizeCompressed: u(0x50), pagesMapSizeUncompressed: u(0x58), pagesMapCorrection: u(0x18),
		sectionsMapID: u(0xC0), sectionsMapSizeCompressed: u(0xB0), sectionsMapSizeUncompressed: u(0xC8), sectionsMapCorrection: u(0xD8),
	}
	pm, err := f.pageBufferR21(m.pagesMapOffset, m.pagesMapSizeCompressed, m.pagesMapSizeUncompressed, m.pagesMapCorrection, 0xEF)
	if err != nil {
		return err
	}
	f.pages = map[int]pageEntry{}
	offset := 0
	for p := 0; p+16 <= len(pm); p += 16 {
		size := int64(binary.LittleEndian.Uint64(pm[p:]))
		id := int64(binary.LittleEndian.Uint64(pm[p+8:]))
		if id < 0 {
			id = -id
		}
		if size < 0 || size > int64(len(f.data)) || id > 1<<31 {
			return errCorrupt
		}
		f.pages[int(id)] = pageEntry{offset, int(size)}
		offset += int(size)
	}
	sm, ok := f.pages[int(m.sectionsMapID)]
	if !ok {
		return errCorrupt
	}
	sb, err := f.pageBufferR21(uint64(sm.seeker), m.sectionsMapSizeCompressed, m.sectionsMapSizeUncompressed, m.sectionsMapCorrection, 239)
	if err != nil {
		return err
	}
	return f.parseSectionMapR21(sb)
}

// pageBufferR21 reads the system page at pageOffset (relative to 0x480): the Reed-Solomon decoded
// bytes, then decompressed.
func (f *file) pageBufferR21(pageOffset, compressed, uncompressed, correction uint64, blockSize int) ([]byte, error) {
	if compressed > uint64(len(f.data)) || uncompressed > uint64(f.sectionLimit()) || correction > 1<<16 || pageOffset > uint64(len(f.data)) {
		return nil, errCorrupt
	}
	v1 := (compressed + 7) &^ 7
	total := int(uint32(v1 * correction))
	factor := (total + blockSize - 1) / blockSize
	length := factor * 255
	at := 0x480 + int(pageOffset)
	if total < 0 || length < 0 || at > len(f.data) || length > len(f.data)-at {
		return nil, errCorrupt
	}
	data := make([]byte, total)
	reedSolomonDecode(f.data[at:at+length], data, factor, blockSize)
	out := make([]byte, uncompressed)
	if err := decompressAC21(data, 0, int(compressed), out); err != nil {
		return nil, err
	}
	return out, nil
}

// r21Page is one data page of an R2007 section.
type r21Page struct {
	offset, size, id, usize, csize int
}

type r21Section struct {
	name     string
	encoding uint64
	pages    []r21Page
}

func (f *file) parseSectionMapR21(b []byte) error {
	f.secs = map[string]*sectionEntry{}
	f.r21 = map[string]*r21Section{}
	p := 0
	u := func() uint64 { v := binary.LittleEndian.Uint64(b[p:]); p += 8; return v }
	for p+64 <= len(b) {
		u() // data size
		maxSize := u()
		u() // encryption
		u() // hash
		nameLen := int(u())
		u()
		encoding := u()
		count := int(u())
		if nameLen < 0 || nameLen > len(b) || count < 0 || count > len(b)/56 || maxSize > uint64(f.sectionLimit()) {
			return errCorrupt
		}
		name := ""
		if nameLen > 0 {
			if p+nameLen > len(b) {
				return errTruncated
			}
			name = decodeUTF16(b[p : p+nameLen])
			p += nameLen
		}
		s := &r21Section{name: name, encoding: encoding}
		for range count {
			if p+56 > len(b) {
				return errTruncated
			}
			pg := r21Page{offset: int(u()), size: int(u()), id: int(u()), usize: int(u()), csize: int(u())}
			u() // checksum
			u() // CRC
			s.pages = append(s.pages, pg)
		}
		if name != "" {
			f.r21[name] = s
			f.secs[name] = &sectionEntry{name: name}
		}
	}
	return nil
}

// sectionR21 assembles a section of an R2007 file: each page is Reed-Solomon decoded when the
// section is encoded (encoding 4) and LZ77 decompressed when it is stored compressed, then
// placed at its start offset.
func (f *file) sectionR21(name string) ([]byte, error) {
	s := f.r21[name]
	if s == nil {
		return nil, nil
	}
	limit := f.sectionLimit()
	var out []byte
	for _, pg := range s.pages {
		pe, ok := f.pages[pg.id]
		if !ok {
			return nil, errCorrupt
		}
		at := 0x480 + pe.seeker
		if pe.size < 0 || at < 0 || at > len(f.data) || pe.size > len(f.data)-at || pg.usize < 0 || pg.usize > maxPageSize || pg.csize < 0 || pg.csize > maxPageSize {
			return nil, errCorrupt
		}
		bytes := f.data[at : at+pe.size]
		if s.encoding == 4 {
			aligned := ((pg.csize+7)&^7 + 250) / 251
			arr := make([]byte, aligned*251)
			reedSolomonDecode(bytes, arr, aligned, 251)
			bytes = arr
		}
		if pg.csize != pg.usize {
			page := make([]byte, pg.usize)
			if err := decompressAC21(bytes, 0, pg.csize, page); err != nil {
				return nil, err
			}
			bytes = page
		} else if len(bytes) > pg.usize {
			bytes = bytes[:pg.usize]
		}
		if pg.offset < 0 || pg.offset > limit || len(bytes) > limit-pg.offset {
			return nil, errCorrupt
		}
		if need := pg.offset + len(bytes); need > len(out) {
			out = append(out, make([]byte, need-len(out))...)
		}
		copy(out[pg.offset:], bytes)
	}
	if out == nil {
		out = []byte{}
	}
	return out, nil
}
