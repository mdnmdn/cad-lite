package dxf

import (
	"errors"
	"io"
	"strings"

	"cad-lite/doc"
	"cad-lite/i18n"
)

// The DXF versions the codec writes.
const (
	// VersionR12 is AutoCAD R12/LT2 DXF, the most widely readable version.
	VersionR12 = "AC1009"
	// Version2000 is AutoCAD 2000/LT2000 DXF, which holds groups, object records, MTEXT and
	// lightweight polylines.
	Version2000 = "AC1015"
)

// needs2000 reports whether d has content only AutoCAD 2000 holds: groups or preserved object
// records. Without a version, the codec writes such a drawing as AC1015.
func needs2000(d *doc.Drawing) bool { return len(d.Groups()) > 0 || len(d.Preserved.Objects) > 0 }

// needsSymbols reports whether d needs the symbol writer (symbolwrite.go): blocks, attribute
// definitions, groups, object records or dimensions (whose drawings are generated blocks).
func needsSymbols(d *doc.Drawing) bool {
	return len(d.Blocks()) > 0 || needs2000(d) || hasAttdef(d) || hasDimensions(d)
}

// AutoVersion implements fileio.VersionWriter: AC1015 when the drawing has groups or object
// records, which R12 cannot hold, else AC1009.
func (Codec) AutoVersion(d *doc.Drawing) string {
	if needs2000(d) {
		return Version2000
	}
	return VersionR12
}

// WriteVersion implements fileio.VersionWriter. "" is AutoVersion. AC1015 always takes the symbol
// writer's AutoCAD 2000 path. AC1009 takes the R12 writer, or the symbol writer's R12 path for a
// drawing with blocks; a drawing that needs AutoCAD 2000 loses its groups and object records there,
// and its MTEXT become TEXT, each with a warning (r12Losses).
func (c Codec) WriteVersion(out io.Writer, d *doc.Drawing, version string) ([]string, error) {
	if version == "" {
		version = c.AutoVersion(d)
	}
	switch version {
	case Version2000:
		return writeSymbols(out, d, true)
	case VersionR12:
		if needsSymbols(d) {
			return writeSymbols(out, d, false)
		}
		return writeR12(out, d)
	}
	return nil, errors.New(i18n.T("dxf.err.version", version, strings.Join(c.Versions(), ", ")))
}

// r12Losses are the warnings of the symbol writer's R12 path for what R12 cannot hold.
func r12Losses(d *doc.Drawing, mtexts int) []string {
	var warns []string
	if n := len(d.Groups()); n > 0 {
		warns = append(warns, i18n.T("dxf.warn.groups_r12", n))
	}
	if n := len(d.Preserved.Objects); n > 0 {
		warns = append(warns, i18n.T("dxf.warn.objects_r12", n))
	}
	if mtexts > 0 {
		warns = append(warns, i18n.T("dxf.warn.mtext_as_text", mtexts))
	}
	return warns
}
