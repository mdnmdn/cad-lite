package dwg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cad-lite/fileio"
	"cad-lite/fileio/dwg"
	"cad-lite/fileio/dxf"
)

// registry is the one the application builds: DXF read and written, DWG read.
func registry() *fileio.Registry {
	r := &fileio.Registry{}
	r.AddReader(dxf.Codec{})
	r.AddReader(dwg.Codec{})
	r.AddWriter(dxf.Codec{})
	return r
}

func TestRegistryOpensDWGByExtension(t *testing.T) {
	r := registry()
	d, warns, err := r.Open(filepath.Join("testdata", "sample_AC1018.dwg"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 56 || len(warns) == 0 {
		t.Errorf("%d entities, %d warnings", d.Len(), len(warns))
	}
	// The extension is matched without regard to case.
	dir := t.TempDir()
	upper := filepath.Join(dir, "PLAN.DWG")
	src, _ := os.ReadFile(filepath.Join("testdata", "sample_AC1015.dwg"))
	if err := os.WriteFile(upper, src, 0o600); err != nil {
		t.Fatal(err)
	}
	if d, _, err := r.Open(upper); err != nil || d.Len() != 56 {
		t.Fatalf("PLAN.DWG: %v", err)
	}
}

func TestRegistryRefusesToWriteDWG(t *testing.T) {
	r := registry()
	d, _, err := r.Open(filepath.Join("testdata", "sample_AC1015.dwg"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := r.Save(filepath.Join(dir, "out.dwg"), d); err == nil || err.Error() != "DWG writing is not supported; save as .dxf" {
		t.Fatalf("Save to .dwg: %v", err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("the failed save left %v", es)
	}
	// Saving the same drawing as DXF works and reads back.
	out := filepath.Join(dir, "out.dxf")
	if _, err := r.Save(out, d); err != nil {
		t.Fatal(err)
	}
	back, _, err := r.Open(out)
	if err != nil || back.Len() != d.Len() {
		t.Fatalf("DXF copy: %v", err)
	}
	if got := r.WritablePath(filepath.Join(dir, "plan.dwg")); got != filepath.Join(dir, "plan.dxf") {
		t.Errorf("WritablePath = %q", got)
	}
}

func TestRegistryReportsAnUnsupportedVersion(t *testing.T) {
	r := registry()
	path := filepath.Join(t.TempDir(), "old.dwg")
	if err := os.WriteFile(path, append([]byte("AC1009"), make([]byte, 400)...), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := r.Open(path)
	if err == nil {
		t.Fatal("an R12 DWG was accepted")
	}
	for _, want := range []string{"AC1009", "R11/R12", "DXF"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
