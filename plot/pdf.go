package plot

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strconv"

	"cad-lite/geom"
	"cad-lite/render"
)

// ptPerMM converts millimetres to PDF points.
const ptPerMM = 72 / 25.4

// WritePDF writes the sheet as a one-page vector PDF whose page is the paper. title becomes the
// document's title.
func (s *Sheet) WritePDF(w io.Writer, title string) error {
	var content bytes.Buffer
	zw := zlib.NewWriter(&content)
	if err := s.writeContent(zw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	pw, ph := s.Width*ptPerMM, s.Height*ptPerMM
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Contents 4 0 R /Resources << >> >>", num(pw), num(ph)),
		"", // the content stream, written below
		fmt.Sprintf("<< /Title %s /Producer (Cad-lite) >>", pdfString(title)),
	}
	bw := bufio.NewWriter(w)
	n := 0
	put := func(s string) { k, _ := bw.WriteString(s); n += k }
	put("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = n
		put(fmt.Sprintf("%d 0 obj\n", i+1))
		if i == 3 {
			put(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n", content.Len()))
			k, _ := bw.Write(content.Bytes())
			n += k
			put("\nendstream")
		} else {
			put(o)
		}
		put("\nendobj\n")
	}
	xref := n
	put(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1))
	for _, off := range offsets {
		put(fmt.Sprintf("%010d 00000 n \n", off))
	}
	put(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref))
	return bw.Flush()
}

// writeContent writes the page's drawing operators.
func (s *Sheet) writeContent(w io.Writer) error {
	bw := bufio.NewWriter(w)
	var buf []byte
	pt := func(p geom.Vec2, op string) {
		buf = strconv.AppendFloat(buf[:0], p.X*ptPerMM, 'f', 2, 64)
		buf = append(buf, ' ')
		buf = strconv.AppendFloat(buf, p.Y*ptPerMM, 'f', 2, 64)
		buf = append(buf, ' ')
		buf = append(buf, op...)
		buf = append(buf, '\n')
		bw.Write(buf)
	}
	c := s.Clip
	fmt.Fprintf(bw, "q\n%s %s %s %s re W n\n1 J 1 j\n", num(c.Min.X*ptPerMM), num(c.Min.Y*ptPerMM),
		num((c.Max.X-c.Min.X)*ptPerMM), num((c.Max.Y-c.Min.Y)*ptPerMM))
	var stroke, fill render.Color
	width := -1.0
	strokeSet, fillSet := false, false
	for _, it := range s.Items {
		if len(it.Points) == 0 {
			continue
		}
		if it.Fill {
			if !fillSet || it.Color != fill {
				fmt.Fprintf(bw, "%s rg\n", rgb(it.Color))
				fill, fillSet = it.Color, true
			}
			pt(it.Points[0], "m")
			for _, p := range it.Points[1:] {
				pt(p, "l")
			}
			bw.WriteString("h f*\n")
			continue
		}
		if !strokeSet || it.Color != stroke {
			fmt.Fprintf(bw, "%s RG\n", rgb(it.Color))
			stroke, strokeSet = it.Color, true
		}
		if it.Width != width {
			fmt.Fprintf(bw, "%s w\n", num(it.Width*ptPerMM))
			width = it.Width
		}
		it.runs(func(ps []geom.Vec2) {
			for i, p := range ps {
				if i == 0 {
					pt(p, "m")
				} else {
					pt(p, "l")
				}
			}
			if len(ps) == 1 {
				pt(ps[0], "l") // a dot: a zero-length segment draws a round cap
			}
			if it.Closed {
				bw.WriteString("h\n")
			}
		})
		bw.WriteString("S\n")
	}
	bw.WriteString("Q\n")
	return bw.Flush()
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

func rgb(c render.Color) string {
	return num(float64(c.R)/255) + " " + num(float64(c.G)/255) + " " + num(float64(c.B)/255)
}

// pdfString is s as a PDF literal string, with non-ASCII runes replaced so it stays valid
// PDFDocEncoding.
func pdfString(s string) string {
	var b bytes.Buffer
	b.WriteByte('(')
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 32 || r > 126:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(')')
	return b.String()
}
