package cmdsys

import (
	"strings"

	"go-draw/geom"
)

// This file is the typed object snap overrides of a point prompt (TextOrderSnap): END, MID, CEN,
// QUA, INT, NEA, PER, TAN, NON, a comma list of them, and the two prefixes FROM (base point, then
// an offset) and M2P/MTP (the midpoint of two points). A name arms an override for the next
// answer only: a click, or typed coordinates, which then act as the pick position, so
// "MID" then "70,0" snaps to the midpoint of the object at 70,0 (AutoCAD scripts do the same).
// Overrides ignore the running snaps and work with them off.
//
// While one is pending the session shows a substitute prompt ("END of:", "Base point:", ...);
// the command's own prompt comes back before the answer reaches it, so the command never sees
// the detour. A state whose substitute prompt is no longer current (Escape, the command moved
// on) is stale and is dropped.

type ovKind int

const (
	ovFrom ovKind = iota
	ovM2P
)

// ovFrame is one pending FROM or M2P: stage 0 waits for the base (or first) point, stage 1 for
// the offset (or second point).
type ovFrame struct {
	kind  ovKind
	stage int
	first geom.Vec2
}

type overrideState struct {
	// armed is a pending object snap override: modes to find, or none for NON.
	armed bool
	modes SnapMode
	none  bool
	label string
	chain []ovFrame
	// orig is the command's own prompt and cur the substitute shown now.
	orig, cur *Prompt
}

type ovName struct {
	name string
	mode SnapMode
}

// overrideNames are the typed names, longest spelling first for a three letter prefix match.
var overrideNames = []ovName{
	{"ENDPOINT", SnapEndpoint}, {"MIDPOINT", SnapMidpoint}, {"CENTER", SnapCenter}, {"NODE", SnapNode},
	{"QUADRANT", SnapQuadrant}, {"INTERSECTION", SnapIntersection}, {"INSERT", SnapInsertion},
	{"PERPENDICULAR", SnapPerpendicular}, {"TANGENT", SnapTangent}, {"NEAREST", SnapNearest},
	{"NONE", 0}, {"QUICK", SnapQuick}, {"APPARENT", SnapApparent}, {"EXTENSION", SnapExtension},
	{"PARALLEL", SnapParallel},
}

// overrideKind classifies one typed name.
type ovWord int

const (
	wordNone ovWord = iota
	wordSnap
	wordNon
	wordFrom
	wordM2P
	wordUnsupported
)

// classifyOverride reads one typed name. canon is the three letter name shown in the "of" prompt.
func classifyOverride(t string) (w ovWord, mode SnapMode, canon string) {
	u := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(t), "_"))
	switch u {
	case "M2P", "MTP":
		return wordM2P, 0, u
	case "TT", "GCE":
		return wordUnsupported, 0, u
	}
	if len(u) < 3 {
		return wordNone, 0, ""
	}
	if strings.HasPrefix("FROM", u) {
		return wordFrom, 0, "FROM"
	}
	for _, n := range overrideNames {
		if !strings.HasPrefix(n.name, u) {
			continue
		}
		canon = n.name[:3]
		switch {
		case n.name == "NONE":
			return wordNon, 0, canon
		case n.mode&(SnapQuick|SnapApparent|SnapExtension|SnapParallel|SnapNode|SnapInsertion) != 0:
			return wordUnsupported, n.mode, canon
		}
		return wordSnap, n.mode, canon
	}
	return wordNone, 0, ""
}

func (o *overrideState) active() bool { return o.armed || len(o.chain) > 0 }

func init() {
	RegisterVarGroup(func(s *Session, _ *Vars) { s.AddTextHandler(TextOrderSnap, overrideText) })
}

// live drops a stale state and reports whether an override is pending.
func (s *Session) overrideLive() bool {
	o := &s.aids.ov
	if o.cur != nil && s.prompt != o.cur {
		*o = overrideState{}
	}
	return o.active()
}

// overrideText is the order-30 text handler.
func overrideText(s *Session, text string) bool {
	live := s.overrideLive()
	if !s.pointPrompt() {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" {
		if live {
			s.clearOverride()
		}
		return false
	}
	tr := s.Translator()
	if s.prompt.Accepts&InputKeyword != 0 {
		if _, ok := tr.MatchKeywords(s.prompt.Keywords, t); ok {
			if live {
				s.clearOverride()
			}
			return false
		}
	}
	if words, ok := overrideWords(t); ok {
		s.armOverride(words, text)
		return true
	}
	if !live || t[0] == '<' {
		if live {
			s.clearOverride()
		}
		return false
	}
	var pt geom.Vec2
	if t == "@" {
		pt = s.mem.Point()
	} else {
		p, shaped, err := parsePoint(t, s.overrideBase(), s.cursor)
		if !shaped && err == nil {
			s.clearOverride()
			return false
		}
		if err != nil {
			s.print(s.Prompt() + " " + text)
			s.print(s.inputError(err))
			return true
		}
		pt = p
	}
	s.overrideAnswer(pt, text, false)
	return true
}

type ovParsed struct {
	w     ovWord
	mode  SnapMode
	canon string
}

// overrideWords parses t as one override word or a comma list of snap names.
func overrideWords(t string) ([]ovParsed, bool) {
	var out []ovParsed
	for _, part := range strings.Split(t, ",") {
		w, m, c := classifyOverride(part)
		if w == wordNone {
			return nil, false
		}
		out = append(out, ovParsed{w, m, c})
	}
	if len(out) > 1 {
		for _, p := range out {
			if p.w != wordSnap && p.w != wordNon && p.w != wordUnsupported {
				return nil, false
			}
		}
	}
	return out, true
}

// armOverride starts the overrides of words and shows the substitute prompt.
func (s *Session) armOverride(words []ovParsed, text string) {
	o := &s.aids.ov
	s.print(s.Prompt() + " " + text)
	for _, p := range words {
		if p.w == wordUnsupported {
			s.ctx.Say("snap.unsupported", p.canon)
			return
		}
	}
	if o.orig == nil {
		o.orig = s.prompt
	}
	var labels []string
	for _, p := range words {
		switch p.w {
		case wordFrom:
			o.chain = append(o.chain, ovFrame{kind: ovFrom})
		case wordM2P:
			o.chain = append(o.chain, ovFrame{kind: ovM2P})
		case wordNon:
			o.armed, o.none, o.modes = true, true, 0
			labels = append(labels, p.canon)
		case wordSnap:
			if o.none {
				o.none, o.modes = false, 0
			}
			o.armed = true
			o.modes |= p.mode
			labels = append(labels, p.canon)
		}
	}
	if len(labels) > 0 {
		o.label = strings.Join(labels, ",")
	}
	s.showOverridePrompt()
}

// showOverridePrompt installs the substitute prompt for the pending state.
func (s *Session) showOverridePrompt() {
	o := &s.aids.ov
	orig := o.orig
	p := Prompt{Accepts: InputPoint, Base: orig.Base}
	tr := s.Translator()
	switch {
	case o.armed && !o.none:
		p.Message = tr.T("snap.of", o.label)
	case len(o.chain) > 0:
		f := o.chain[len(o.chain)-1]
		switch {
		case f.kind == ovFrom && f.stage == 0:
			p.Message = tr.T("snap.from_base")
		case f.kind == ovFrom:
			p.Message, p.Base = tr.T("snap.from_offset"), &f.first
		case f.stage == 0:
			p.Message = tr.T("snap.m2p_first")
		default:
			p.Message = tr.T("snap.m2p_second")
		}
	default:
		cp := *orig
		p = cp
	}
	o.cur = &p
	s.prompt = o.cur
}

// clearOverride drops any pending override and puts the command's prompt back.
func (s *Session) clearOverride() {
	o := &s.aids.ov
	if o.cur != nil && s.prompt == o.cur && o.orig != nil {
		s.prompt = o.orig
	}
	*o = overrideState{}
}

// overrideBase is the point relative typed input measures from while an override is pending: the
// FROM base for its offset, else the command's base.
func (s *Session) overrideBase() *geom.Vec2 {
	o := &s.aids.ov
	if !o.armed && len(o.chain) > 0 {
		if f := o.chain[len(o.chain)-1]; f.kind == ovFrom && f.stage == 1 {
			p := f.first
			return &p
		}
	}
	if o.orig != nil && o.orig.Base != nil {
		return o.orig.Base
	}
	return s.mem.base()
}

// overrideAnswer takes the next point pt (a click, or typed coordinates) while an override is
// pending: it resolves the armed snap, advances FROM and M2P, and answers the command when the
// point is complete. echo is what the command line shows for the answer.
func (s *Session) overrideAnswer(pt geom.Vec2, echo string, shift bool) {
	o := &s.aids.ov
	typed := echo != ""
	if echo == "" {
		echo = FormatPoint(pt)
	}
	if o.armed && !o.none {
		hit, ok := FindTyped(s.ctx.Doc, s.ctx.Index, pt, s.aperture(), o.modes, s.snapBase())
		if !ok {
			s.print(s.Prompt() + " " + echo)
			s.ctx.Say("snap.none_found")
			o.armed, o.modes, o.label = false, 0, ""
			if len(o.chain) == 0 {
				s.clearOverride()
			} else {
				s.showOverridePrompt()
			}
			return
		}
		pt = hit.Point
		if !typed {
			echo = FormatPoint(pt)
		}
	}
	o.armed, o.none, o.modes, o.label = false, false, 0, ""
	for len(o.chain) > 0 {
		f := &o.chain[len(o.chain)-1]
		switch {
		case f.stage == 0:
			s.print(s.Prompt() + " " + echo)
			f.first, f.stage = pt, 1
			s.showOverridePrompt()
			return
		case f.kind == ovM2P:
			pt = f.first.Lerp(pt, 0.5)
		}
		o.chain = o.chain[:len(o.chain)-1]
	}
	if o.orig != nil && s.prompt == o.cur {
		s.prompt = o.orig
	}
	*o = overrideState{}
	if !typed {
		echo = FormatPoint(pt)
	}
	s.respond(Input{Kind: InputPoint, Point: pt, Shift: shift}, echo)
}

// snapBase is the previous point of the command that Perpendicular and Tangent measure from: the
// prompt's base, nil at a command's first point (AutoCAD defers those two there; go-draw finds
// nothing).
func (s *Session) snapBase() *geom.Vec2 {
	if o := &s.aids.ov; o.orig != nil {
		return o.orig.Base
	}
	if s.prompt != nil {
		return s.prompt.Base
	}
	return nil
}

// snapListNames are the three letter names of the OSMODE bits, lowest bit first.
var snapListNames = []string{"End", "Mid", "Cen", "Nod", "Qua", "Int", "Ins", "Per", "Tan", "Nea", "Qui", "App", "Ext", "Par"}

// SnapListString writes mode as the -OSNAP list ("End,Cen,Int"), or "NONe" for no mode.
func SnapListString(mode SnapMode) string {
	var out []string
	for i, n := range snapListNames {
		if mode&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return "NONe"
	}
	return strings.Join(out, ",")
}

// ParseSnapList reads a -OSNAP answer: a comma list of snap names (three letters suffice, any
// case) or NONe. It returns the mode set, and the first word it could not read with ok false.
func ParseSnapList(text string) (mode SnapMode, bad string, ok bool) {
	for _, part := range strings.Split(text, ",") {
		w, m, _ := classifyOverride(part)
		switch {
		case w == wordSnap, w == wordUnsupported && m != 0:
			mode |= m
		case w == wordNon:
		default:
			return 0, strings.TrimSpace(part), false
		}
	}
	return mode, "", true
}
