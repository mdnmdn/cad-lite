package dwg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// Size caps. A DWG file is read whole, so these bound memory against a corrupt or hostile
// header, not against a big drawing: the largest sample is about 1 MB, and drawings of hundreds
// of megabytes stay under the caps.
const (
	maxFileSize    = 1 << 30
	maxSectionSize = 1 << 28
	maxPageSize    = 1 << 26
)

// Version numbers in file order. The AC10xx magic of a file is the number after "AC".
const (
	verR13 = 1012
	verR14 = 1014
	verR15 = 1015 // R2000
	verR18 = 1018 // R2004
	verR21 = 1021 // R2007
	verR24 = 1024 // R2010
	verR27 = 1027 // R2013
	verR32 = 1032 // R2018
)

// releaseNames maps the file magic to the release it belongs to, for messages.
var releaseNames = map[string]string{
	"MC0.0": "1.0", "AC1.2": "1.2", "AC1.4": "1.4", "AC1.50": "2.0", "AC2.10": "2.10",
	"AC1002": "2.5", "AC1003": "2.6", "AC1004": "R9", "AC1006": "R10", "AC1009": "R11/R12",
	"AC1012": "R13", "AC1014": "R14", "AC1015": "R2000", "AC1018": "R2004", "AC1021": "R2007",
	"AC1024": "R2010", "AC1027": "R2013", "AC1032": "R2018",
}

// file is an opened DWG container: the raw bytes, what the file header says and, once read,
// the section directory.
type file struct {
	data    []byte
	magic   string // "AC1015"
	ver     int
	maint   byte
	codepg  int
	records map[int]locator          // R13 to R2000: section locators by record number
	secs    map[string]*sectionEntry // R2004 and later: sections by name
	r21     map[string]*r21Section   // R2007: the section map of that format
	pages   map[int]pageEntry        // R2004 and later: file pages by page number
	cache   map[string][]byte
	warns   []string
}

// locator is a section locator record of the R13 to R2000 file header.
type locator struct{ seeker, size int }

// pageEntry is one page of the R2004 page map: its offset in the file and its size.
type pageEntry struct{ seeker, size int }

// sectionEntry is a section description of the R2004 section map.
type sectionEntry struct {
	name       string
	size       int // decompressed size of the whole section
	pageMax    int // decompressed size of a full page
	compressed bool
	encrypted  int
	pages      []sectionPage
}

type sectionPage struct{ number, size, offset int }

var errNotDWG = errors.New("dwg: not a DWG file")

// unsupportedError reports a DWG version this reader does not read. Its Error text is built by
// the caller in the user's language.
type unsupportedError struct{ magic string }

func (e *unsupportedError) Error() string { return "dwg: unsupported version " + e.magic }

func (f *file) warnf(format string, a ...any) { f.warns = append(f.warns, fmt.Sprintf(format, a...)) }

// openFile parses the file header and the section directory of data.
func openFile(data []byte) (*file, error) {
	if len(data) < 6 || !(data[0] == 'A' && data[1] == 'C' || data[0] == 'M' && data[1] == 'C') {
		return nil, errNotDWG
	}
	f := &file{data: data, magic: strings.TrimRight(string(data[:6]), "\x00"), cache: map[string][]byte{}}
	if strings.HasPrefix(f.magic, "AC10") {
		fmt.Sscanf(f.magic[2:], "%d", &f.ver)
	}
	var read func() error
	switch f.ver {
	case verR13, verR14, verR15:
		read = f.readHeaderR15
	case verR18, verR24, verR27, verR32:
		read = f.readHeaderR18
	case verR21:
		read = f.readHeaderR21
	}
	if read != nil {
		var herr error
		if perr := safe(func() { herr = read() }); perr != nil {
			herr = perr
		}
		return f, herr
	}
	if _, ok := releaseNames[f.magic]; !ok {
		return nil, errNotDWG
	}
	return nil, &unsupportedError{f.magic}
}

// sectionLimit bounds the decompressed size of a section: a section that claims to be far larger
// than the file that holds it is corrupt (real drawings expand by a small factor), and trusting it
// would let a few kilobytes ask for gigabytes.
func (f *file) sectionLimit() int { return min(maxSectionSize, 64*len(f.data)+1<<20) }

func le32(b []byte) int { return int(int32(binary.LittleEndian.Uint32(b))) }

// readHeaderR15 reads the file header of R13 to R2000: a magic, a code page and the section
// locator records.
func (f *file) readHeaderR15() error {
	if len(f.data) < 0x19 {
		return errTruncated
	}
	r := newBitReader(f.data)
	r.seekByte(0x0B)
	f.maint = r.rc()
	r.seekByte(0x13)
	f.codepg = int(r.rs())
	n := int(r.rl())
	if n < 0 || n > 64 {
		return errCorrupt
	}
	f.records = map[int]locator{}
	for range n {
		num := int(r.rc())
		seeker, size := int(r.rl()), int(r.rl())
		if seeker < 0 || size < 0 || seeker > len(f.data) || size > len(f.data)-seeker {
			// A locator pointing outside the file (a stripped optional section) is dropped.
			continue
		}
		f.records[num] = locator{seeker, size}
	}
	return nil
}

// section returns the decompressed bytes of the named section ("AcDb:Header" and so on), or nil
// when the file has none.
func (f *file) section(name string) ([]byte, error) {
	if b, ok := f.cache[name]; ok {
		return b, nil
	}
	var b []byte
	var err error
	if perr := safe(func() {
		switch {
		case f.ver <= verR15:
			b, err = f.sectionR15(name)
		case f.ver == verR21:
			b, err = f.sectionR21(name)
		default:
			b, err = f.sectionR18(name)
		}
	}); perr != nil {
		b, err = nil, perr
	}
	if err == nil {
		f.cache[name] = b
	}
	return b, err
}

// r15Records maps the section names to the locator record numbers of R13 to R2000.
var r15Records = map[string]int{"AcDb:Header": 0, "AcDb:Classes": 1, "AcDb:Handles": 2, "AcDb:ObjFreeSpace": 3, "AcDb:Template": 4, "AcDb:AuxHeader": 5}

func (f *file) sectionR15(name string) ([]byte, error) {
	rec, ok := r15Records[name]
	if !ok {
		return nil, nil
	}
	l, ok := f.records[rec]
	if !ok {
		return nil, nil
	}
	return f.data[l.seeker : l.seeker+l.size], nil
}

// readHeaderR18 reads the file header of R2004 and later (R2010, R2013 and R2018 share it): a
// masked block that locates the page map, which in turn locates the section map.
func (f *file) readHeaderR18() error {
	if len(f.data) < 0x100 {
		return errTruncated
	}
	f.maint = f.data[0x0B]
	f.codepg = int(int16(binary.LittleEndian.Uint16(f.data[0x13:])))
	hdr := append([]byte(nil), f.data[0x80:0x80+0x6C]...)
	for i, m := range magicSequence(len(hdr)) {
		hdr[i] ^= m
	}
	if string(hdr[:11]) != "AcFssFcAJMB" {
		f.warnf("file ID string is not AcFssFcAJMB")
	}
	stored := binary.LittleEndian.Uint32(hdr[0x68:])
	binary.LittleEndian.PutUint32(hdr[0x68:], 0)
	if crc32Of(0, hdr) != stored {
		f.warnf("file header CRC mismatch")
	}
	pageMapAddr := int(binary.LittleEndian.Uint64(hdr[0x54:])) + 0x100
	pageMapID := int(binary.LittleEndian.Uint32(hdr[0x50:]))
	sectionMapID := int(binary.LittleEndian.Uint32(hdr[0x5C:]))

	// System pages: a 20-byte header (type, decompressed size, compressed size, compression,
	// checksum) followed by LZ77 data.
	sysPage := func(at int) ([]byte, error) {
		if at < 0 || at > len(f.data)-20 {
			return nil, errTruncated
		}
		dsize, csize := le32(f.data[at+4:]), le32(f.data[at+8:])
		if dsize < 0 || dsize > maxPageSize || csize < 0 || csize > len(f.data)-at-20 {
			return nil, errCorrupt
		}
		return decompressAC18(f.data[at+20:at+20+csize], dsize)
	}
	pm, err := sysPage(pageMapAddr)
	if err != nil {
		return err
	}
	f.pages = map[int]pageEntry{}
	total := 0x100
	for p := 0; p+8 <= len(pm); {
		num, size := le32(pm[p:]), le32(pm[p+4:])
		p += 8
		if size < 0 || size > len(f.data) {
			return errCorrupt
		}
		if num >= 0 {
			f.pages[num] = pageEntry{total, size}
		} else {
			p += 16 // parent, left, right, 0: a gap
		}
		total += size
	}
	if pageMapID != 0 {
		if _, ok := f.pages[pageMapID]; !ok {
			f.warnf("page map is not listed in itself")
		}
	}
	sm, ok := f.pages[sectionMapID]
	if !ok {
		return errCorrupt
	}
	smData, err := sysPage(sm.seeker)
	if err != nil {
		return err
	}
	return f.parseSectionMap(smData)
}

func (f *file) parseSectionMap(b []byte) error {
	if len(b) < 20 {
		return errTruncated
	}
	n := le32(b)
	if n < 0 || n > 1024 {
		return errCorrupt
	}
	f.secs = map[string]*sectionEntry{}
	p := 20
	for range n {
		if p+0x60 > len(b) {
			return errTruncated
		}
		s := &sectionEntry{}
		size := binary.LittleEndian.Uint64(b[p:])
		count := le32(b[p+8:])
		s.pageMax = le32(b[p+12:])
		s.compressed = le32(b[p+20:]) == 2
		s.encrypted = le32(b[p+28:])
		name := b[p+32 : p+96]
		if i := indexZero(name); i >= 0 {
			name = name[:i]
		}
		s.name = string(name)
		p += 96
		if size > uint64(f.sectionLimit()) || count < 0 || count > len(b)/16 || s.pageMax < 0 || s.pageMax > maxPageSize {
			return errCorrupt
		}
		s.size = int(size)
		for range count {
			if p+16 > len(b) {
				return errTruncated
			}
			pg := sectionPage{number: le32(b[p:]), size: le32(b[p+4:]), offset: int(binary.LittleEndian.Uint64(b[p+8:]))}
			p += 16
			s.pages = append(s.pages, pg)
		}
		f.secs[s.name] = s
	}
	return nil
}

func indexZero(b []byte) int {
	for i, v := range b {
		if v == 0 {
			return i
		}
	}
	return -1
}

// sectionR18 assembles a section of an R2004-or-later file from its data pages. Each page has
// a 32-byte header masked with 0x4164536B xor its file offset, then the (usually LZ77
// compressed) data; pages are placed by their start offset, gaps staying zero.
func (f *file) sectionR18(name string) ([]byte, error) {
	s := f.secs[name]
	if s == nil {
		return nil, nil
	}
	out := make([]byte, 0, min(s.size, f.sectionLimit()))
	for _, pg := range s.pages {
		pe, ok := f.pages[pg.number]
		if !ok || pe.seeker < 0 || pe.seeker > len(f.data)-32 {
			return nil, errCorrupt
		}
		mask := uint32(0x4164536B) ^ uint32(pe.seeker)
		h := f.data[pe.seeker : pe.seeker+32]
		var w [8]int
		for i := range w {
			w[i] = int(int32(binary.LittleEndian.Uint32(h[4*i:]) ^ mask))
		}
		// w[0] is the page type, w[1] the section number, w[2] the compressed size, w[3] the
		// size of the page in the file and w[4] the start offset; the page expands to at most
		// the section's page size.
		csize := w[2]
		if csize < 0 || csize > len(f.data)-pe.seeker-32 {
			return nil, errCorrupt
		}
		raw := f.data[pe.seeker+32 : pe.seeker+32+csize]
		var page []byte
		if s.compressed {
			var err error
			if page, err = decompressAC18(raw, max(s.pageMax, 1)); err != nil {
				return nil, err
			}
		} else {
			page = raw
		}
		if limit := f.sectionLimit(); pg.offset < 0 || pg.offset > limit || len(page) > limit-pg.offset {
			return nil, errCorrupt
		}
		if need := pg.offset + len(page); need > len(out) {
			out = append(out, make([]byte, need-len(out))...)
		}
		copy(out[pg.offset:], page)
	}
	return out, nil
}
