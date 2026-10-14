package dwg

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
)

func TestDecompressAC18Literals(t *testing.T) {
	// 0x13 = 19: opcode > 0x11 copies opcode-17 = 2 literal bytes, then 0x11 ends.
	got, err := decompressAC18([]byte{0x13, 'a', 'b', 0x11}, 100)
	if err != nil || string(got) != "ab" {
		t.Fatalf("got %q, %v", got, err)
	}
	// Opcode below 0x10: a literal run of (opcode&0xF)+3 bytes: 0x02 -> 5 bytes.
	got, err = decompressAC18(append(append([]byte{0x02}, "hello"...), 0x11), 100)
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	// A zero low nibble continues in following bytes: 0x00, then a byte b adds 0x0F+b+3.
	long := bytes.Repeat([]byte{'x'}, 0x0F+2+3)
	src := append([]byte{0x00, 0x02}, long...)
	src = append(src, 0x11)
	got, err = decompressAC18(src, 100)
	if err != nil || !bytes.Equal(got, long) {
		t.Fatalf("long run: %d bytes, %v", len(got), err)
	}
}

func TestDecompressAC18BackReference(t *testing.T) {
	// Opcodes 0x40 and up copy (op>>4)-1 bytes from (op>>2&3 | next<<2)+1 bytes back.
	// "abcd", then op 0x5C (4 bytes, offset bits 3, next byte 0: offset 4), then the end marker.
	src := []byte{0x15, 'a', 'b', 'c', 'd', 0x5C, 0x00, 0x11}
	got, err := decompressAC18(src, 100)
	if err != nil || string(got) != "abcdabcd" {
		t.Fatalf("got %q, %v", got, err)
	}
	// An offset shorter than the length repeats the pattern: "z", then 6 bytes from 1 back
	// (op 0x70, next byte 0).
	src = []byte{0x12, 'z', 0x70, 0x00, 0x11}
	got, err = decompressAC18(src, 100)
	if err != nil || string(got) != "zzzzzzz" {
		t.Fatalf("overlap: got %q, %v", got, err)
	}
}

func TestDecompressAC18MatchOpcodes(t *testing.T) {
	// Opcodes 0x20 to 0x3F copy (op&0x1F)+2 bytes from an offset in the next two bytes: after the
	// literals "ABCDE", op 0x23 (5 bytes) with bytes 0x10 0x00 (offset (0x10>>2 | 0<<6)+1 = 5).
	src := []byte{0x16, 'A', 'B', 'C', 'D', 'E', 0x23, 0x10, 0x00, 0x11}
	got, err := decompressAC18(src, 100)
	if err != nil || string(got) != "ABCDEABCDE" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDecompressAC18Errors(t *testing.T) {
	cases := map[string][]byte{
		"empty":                {},
		"no terminator":        {0x13, 'a', 'b'},
		"literal past the end": {0x20, 'a'},
		"offset before start":  {0x12, 'a', 0x70, 0x20, 0x11},
	}
	for name, src := range cases {
		if _, err := decompressAC18(src, 100); err == nil {
			t.Errorf("%s: no error", name)
		} else if !errors.Is(err, errLZ77) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The output cap holds: a stream that would expand past it fails.
	if _, err := decompressAC18([]byte{0x13, 'a', 'b', 0x11}, 1); err == nil {
		t.Error("expansion past the cap was accepted")
	}
	// An enormous run length claims far more than the data has.
	if _, err := decompressAC18([]byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF}, 1<<20); err == nil {
		t.Error("an unbacked literal run was accepted")
	}
}

func TestDecompressAC18Random(t *testing.T) {
	// Arbitrary bytes must end in an error or a result within the cap, never a panic.
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 2000; i++ {
		src := make([]byte, rng.Intn(64))
		rng.Read(src)
		out, err := decompressAC18(src, 4096)
		if err == nil && len(out) > 4096 {
			t.Fatalf("output of %d bytes exceeds the cap", len(out))
		}
	}
}

func TestRunTailCoversEveryByte(t *testing.T) {
	// For every run length from 1 to 31 the pieces must write each destination byte exactly once.
	for n := 1; n <= 31; n++ {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(i + 1)
		}
		dst := make([]byte, n)
		d := &lz21{src: src, dst: dst}
		d.copyRun(n)
		seen := map[byte]bool{}
		for _, b := range dst {
			if b == 0 || seen[b] {
				t.Errorf("run of %d: destination %v is not a permutation of the source", n, dst)
				break
			}
			seen[b] = true
		}
	}
}

func TestCopyRunOrder(t *testing.T) {
	seq := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i)
		}
		return b
	}
	run := func(n int) []byte {
		dst := make([]byte, n)
		(&lz21{src: seq(n), dst: dst}).copyRun(n)
		return dst
	}
	// Two bytes and three bytes are reversed; four are plain.
	if got := run(2); !bytes.Equal(got, []byte{1, 0}) {
		t.Errorf("run(2) = %v", got)
	}
	if got := run(3); !bytes.Equal(got, []byte{2, 1, 0}) {
		t.Errorf("run(3) = %v", got)
	}
	if got := run(4); !bytes.Equal(got, []byte{0, 1, 2, 3}) {
		t.Errorf("run(4) = %v", got)
	}
	// Five: the last byte first, then the first four.
	if got := run(5); !bytes.Equal(got, []byte{4, 0, 1, 2, 3}) {
		t.Errorf("run(5) = %v", got)
	}
	// A full 32-byte block is its four 8-byte words in reverse order.
	got := run(32)
	for w := 0; w < 4; w++ {
		for i := 0; i < 8; i++ {
			if got[8*w+i] != byte(8*(3-w)+i) {
				t.Fatalf("run(32) = %v", got)
			}
		}
	}
}

func TestDecompressAC21(t *testing.T) {
	// Opcode 0x00..: the first byte is the opcode; a literal run follows with length opcode+8.
	// Stream: op 0x00 (literal length 8), 8 bytes, then end.
	src := append([]byte{0x00}, 1, 2, 3, 4, 5, 6, 7, 8)
	dst := make([]byte, 8)
	if err := decompressAC21(src, 0, len(src), dst); err != nil {
		t.Fatal(err)
	}
	// Eight bytes are one 8-byte word, copied plain.
	if !bytes.Equal(dst, []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Errorf("dst = %v", dst)
	}
	// Bad ranges are errors, not panics.
	for _, c := range []struct{ off, n int }{{-1, 4}, {0, 99}, {0, 0}} {
		if err := decompressAC21(src, c.off, c.n, make([]byte, 8)); err == nil {
			t.Errorf("range %v accepted", c)
		}
	}
	if err := decompressAC21(src, 0, len(src), make([]byte, 2)); err == nil {
		t.Error("an output buffer that is too small was accepted")
	}
}

func TestDecompressAC21Random(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for i := 0; i < 3000; i++ {
		src := make([]byte, 1+rng.Intn(80))
		rng.Read(src)
		dst := make([]byte, rng.Intn(300))
		_ = decompressAC21(src, 0, len(src), dst)
	}
}
