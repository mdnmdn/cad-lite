package commands

import (
	"strings"

	"go-draw/cmdsys"
	"go-draw/geom"
	"go-draw/render"
)

// multipleCmd is MULTIPLE: it asks for a command name and runs that command again each time it
// completes, until Escape cancels it. Every completed run is its own undo step.
type multipleCmd struct {
	reg   *cmdsys.Registry
	name  string
	inner cmdsys.Command
}

func (c *multipleCmd) Name() string {
	if c.inner != nil {
		return c.inner.Name()
	}
	return "MULTIPLE"
}

func (c *multipleCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.multiple.prompt.name"), Accepts: cmdsys.InputText}
}

// begin starts a fresh instance of the repeated command and returns its first prompt.
func (c *multipleCmd) begin(ctx *cmdsys.Context) *cmdsys.Prompt {
	inner, ok := c.reg.Lookup(c.name)
	if !ok {
		return nil
	}
	c.inner = inner
	return inner.Start(ctx)
}

func (c *multipleCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.inner == nil {
		name := strings.ToUpper(strings.TrimSpace(in.Text))
		inner, ok := c.reg.Lookup(name)
		switch {
		case !ok:
			ctx.Say("session.unknown_command", name)
			return nil
		case inner.Name() == "MULTIPLE":
			return nil
		}
		c.name = inner.Name()
		return c.begin(ctx)
	}
	if p := c.inner.Input(ctx, in); p != nil {
		return p
	}
	// The run completed: keep it as one undo step and start the next. A command that ends in
	// Start (nothing to ask) would repeat forever, so it ends the loop.
	ctx.CommitStep()
	return c.begin(ctx)
}

func (c *multipleCmd) Cancel(ctx *cmdsys.Context) {
	if c.inner != nil {
		c.inner.Cancel(ctx)
	}
}

func (c *multipleCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.inner != nil {
		c.inner.Preview(ctx, cur, dl)
	}
}

// registerMultiple adds MULTIPLE. RESUME is not provided: a script runs to its end without a
// chance to interrupt it, so there is nothing to resume.
func registerMultiple(r *cmdsys.Registry) {
	r.Register("MULTIPLE", func() cmdsys.Command { return &multipleCmd{reg: r} })
}
