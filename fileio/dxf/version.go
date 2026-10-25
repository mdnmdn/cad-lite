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

// needs2000 reports whether d has content only AutoCAD 2000 holds: groups, preserved object
// records, or lineweights and linetype scales (an entity's, in model space or a block, or
// $CELWEIGHT and $CELTSCALE). Without a version, the codec writes such a drawing as AC1015.
func needs2000(d *doc.Drawing) bool {
	if len(d.Groups()) > 0 || len(d.Preserved.Objects) > 0 {
		return true
	}
	if d.CurrentLineweight != doc.LineweightByLayer || doc.LinetypeScaleOf(d.CurrentLinetypeScale) != 1 {
		return true
	}
	modern := func(e doc.Entity) bool {
		c := e.Common()
		return c.Lineweight != doc.LineweightByLayer || c.LtScale() != 1
	}
	for _, h := range d.Handles() {
		if modern(d.Entity(h)) {
			return true
		}
	}
	for _, b := range d.Blocks() {
		for _, e := range b.Entities {
			if modern(e) {
				return true
			}
		}
	}
	return false
}

// needsSymbols reports whether d needs the symbol writer (symbolwrite.go): blocks, attribute
// definitions, groups, object records, dimensions (whose drawings are generated blocks) or
// dimension styles with arrow blocks (which it generates).
func needsSymbols(d *doc.Drawing) bool {
	return len(d.Blocks()) > 0 || needs2000(d) || hasAttdef(d) || hasDimensions(d) || len(arrowNames(d)) > 0
}

// AutoVersion implements fileio.VersionWriter: AC1015 when the drawing has content R12 cannot
// hold (needs2000), else AC1009.
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
