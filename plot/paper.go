package plot

import "strings"

// Paper is a sheet size, portrait: Width is the short side.
type Paper struct {
	// Name is the size's usual name ("A4", "Letter").
	Name string
	// Width and Height are the portrait size in millimetres.
	Width, Height float64
}

// Papers are the sizes the plot dialog offers, ISO first and then the North American ones.
var Papers = []Paper{
	{"A4", 210, 297},
	{"A3", 297, 420},
	{"A2", 420, 594},
	{"A1", 594, 841},
	{"A0", 841, 1189},
	{"Letter", 215.9, 279.4},
	{"Legal", 215.9, 355.6},
	{"Tabloid", 279.4, 431.8},
}

// PaperByName returns the paper of Papers called name, in any case, and whether there is one.
func PaperByName(name string) (Paper, bool) {
	for _, p := range Papers {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Paper{}, false
}

// Size returns the paper's width and height as oriented.
func (p Paper) Size(landscape bool) (w, h float64) {
	w, h = min(p.Width, p.Height), max(p.Width, p.Height)
	if landscape {
		w, h = h, w
	}
	return w, h
}
