package dwg

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnsupportedVersionMessage(t *testing.T) {
	for _, c := range []struct{ magic, release string }{{"AC1009", "R11/R12"}, {"AC1006", "R10"}, {"AC1003", "2.6"}} {
		_, _, err := Codec{}.Read(bytes.NewReader(append([]byte(c.magic), make([]byte, 300)...)))
		if err == nil {
			t.Fatalf("%s: no error", c.magic)
		}
		for _, want := range []string{c.magic, c.release, "DXF"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not mention %q", c.magic, err, want)
			}
		}
	}
}

func TestNotADrawing(t *testing.T) {
	for name, in := range map[string][]byte{
		"empty":     nil,
		"one byte":  {'A'},
		"text":      []byte("0\nSECTION\n2\nHEADER\n"),
		"zip":       []byte("PK\x03\x04" + strings.Repeat("\x00", 100)),
		"too short": []byte("AC1015"),
	} {
		d, warns, err := Codec{}.Read(bytes.NewReader(in))
		if err == nil || d != nil || warns != nil {
			t.Errorf("%s: got %v, %v, %v; want an error", name, d, warns, err)
		}
	}
	_, _, err := Codec{}.Read(bytes.NewReader([]byte("0\nSECTION\n")))
	if err == nil || !strings.Contains(err.Error(), "not a DWG") {
		t.Errorf("a DXF fed to the DWG reader: %v", err)
	}
}

func TestReadErrorPropagates(t *testing.T) {
	if _, _, err := (Codec{}).Read(failingReader{}); err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("a read error was lost: %v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errDisk }

var errDisk = &diskError{}

type diskError struct{}

func (*diskError) Error() string { return "disk on fire" }

func TestExtensions(t *testing.T) {
	if got := (Codec{}).Extensions(); len(got) != 1 || got[0] != ".dwg" {
		t.Errorf("extensions = %v", got)
	}
}

func TestTruncatedFiles(t *testing.T) {
	// Every prefix of a drawing must read to an error or to a drawing, never panic or hang.
	debugPanics = true
	defer func() { debugPanics = false }()
	for _, file := range []string{"empty_AC1032.dwg", "sample_AC1015.dwg", "sample_AC1018.dwg", "sample_AC1021.dwg", "sample_AC1024.dwg"} {
		data := mustBytes(t, "testdata/"+file)
		for _, n := range []int{0, 1, 5, 6, 7, 0x40, 0x7F, 0x80, 0xFF, 0x100, 0x101, 0x480, 0x500, 1000, 4096, len(data) / 3, len(data) / 2, len(data) - 1000, len(data) - 1} {
			if n < 0 || n > len(data) {
				continue
			}
			if _, _, err := importDrawing(data[:n]); err == nil && n < 0x80 {
				t.Errorf("%s truncated to %d bytes was accepted", file, n)
			}
		}
	}
}
