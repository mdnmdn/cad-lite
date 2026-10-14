package cmdsys

import (
	"cad-lite/doc"
	"cad-lite/geom"
)

// lastState is the "last" memory AutoCAD keeps between commands: the last point entered
// (LASTPOINT, what @ and relative input measure from), the last entity a command created
// (selected by the L selection option) and the end of the last line or arc (LINE Enter-continue
// and ARC Enter-continue). One per session.
type lastState struct {
	point   geom.Vec2
	hasPt   bool
	entity  doc.Handle
	hasEnt  bool
	end     geom.Vec2
	tangent float64
	hasEnd  bool
	// setvar is the variable SETVAR named last (its <default>); script the file SCRIPT ran last.
	setvar string
	script string
}

// Point returns the last point, the origin before any was entered.
func (l *lastState) Point() geom.Vec2 { return l.point }

// SetPoint records p as the last point.
func (l *lastState) SetPoint(p geom.Vec2) { l.point, l.hasPt = p, true }

// base returns the last point for relative input, or nil before one exists.
func (l *lastState) base() *geom.Vec2 {
	if !l.hasPt {
		return nil
	}
	p := l.point
	return &p
}

// LastPoint returns the last point entered, typed or picked: the origin before any.
func (c *Context) LastPoint() geom.Vec2 { return c.last.Point() }

// SetLastPoint records p as the last point (ID and every command that takes a point call it;
// the session already records each point answered at a prompt).
func (c *Context) SetLastPoint(p geom.Vec2) { c.last.SetPoint(p) }

// LastEntity returns the entity the latest committed command added last, and false when no
// command has created one. The session sets it when a command's transaction commits.
func (c *Context) LastEntity() (doc.Handle, bool) { return c.last.entity, c.last.hasEnt }

// SetLastEntity overrides the last entity.
func (c *Context) SetLastEntity(h doc.Handle) { c.last.entity, c.last.hasEnt = h, true }

// LastEnd returns the end point and the end tangent direction (radians) of the last line or arc
// a draw command made, for Enter-continue; ok is false until a command sets it.
func (c *Context) LastEnd() (p geom.Vec2, tangent float64, ok bool) {
	return c.last.end, c.last.tangent, c.last.hasEnd
}

// SetLastEnd records the end point and tangent of the last line or arc.
func (c *Context) SetLastEnd(p geom.Vec2, tangent float64) {
	c.last.end, c.last.tangent, c.last.hasEnd = p, tangent, true
}

// LastScriptFile returns the script file SCRIPT ran last, the <default> of its prompt.
func (c *Context) LastScriptFile() string { return c.last.script }

// SetLastScriptFile records the script file SCRIPT ran.
func (c *Context) SetLastScriptFile(name string) { c.last.script = name }
