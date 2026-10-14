package commands

import (
	"slices"
	"strings"

	"go-draw/cmdsys"
	"go-draw/doc"
)

// The PURGE keyword ids.
const (
	pgLayers = "LAyers"
	pgZero   = "Zero-length geometry"
	pgAll    = "zAll"
)

type purgeStep int

const (
	pgType purgeStep = iota
	pgNames
	pgVerify
	pgEach
)

// purgeCmd is -PURGE and PURGE (which has no dialog here): unused layers, zero-length geometry,
// or both.
type purgeCmd struct {
	base
	name   string
	step   purgeStep
	kind   string
	names  string
	verify bool
	queue  []*doc.Layer
}

func (c *purgeCmd) Name() string { return c.name }

func (c *purgeCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.purge.prompt.type"), Accepts: cmdsys.InputKeyword, Keywords: []string{pgLayers, pgZero, pgAll}, NoOr: true}
}

func (c *purgeCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case pgType:
		c.kind = in.Keyword
		if c.kind == pgZero {
			c.zeroLength(ctx)
			return nil
		}
		c.step = pgNames
		def := cmdsys.Input{Kind: cmdsys.InputText, Text: "*"}
		return &cmdsys.Prompt{Message: ctx.T("cmd.purge.prompt.names"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Default: &def}
	case pgNames:
		c.names = "*"
		if in.Kind == cmdsys.InputText {
			c.names = in.Text
		}
		c.step = pgVerify
		def := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Yes"}
		return &cmdsys.Prompt{Message: ctx.T("cmd.purge.prompt.verify"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: []string{"Yes", "No"}, Default: &def}
	case pgVerify:
		c.verify = in.Keyword != "No"
		if c.kind == pgAll {
			c.zeroLength(ctx)
		}
		c.queue = c.unused(ctx)
		return c.next(ctx)
	case pgEach:
		l := c.queue[0]
		c.queue = c.queue[1:]
		if in.Keyword == "Yes" {
			c.remove(ctx, l)
		}
		return c.next(ctx)
	}
	return nil
}

// unused returns the layers matching the names no object is on, other than layer 0 and the
// current layer.
func (c *purgeCmd) unused(ctx *cmdsys.Context) []*doc.Layer {
	var out []*doc.Layer
	used := map[string]bool{"0": true, ctx.Doc.CurrentLayer: true}
	for _, h := range ctx.Doc.Handles() {
		used[ctx.Doc.Entity(h).Common().Layer] = true
	}
	for _, pat := range strings.Split(c.names, ",") {
		for _, l := range ctx.Doc.Layers() {
			if !used[l.Name] && wildcardMatch(strings.TrimSpace(pat), l.Name) {
				out = append(out, l)
				used[l.Name] = true
			}
		}
	}
	return out
}

func (c *purgeCmd) next(ctx *cmdsys.Context) *cmdsys.Prompt {
	for len(c.queue) > 0 {
		if c.verify {
			c.step = pgEach
			def := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "No"}
			return &cmdsys.Prompt{Message: ctx.T("cmd.purge.prompt.each", c.queue[0].Name), Accepts: cmdsys.InputKeyword, Keywords: []string{"Yes", "No"}, Default: &def}
		}
		c.remove(ctx, c.queue[0])
		c.queue = c.queue[1:]
	}
	return nil
}

func (c *purgeCmd) remove(ctx *cmdsys.Context, l *doc.Layer) {
	name := l.Name
	if ctx.Doc.RemoveLayer(name) {
		ctx.Say("cmd.purge.msg.deleted", name)
	}
}

// zeroLength erases the objects with no length: a line whose ends coincide, a circle or arc of
// radius 0, an arc with no sweep, a polyline whose vertices all coincide.
func (c *purgeCmd) zeroLength(ctx *cmdsys.Context) {
	n := 0
	for _, h := range slices.Clone(ctx.Doc.Handles()) {
		e := ctx.Doc.Entity(h)
		if e != nil && zeroLength(e) && editable(ctx, e) {
			ctx.Tx().Erase(h)
			n++
		}
	}
	ctx.SayPlural("cmd.purge.msg.zero_length", n)
}

func zeroLength(e doc.Entity) bool {
	switch e := e.(type) {
	case *doc.Line:
		return e.Start.Near(e.End, 1e-12)
	case *doc.Circle:
		return e.Radius <= 0
	case *doc.Arc:
		return e.Radius <= 0 || e.StartAngle == e.EndAngle
	case *doc.LWPolyline:
		for _, p := range e.Points[min(1, len(e.Points)):] {
			if !p.Near(e.Points[0], 1e-12) {
				return false
			}
		}
		return len(e.Points) > 0
	}
	return false
}

// registerPurge adds PURGE and -PURGE (alias PU).
func registerPurge(r *cmdsys.Registry) {
	r.Register("PURGE", func() cmdsys.Command { return &purgeCmd{name: "PURGE"} })
	r.Alias("PU", "PURGE")
	cmdsys.RegisterCLI(r, "PURGE", func() cmdsys.Command { return &purgeCmd{name: "-PURGE"} }, "PU")
}
