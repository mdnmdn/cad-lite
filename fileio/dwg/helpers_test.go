package dwg

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"cad-lite/doc"
)

func mustRead(t testing.TB, path string, c Codec) *doc.Drawing {
	t.Helper()
	d, _, err := readFile(path, c)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return d
}

func readFile(path string, c Codec) (*doc.Drawing, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return c.Read(bytes.NewReader(data))
}

func mustBytes(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// geometrySignature is a string that is equal for equal geometry, to the precision the files hold.
func geometrySignature(e doc.Entity) string {
	switch e := e.(type) {
	case *doc.Line:
		return fmt.Sprintf("L %.9g %.9g %.9g %.9g", e.Start.X, e.Start.Y, e.End.X, e.End.Y)
	case *doc.Circle:
		return fmt.Sprintf("C %.9g %.9g %.9g", e.Center.X, e.Center.Y, e.Radius)
	case *doc.Arc:
		return fmt.Sprintf("A %.9g %.9g %.9g %.9g %.9g", e.Center.X, e.Center.Y, e.Radius, e.StartAngle, e.EndAngle)
	case *doc.LWPolyline:
		s := fmt.Sprintf("P %v", e.Closed)
		for i, p := range e.Points {
			s += fmt.Sprintf(" %.9g,%.9g,%.9g", p.X, p.Y, e.Bulges[i])
		}
		return s
	case *doc.Text:
		return fmt.Sprintf("T %q %.9g,%.9g %.9g,%.9g %.9g %.9g %.9g %.9g %d %d %d %s", e.Value, e.Insert.X, e.Insert.Y, e.Align.X, e.Align.Y,
			e.Height, e.Rotation, e.WidthFactor, e.Oblique, e.HAlign, e.VAlign, e.Generation, strings.ToUpper(e.Style))
	}
	return e.Class()
}
