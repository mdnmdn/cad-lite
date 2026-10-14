package dwg

import "cad-lite/doc"

// readEntities is the second pass: it keeps the model space entities the document models and
// counts the others.
func (imp *importer) readEntities() {
	layers := imp.layerNames()
	styles := func(h uint64) string { return imp.styleNames[h] }
	for _, ref := range imp.ents {
		if subEntity(ref.typ) {
			continue
		}
		o, err := imp.src.openObject(ref.handle, imp.omap[ref.handle])
		if err != nil {
			imp.bad++
			continue
		}
		var e doc.Entity
		var inModel, paper bool
		err = safe(func() {
			h := o.entityHeader()
			inModel = h.mode == 2 || h.mode == 0 && imp.msBlock != 0 && h.owner == imp.msBlock
			if !inModel {
				paper = h.mode == 1
				return
			}
			switch {
			case ref.typ == tLine:
				e = o.readLine(h, layers)
			case ref.typ == tCircle:
				e = o.readCircle(h, layers)
			case ref.typ == tArc:
				e = o.readArc(h, layers)
			case ref.typ == tText:
				e = o.readText(h, layers, styles)
			case ref.typ == tLWPolyline || imp.className(ref.typ) == "LWPOLYLINE":
				e = o.readLWPolyline(h, layers)
			case ref.typ == tPolyline2D:
				p := o.readPolyline2D(h)
				if p.flags&(8|16|64) == 0 {
					e = imp.assemblePolyline(o.common(h, layers), p)
				}
			}
		})
		switch {
		case err != nil:
			imp.bad++
		case !inModel:
			if paper {
				imp.paperSpace++
			}
		case e != nil:
			imp.loaded = append(imp.loaded, e)
		default:
			imp.skipped[imp.typeName(ref.typ)]++
		}
	}
}

// assemblePolyline gathers the vertices of a 2D polyline: the owned list from R2004, the chain
// from the first to the last vertex before. It returns nil when no vertex can be read.
func (imp *importer) assemblePolyline(c doc.EntityCommon, p polyline2D) doc.Entity {
	var vs []vertex2D
	read := func(h uint64) (v vertex2D, ok bool) {
		if imp.budget--; imp.budget < 0 {
			return v, false
		}
		off, found := imp.omap[h]
		if !found {
			return v, false
		}
		o, err := imp.src.openObject(h, off)
		if err != nil || o.typ != tVertex2D || !o.crcOK && !ignoreCRC {
			return v, false
		}
		return v, safe(func() { v = o.readVertex2D(o.entityHeader()) }) == nil
	}
	if len(p.owned) > 0 {
		for _, h := range p.owned {
			if v, ok := read(h); ok {
				vs = append(vs, v)
			}
		}
	} else if p.first != 0 {
		h := p.first
		seen := map[uint64]bool{}
		for range len(imp.omap) {
			if seen[h] {
				break
			}
			seen[h] = true
			v, ok := read(h)
			if !ok {
				break
			}
			vs = append(vs, v)
			if h == p.last {
				break
			}
			h = v.next
		}
	}
	if len(vs) == 0 {
		return nil
	}
	return polylineEntity(c, p, vs)
}
