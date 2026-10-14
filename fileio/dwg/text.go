package dwg

import (
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// codePages maps the code page index of the file header (spec table "DWG code pages") to a
// decoder. Index 0 (undefined) and any unknown index decode as Windows-1252, the code page of
// Western drawings.
var codePages = map[int]encoding.Encoding{
	1: charmap.Windows1252, 2: charmap.ISO8859_1, 3: charmap.ISO8859_2, 4: charmap.ISO8859_3,
	5: charmap.ISO8859_4, 6: charmap.ISO8859_5, 7: charmap.ISO8859_6, 8: charmap.ISO8859_7,
	9: charmap.ISO8859_8, 10: charmap.ISO8859_9, 11: charmap.CodePage437, 12: charmap.CodePage850,
	13: charmap.CodePage852, 14: charmap.CodePage855, 16: charmap.CodePage860,
	18: charmap.CodePage863, 20: charmap.CodePage865,
	22: japanese.ShiftJIS, 23: charmap.Macintosh, 24: traditionalchinese.Big5,
	25: korean.EUCKR, 26: korean.EUCKR, 27: charmap.CodePage866, 28: charmap.Windows1250,
	29: charmap.Windows1251, 30: charmap.Windows1252, 31: simplifiedchinese.GBK, 32: charmap.Windows1253,
	33: charmap.Windows1254, 34: charmap.Windows1255, 35: charmap.Windows1256, 36: charmap.Windows1257,
	37: charmap.Windows874, 38: japanese.ShiftJIS, 39: simplifiedchinese.GBK, 40: korean.EUCKR,
	41: traditionalchinese.Big5, 42: korean.EUCKR, 44: charmap.Windows1258,
}

// decodeText turns the bytes of a TV string into text, stopping at the first NUL. Code page
// index cp selects the decoder; bytes the decoder rejects are dropped.
func decodeText(b []byte, cp int) string {
	if i := indexZero(b); i >= 0 {
		b = b[:i]
	}
	if len(b) == 0 {
		return ""
	}
	enc := codePages[cp]
	if enc == nil {
		enc = charmap.Windows1252
	}
	s, err := enc.NewDecoder().Bytes(b)
	if err != nil {
		return strings.ToValidUTF8(string(b), "")
	}
	return string(s)
}

// decodeUTF16 turns the bytes of an R2007+ TU string (UTF-16LE) into text, stopping at the
// first NUL.
func decodeUTF16(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c := uint16(b[i]) | uint16(b[i+1])<<8
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// tv reads a variable text (TV) from r, in the code page cp (R2004 and earlier): a BS byte
// length and that many bytes.
func (r *bitReader) tv(cp int) string {
	n := int(r.bs())
	if n <= 0 {
		return ""
	}
	return decodeText(r.bytes(n), cp)
}

// tu reads a unicode text (TU, R2007 and later): a BS length in characters and two bytes per
// character.
func (r *bitReader) tu() string {
	n := int(r.bs())
	if n <= 0 {
		return ""
	}
	return decodeUTF16(r.bytes(n * 2))
}
