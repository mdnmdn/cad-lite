package dwg

// headerVars are the header variables the importer uses.
type headerVars struct {
	insUnits       int
	handSeed       uint64
	clayer         uint64
	extMin, extMax [3]float64
	limMin, limMax [2]float64
	hasUnits       bool
}

// readHeader parses the AcDb:Header section up to $INSUNITS (spec chapter 9). The variables are
// stored one after another with no names, so reading INSUNITS means reading every variable before
// it; the ones that matter are kept and the rest are skipped. A section that fails to parse yields
// ok == false and the importer keeps its defaults.
func (f *file) readHeader() (h headerVars, ok bool) {
	data, err := f.section("AcDb:Header")
	if err != nil || data == nil {
		return h, false
	}
	err = safe(func() { h = f.parseHeader(data) })
	return h, err == nil
}

func (f *file) parseHeader(data []byte) (h headerVars) {
	v := f.ver
	r := newBitReader(data)
	r.skipBytes(16) // start sentinel
	r.rl()          // size of the section
	if v >= verR24 && f.maint > 3 || v >= verR32 {
		r.rl() // high half of a 64-bit size
	}
	initial := r.tell()
	text, hnd := r, r
	if v >= verR21 {
		sizeBits := int(r.rl())
		last := initial + sizeBits - 1
		text, _ = stringStream(r, last)
		hnd = r.clone()
		hnd.seekBit(last + 1)
	}
	tv := func() {
		if v >= verR21 {
			text.tu()
		} else {
			text.tv(f.codepg)
		}
	}
	handle := func() uint64 { return hnd.handle(0) }
	cmc := func() { // a colour: index, then from R2004 an RGB value, flags and optional names
		r.bs()
		if v >= verR18 {
			r.bl()
			flag := r.rc()
			if flag&1 != 0 {
				tv()
			}
			if flag&2 != 0 {
				tv()
			}
		}
	}
	bits := func(n int) {
		for range n {
			r.bit()
		}
	}
	shorts := func(n int) {
		for range n {
			r.bs()
		}
	}
	doubles := func(n int) {
		for range n {
			r.bd()
		}
	}
	r14 := v <= verR14
	r2000 := v >= verR15
	if v >= verR27 {
		r.bll() // required versions
	}
	doubles(4)
	for range 4 {
		tv()
	}
	r.bl()
	r.bl()
	if r14 {
		r.bs()
	}
	if v < verR18 {
		handle() // current viewport entity header
	}
	bits(2) // DIMASO, DIMSHO
	if r14 {
		r.bit() // DIMSAV
	}
	bits(7) // PLINEGEN, ORTHOMODE, REGENMODE, FILLMODE, QTEXTMODE, PSLTSCALE, LIMCHECK
	if r14 {
		r.bit() // BLIPMODE
	}
	if v >= verR18 {
		r.bit()
	}
	bits(4) // USRTIMER, SKPOLY, ANGDIR, SPLFRAME
	if r14 {
		bits(2) // ATTREQ, ATTDIA
	}
	bits(2) // MIRRTEXT, WORLDVIEW
	if r14 {
		r.bit() // WIREFRAME
	}
	bits(3) // TILEMODE, PLIMCHECK, VISRETAIN
	if r14 {
		r.bit() // DELOBJ
	}
	bits(2) // DISPSILH, PELLIPSE
	r.bs()  // PROXYGRAPHICS
	if r14 {
		r.bs() // DRAGMODE
	}
	shorts(5) // TREEDEPTH, LUNITS, LUPREC, AUNITS, AUPREC
	if r14 {
		r.bs() // OSMODE
	}
	r.bs() // ATTMODE
	if r14 {
		r.bs() // COORDS
	}
	r.bs() // PDMODE
	if r14 {
		r.bs() // PICKSTYLE
	}
	if v >= verR18 {
		r.bl()
		r.bl()
		r.bl()
	}
	shorts(5)  // USERI1-5
	shorts(12) // SPLINESEGS .. ISOLINES
	shorts(2)  // CMLJUST, TEXTQLTY
	doubles(2) // LTSCALE, TEXTSIZE
	doubles(2) // TRACEWID, SKETCHINC
	doubles(4) // FILLETRAD, THICKNESS, ANGBASE, PDSIZE
	doubles(1) // PLINEWID
	doubles(5) // USERR1-5
	doubles(4) // CHAMFERA-D
	doubles(3) // FACETRES, CMLSCALE, CELTSCALE
	tv()       // MENUNAME
	r.bl()     // TDCREATE
	r.bl()
	r.bl() // TDUPDATE
	r.bl()
	if v >= verR18 {
		r.bl()
		r.bl()
		r.bl()
	}
	r.bl() // TDINDWG
	r.bl()
	r.bl() // TDUSRTIMER
	r.bl()
	cmc() // CECOLOR
	h.handSeed = r.handle(0)
	h.clayer = handle()
	handle() // TEXTSTYLE
	handle() // CELTYPE
	if v >= verR21 {
		handle() // CMATERIAL
	}
	handle() // DIMSTYLE
	handle() // CMLSTYLE
	if r2000 {
		r.bd() // PSVPSCALE
	}
	doubles(3) // INSBASE (PSPACE)
	doubles(3) // EXTMIN (PSPACE)
	doubles(3) // EXTMAX (PSPACE)
	r.rd()
	r.rd() // LIMMIN (PSPACE)
	r.rd()
	r.rd()     // LIMMAX (PSPACE)
	r.bd()     // ELEVATION (PSPACE)
	doubles(9) // UCSORG, UCSXDIR, UCSYDIR (PSPACE)
	handle()   // UCSNAME (PSPACE)
	if r2000 {
		handle() // PUCSORTHOREF
		r.bs()   // PUCSORTHOVIEW
		handle() // PUCSBASE
		doubles(18)
	}
	doubles(3) // INSBASE (MSPACE)
	h.extMin[0], h.extMin[1], h.extMin[2] = r.point3bd()
	h.extMax[0], h.extMax[1], h.extMax[2] = r.point3bd()
	h.limMin[0], h.limMin[1] = r.point2()
	h.limMax[0], h.limMax[1] = r.point2()
	r.bd()     // ELEVATION
	doubles(9) // UCSORG, UCSXDIR, UCSYDIR
	handle()   // UCSNAME
	if r2000 {
		handle() // UCSORTHOREF
		r.bs()   // UCSORTHOVIEW
		handle() // UCSBASE
		doubles(18)
		tv() // DIMPOST
		tv() // DIMAPOST
	}
	if r14 {
		bits(11) // DIMTOL .. DIMSOXD
		r.rc()   // DIMALTD
		r.rc()   // DIMZIN
		bits(2)  // DIMSD1, DIMSD2
		r.rc()   // DIMTOLJ
		r.rc()   // DIMJUST
		r.rc()   // DIMFIT
		r.bit()  // DIMUPT
		for range 4 {
			r.rc() // DIMTZIN, DIMALTZ, DIMALTTZ, DIMTAD
		}
		shorts(6)
		handle() // DIMTXSTY
	}
	doubles(9) // DIMSCALE .. DIMTM
	if v >= verR21 {
		r.bd() // DIMFXL
		r.bd() // DIMJOGANG
		r.bs() // DIMTFILL
		cmc()  // DIMTFILLCLR
	}
	if r2000 {
		bits(6) // DIMTOL .. DIMSE2
		shorts(3)
	}
	if v >= verR21 {
		r.bs() // DIMARCSYM
	}
	doubles(8) // DIMTXT .. DIMGAP
	if r14 {
		for range 5 {
			tv()
		}
	}
	if r2000 {
		r.bd()  // DIMALTRND
		bits(1) // DIMALT
		r.bs()  // DIMALTD
		bits(4) // DIMTOFL, DIMSAH, DIMTIX, DIMSOXD
	}
	cmc() // DIMCLRD
	cmc() // DIMCLRE
	cmc() // DIMCLRT
	if r2000 {
		shorts(11) // DIMADEC .. DIMJUST
		bits(2)    // DIMSD1, DIMSD2
		shorts(4)  // DIMTOLJ, DIMTZIN, DIMALTZ, DIMALTTZ
		r.bit()    // DIMUPT
		r.bs()     // DIMATFIT
	}
	if v >= verR21 {
		r.bit() // DIMFXLON
	}
	if v >= verR24 {
		r.bit() // DIMTXTDIRECTION
		r.bd()  // DIMALTMZF
		tv()    // DIMALTMZS
		r.bd()  // DIMMZF
		tv()    // DIMMZS
	}
	if r2000 {
		for range 5 {
			handle() // DIMTXSTY, DIMLDRBLK, DIMBLK, DIMBLK1, DIMBLK2
		}
	}
	if v >= verR21 {
		for range 3 {
			handle() // DIMLTYPE, DIMLTEX1, DIMLTEX2
		}
	}
	if r2000 {
		shorts(2) // DIMLWD, DIMLWE
	}
	for range 9 {
		handle() // the nine table control objects
	}
	if v <= verR15 {
		handle() // viewport entity header control object
	}
	for range 3 {
		handle() // ACAD_GROUP, ACAD_MLINESTYLE, named objects dictionaries
	}
	if r2000 {
		r.bs()
		r.bs()
		tv() // HYPERLINKBASE
		tv() // STYLESHEET
		for range 3 {
			handle()
		}
	}
	if v >= verR18 {
		handle()
		handle()
	}
	if v >= verR21 {
		handle()
		if v >= verR27 {
			handle()
		}
	}
	if r2000 {
		r.bl() // flags
		h.insUnits = int(r.bs())
		h.hasUnits = true
	}
	return h
}
