package dxf

import (
	"cad-lite/doc"
	"strings"
)

func readObjects(d *doc.Drawing, objs []object) {
	names := map[doc.Handle]string{}
	for _, o := range objs {
		bumpSeed(d, rawPairs(o))
		if o.typ != "DICTIONARY" {
			continue
		}
		var key string
		for _, p := range o.pairs {
			if p.code == 3 {
				key = p.val
			}
			if p.code == 350 || p.code == 360 {
				h, _ := doc.ParseHandle(strings.TrimSpace(p.val))
				names[h] = key
			}
		}
	}
	for _, o := range objs {
		if o.typ == "GROUP" {
			g := &doc.Group{Description: o.str(300), Unnamed: o.int(70) == 1, Selectable: o.int(71) == 1}
			g.H, _ = doc.ParseHandle(o.str(5))
			g.Name = names[g.H]
			if g.Name == "" {
				g.Name = "*A" + g.H.String()
				g.Unnamed = true
			}
			for _, p := range o.pairs {
				if p.code == 340 {
					h, _ := doc.ParseHandle(strings.TrimSpace(p.val))
					g.Members = append(g.Members, h)
				}
			}
			d.LoadGroup(g)
		} else {
			d.Preserved.Objects = append(d.Preserved.Objects, doc.RawRecord{Pairs: rawPairs(o)})
		}
	}
}
