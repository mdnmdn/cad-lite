package dwg

import "hash/crc32"

// crc16Table is the table of the DWG 16-bit CRC (the spec's "CRC 8", a reflected CRC-16 with
// polynomial 0xA001, the one ARC and Modbus use).
var crc16Table = func() (t [256]uint16) {
	for i := range t {
		c := uint16(i)
		for range 8 {
			if c&1 != 0 {
				c = c>>1 ^ 0xA001
			} else {
				c >>= 1
			}
		}
		t[i] = c
	}
	return
}()

// crc16 updates seed with b. Objects, the object map and the R13 to R2000 sections are
// protected with seed 0xC0C1 (R2000 file header: 0).
func crc16(seed uint16, b []byte) uint16 {
	for _, v := range b {
		seed = seed>>8 ^ crc16Table[byte(seed)^v]
	}
	return seed
}

// crc32Of is the standard CRC-32 (IEEE) of b continuing from seed, as the R2004 file header
// uses it (seed 0 over the 0x6C bytes with the CRC field zeroed).
func crc32Of(seed uint32, b []byte) uint32 { return crc32.Update(seed, crc32.IEEETable, b) }

// pageChecksum is the Adler-style checksum of an R2004 section page: two running sums modulo
// 0xFFF1, summed in chunks of 0x15B0 bytes, with seed in the low and high halves.
func pageChecksum(seed uint32, b []byte) uint32 {
	sum1, sum2 := seed&0xFFFF, seed>>16
	for len(b) > 0 {
		n := min(len(b), 0x15B0)
		for _, v := range b[:n] {
			sum1 += uint32(v)
			sum2 += sum1
		}
		sum1 %= 0xFFF1
		sum2 %= 0xFFF1
		b = b[n:]
	}
	return sum2<<16 | sum1&0xFFFF
}

// magicSequence returns the first n bytes of the pseudo-random sequence (a linear congruential
// generator seeded with 1) that masks the R2004 file header and the section page headers.
func magicSequence(n int) []byte {
	out := make([]byte, n)
	seed := int32(1)
	for i := range out {
		seed = seed*0x343FD + 0x269EC3
		out[i] = byte(seed >> 16)
	}
	return out
}
