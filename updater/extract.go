package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtracted bounds the unpacked size of an archive (zip-bomb guard).
const maxExtracted = 1 << 30

// Extract unpacks a .zip or .tar.gz into dest, which must exist. Entries that
// would land outside dest (absolute paths, "..") are an error; file modes are
// kept; symlinks are recreated only when their target stays inside dest.
func Extract(archive, dest string) error {
	switch {
	case strings.HasSuffix(archive, ".zip"):
		return extractZip(archive, dest)
	case strings.HasSuffix(archive, ".tar.gz"), strings.HasSuffix(archive, ".tgz"):
		return extractTarGz(archive, dest)
	}
	return fmt.Errorf("updater: unknown archive type %s", filepath.Base(archive))
}

// inside joins name to dest and rejects results outside it.
func inside(dest, name string) (string, error) {
	p := filepath.Join(dest, filepath.FromSlash(name))
	rel, err := filepath.Rel(dest, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("updater: archive entry %q escapes the target folder", name)
	}
	return p, nil
}

func symlinkInside(dest, link, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("updater: symlink %q points outside the archive", link)
	}
	if _, err := inside(dest, filepath.Join(filepath.Dir(link), target)); err != nil {
		return fmt.Errorf("updater: symlink %q points outside the archive", link)
	}
	return nil
}

func writeFile(path string, mode os.FileMode, r io.Reader, budget *int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm()|0o200)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, *budget+1))
	*budget -= n
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && *budget < 0 {
		err = errors.New("updater: archive unpacks to more than 1 GB")
	}
	if err != nil {
		return err
	}
	return os.Chmod(path, mode.Perm())
}

func extractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer zr.Close()
	budget := int64(maxExtracted)
	for _, f := range zr.File {
		p, err := inside(dest, f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case f.FileInfo().IsDir():
			err = os.MkdirAll(p, 0o755)
		case mode&os.ModeSymlink != 0:
			var rc io.ReadCloser
			if rc, err = f.Open(); err == nil {
				var t []byte
				t, err = io.ReadAll(io.LimitReader(rc, 4096))
				rc.Close()
				if err == nil {
					err = symlinkInside(dest, f.Name, string(t))
				}
				if err == nil {
					err = os.MkdirAll(filepath.Dir(p), 0o755)
				}
				if err == nil {
					err = os.Symlink(string(t), p)
				}
			}
		case mode.IsRegular():
			var rc io.ReadCloser
			if rc, err = f.Open(); err == nil {
				err = writeFile(p, mode, rc, &budget)
				rc.Close()
			}
		}
		if err != nil {
			return fmt.Errorf("updater: unpacking %s: %w", f.Name, err)
		}
	}
	return nil
}

func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	budget := int64(maxExtracted)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("updater: %w", err)
		}
		p, err := inside(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			err = writeFile(p, h.FileInfo().Mode(), tr, &budget)
		case tar.TypeSymlink:
			if err = symlinkInside(dest, h.Name, h.Linkname); err == nil {
				if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
					err = os.Symlink(h.Linkname, p)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("updater: unpacking %s: %w", h.Name, err)
		}
	}
}

// findFile returns the first regular file under root whose base name is name.
func findFile(root, name string) (string, bool) {
	var found string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && found == "" && !d.IsDir() && d.Type().IsRegular() && d.Name() == name {
			found = p
		}
		return nil
	})
	return found, found != ""
}
