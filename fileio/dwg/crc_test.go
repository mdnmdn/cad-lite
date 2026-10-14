package dwg

import (
	"bytes"
	"hash/adler32"
	"hash/crc32"
	"math/rand"
	"testing"
)

func TestCRC16(t *testing.T) {
	// The check value of CRC-16/ARC, the reflected 0xA001 CRC the DWG "CRC 8" is.
	if got := crc16(0, []byte("123456789")); got != 0xBB3D {
		t.Errorf("crc16(123456789) = %#04x, want 0xBB3D", got)
	}
	if got := crc16(0xC0C1, nil); got != 0xC0C1 {
		t.Errorf("crc16 of no data changed the seed: %#04x", got)
	}
	// Table spot checks against the published table (entry 1 is 0xC0C1, entry 255 is 0x4040).
	if crc16Table[1] != 0xC0C1 || crc16Table[255] != 0x4040 || crc16Table[128] != 0xA001 {
		t.Errorf("crc16 table entries are wrong: %#x %#x %#x", crc16Table[1], crc16Table[255], crc16Table[128])
	}
	// Continuing is the same as one pass.
	data := []byte("the quick brown fox")
	if crc16(crc16(0xC0C1, data[:7]), data[7:]) != crc16(0xC0C1, data) {
		t.Error("crc16 does not continue from a partial result")
	}
}

func TestCRC32(t *testing.T) {
	if got := crc32Of(0, []byte("123456789")); got != 0xCBF43926 {
		t.Errorf("crc32(123456789) = %#08x, want 0xCBF43926", got)
	}
	data := make([]byte, 1000)
	rand.New(rand.NewSource(3)).Read(data)
	if got, want := crc32Of(0, data), crc32.ChecksumIEEE(data); got != want {
		t.Errorf("crc32 = %#x, want %#x", got, want)
	}
}

func TestPageChecksum(t *testing.T) {
	// With seed 1 the page checksum is Adler-32; the chunked sums must agree for data longer
	// than one 0x15B0-byte chunk.
	rng := rand.New(rand.NewSource(5))
	for _, n := range []int{0, 1, 100, 0x15B0, 0x15B1, 50000} {
		data := make([]byte, n)
		rng.Read(data)
		if got, want := pageChecksum(1, data), adler32.Checksum(data); got != want {
			t.Errorf("pageChecksum(1, %d bytes) = %#x, want adler32 %#x", n, got, want)
		}
	}
}

func TestMagicSequence(t *testing.T) {
	// The mask the R2004 file header is XORed with, as the spec lists its first 92 bytes (the
	// generator continues to 0x6C bytes).
	want := []byte{
		0x29, 0x23, 0xBE, 0x84, 0xE1, 0x6C, 0xD6, 0xAE, 0x52, 0x90, 0x49, 0xF1, 0xF1, 0xBB, 0xE9, 0xEB,
		0xB3, 0xA6, 0xDB, 0x3C, 0x87, 0x0C, 0x3E, 0x99, 0x24, 0x5E, 0x0D, 0x1C, 0x06, 0xB7, 0x47, 0xDE,
		0xB3, 0x12, 0x4D, 0xC8, 0x43, 0xBB, 0x8B, 0xA6, 0x1F, 0x03, 0x5A, 0x7D, 0x09, 0x38, 0x25, 0x1F,
		0x5D, 0xD4, 0xCB, 0xFC, 0x96, 0xF5, 0x45, 0x3B, 0x13, 0x0D, 0x89, 0x0A, 0x1C, 0xDB, 0xAE, 0x32,
		0x20, 0x9A, 0x50, 0xEE, 0x40, 0x78, 0x36, 0xFD, 0x12, 0x49, 0x32, 0xF6, 0x9E, 0x7D, 0x49, 0xDC,
		0xAD, 0x4F, 0x14, 0xF2, 0x44, 0x40, 0x66, 0xD0, 0x6B, 0xC4, 0x30, 0xB7,
	}
	if got := magicSequence(len(want)); !bytes.Equal(got, want) {
		t.Errorf("magic sequence = % x, want % x", got, want)
	}
}
