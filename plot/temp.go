package plot

import (
	"os"
	"path/filepath"
	"time"
)

// tempAge is how long a PDF written for the printer or the viewer is kept: the spooler or the
// viewer may read it after Print or OpenViewer returns (always so on Windows).
const tempAge = 24 * time.Hour

// TempDir is the folder TempPDF writes to.
func TempDir() string { return filepath.Join(os.TempDir(), "cad-lite-plot") }

// TempPDF creates an empty PDF file in TempDir for a print job or the viewer, first removing the
// files there older than a day.
func TempPDF() (*os.File, error) {
	dir := TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	cleanTemp(dir, time.Now().Add(-tempAge))
	return os.CreateTemp(dir, "plot-*.pdf")
}

// cleanTemp removes the regular files in dir last changed before cutoff, ignoring failures.
func cleanTemp(dir string, cutoff time.Time) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range es {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
