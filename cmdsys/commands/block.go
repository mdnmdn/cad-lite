package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"errors"
	"math"
	"path/filepath"
	"strings"
)

func registerBlock(r *cmdsys.Registry) {
	for _, name := range []string{"BLOCK", "-BLOCK", "BLOCKREDEFINE", "INSERT", "-INSERT", "WBLOCK"} {
		n := name
		r.Register(n, func() cmdsys.Command { return &blockCmd{name: n} })
	}
	r.Alias("B", "BLOCK")
	r.Alias("I", "INSERT")
}

type blockCmd struct {
	base
	selecting
	name, block string
	stage       int
	point       geom.Vec2
	scale       float64
	insert      *doc.Insert
	attrs       []*doc.AttributeDefinition
	attr        int
	library     *doc.Drawing
}

func (c *blockCmd) Name() string    { return c.name }
func (c *blockCmd) HumanOnly() bool { return c.name == "WBLOCK" }
func (c *blockCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return symbolPrompt(ctx, "block_name", cmdsys.InputText)
}
func (c *blockCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case 0:
		c.block = strings.TrimSpace(in.Text)
		if c.name == "INSERT" || c.name == "-INSERT" {
			if ctx.Doc.Block(c.block) == nil {
				if ctx.AgentCaller() {
					ctx.Say("session.agent_forbidden", c.name)
					return nil
				}
				if !strings.EqualFold(filepath.Ext(c.block), ".dxf") {
					return symbolError(ctx, errors.New("block does not exist; a library path must end in .dxf"))
				}
				if ctx.Files == nil {
					return symbolError(ctx, errors.New("file service unavailable"))
				}
				d, w, err := ctx.Files.Open(c.block)
				for _, line := range w {
					ctx.Printf("%s", line)
				}
				if err != nil {
					return symbolError(ctx, err)
				}
				c.library = d
				c.block = strings.TrimSuffix(filepath.Base(c.block), filepath.Ext(c.block))
				if err = validateLibrary(ctx.Doc, c.library, c.block); err != nil {
					return symbolError(ctx, err)
				}
			}
			c.stage = 4
			return symbolPrompt(ctx, "insert_point", cmdsys.InputPoint)
		}
		if c.name == "WBLOCK" {
			if ctx.Doc.Block(c.block) == nil {
				return symbolError(ctx, errors.New("block does not exist"))
			}
			c.stage = 8
			return symbolPrompt(ctx, "library_path", cmdsys.InputText)
		}
		if c.name != "BLOCKREDEFINE" && ctx.Doc.Block(c.block) != nil {
			return symbolError(ctx, errors.New("block already exists; use BLOCKREDEFINE"))
		}
		if c.name == "BLOCKREDEFINE" && ctx.Doc.Block(c.block) == nil {
			return symbolError(ctx, errors.New("block does not exist"))
		}
		c.stage = 1
		return symbolPrompt(ctx, "base_point", cmdsys.InputPoint)
	case 1:
		c.point = in.Point
		c.stage = 2
		if c.begin(ctx) {
			return c.define(ctx)
		}
		return selectPrompt(ctx)
	case 2:
		if c.take(ctx, in) {
			return c.define(ctx)
		}
		return nil
	case 4:
		c.point = in.Point
		c.stage = 5
		p := symbolPrompt(ctx, "scale", cmdsys.InputDistance)
		p.Default = &cmdsys.Input{Kind: cmdsys.InputDistance, Value: 1}
		return p
	case 5:
		if in.Value <= 0 || math.IsInf(in.Value, 0) || math.IsNaN(in.Value) {
			return symbolError(ctx, errors.New("scale must be finite and positive"))
		}
		c.scale = in.Value
		c.stage = 6
		p := symbolPrompt(ctx, "rotation", cmdsys.InputAngle)
		p.Default = &cmdsys.Input{Kind: cmdsys.InputAngle, Value: 0}
		return p
	case 6:

		c.insert = doc.NewInsert(c.block, c.point)
		c.insert.ScaleX, c.insert.ScaleY, c.insert.Rotation = c.scale, c.scale, in.Value
		definition := ctx.Doc.Block(c.block)
		if c.library != nil {
			definition = c.library.Block(c.block)
		}
		if definition != nil {
			for _, e := range definition.Entities {
				if a, ok := e.(*doc.AttributeDefinition); ok && !a.Constant {
					c.attrs = append(c.attrs, a)
				}
			}
		}
		c.stage = 7
		if len(c.attrs) > 0 {
			return c.attributePrompt(ctx)
		}
		return c.place(ctx)
	case 7:
		a := c.attrs[c.attr]
		value := in.Text
		if in.Kind == cmdsys.InputEnter {
			value = a.Value
		}
		c.insert.Attributes = append(c.insert.Attributes, doc.Attribute{Tag: a.Tag, Value: value})
		c.attr++
		if c.attr < len(c.attrs) {
			return c.attributePrompt(ctx)
		}
		return c.place(ctx)
	case 8:
		if ctx.AgentCaller() {
			ctx.Say("session.agent_forbidden", c.name)
			return nil
		}
		if ctx.Files == nil {
			return symbolError(ctx, errors.New("file service unavailable"))
		}
		out := doc.New()
		out.Header.HandSeed = ctx.Doc.Header.HandSeed
		out.Preserved.Tables = ctx.Doc.Preserved.Tables
		for _, l := range ctx.Doc.Layers() {
			*out.AddLayer(l.Name, l.Color) = *l
		}
		for _, b := range ctx.Doc.Blocks() {
			out.LoadBlock(b)
		}
		tx, _ := out.Begin()
		tx.Add(doc.NewInsert(c.block, geom.V(0, 0)))
		tx.Commit()
		w, err := ctx.Files.Save(in.Text, out)
		for _, line := range w {
			ctx.Printf("%s", line)
		}
		return symbolError(ctx, err)
	}
	return nil
}
func (c *blockCmd) attributePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	a := c.attrs[c.attr]
	label := a.Prompt
	if label == "" {
		label = a.Tag
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.symbol.prompt.attribute", label, a.Value), Accepts: cmdsys.InputText | cmdsys.InputEnter, Literal: true}
}
func (c *blockCmd) define(ctx *cmdsys.Context) *cmdsys.Prompt {
	b := &doc.Block{Name: c.block, Base: c.point}
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		if !ctx.Doc.EntityEditable(e) {
			return symbolError(ctx, errors.New("object is on a locked or hidden layer"))
		}
		b.Entities = append(b.Entities, e.Clone().(doc.Entity))
	}
	if len(b.Entities) == 0 {
		return nil
	}
	if err := ctx.Tx().DefineBlock(b); err != nil {
		return symbolError(ctx, err)
	}
	if c.name != "BLOCKREDEFINE" {
		for _, h := range c.hs {
			ctx.Tx().Erase(h)
		}
		ctx.Tx().Add(doc.NewInsert(c.block, c.point))
	}
	return nil
}
func libraryPlan(d, src *doc.Drawing, name string) ([]*doc.Block, error) {
	if d.Block(name) != nil {
		return nil, errors.New("library block name conflicts")
	}
	pending := src.Blocks()
	for _, b := range pending {
		if d.Block(b.Name) != nil {
			return nil, errors.New("library definition name conflicts")
		}
		for _, e := range b.Entities {
			if _, ok := e.(*doc.Unknown); ok {
				return nil, errors.New("library has unsupported objects")
			}
		}
	}
	if src.Len() == 0 {
		return nil, errors.New("library drawing is empty")
	}
	direct := false
	if src.Len() == 1 {
		if i, ok := src.Entity(src.Handles()[0]).(*doc.Insert); ok && strings.EqualFold(i.BlockName, name) && i.Position == geom.V(0, 0) && i.Rotation == 0 && i.ScaleX == 1 && i.ScaleY == 1 {
			direct = true
		}
	}
	if !direct {
		if src.Block(name) != nil {
			return nil, errors.New("library definition name conflicts")
		}
		b := &doc.Block{Name: name}
		for _, h := range src.Handles() {
			e := src.Entity(h)
			if _, ok := e.(*doc.Unknown); ok {
				return nil, errors.New("library has unsupported objects")
			}
			b.Entities = append(b.Entities, e.Clone().(doc.Entity))
		}
		pending = append(pending, b)
	}
	temp := doc.New()
	for _, b := range d.Blocks() {
		temp.LoadBlock(b)
	}
	tx, _ := temp.Begin()
	var plan []*doc.Block
	for len(pending) > 0 {
		var next []*doc.Block
		progress := false
		for _, b := range pending {
			ready := true
			for _, e := range b.Entities {
				if i, ok := e.(*doc.Insert); ok && temp.Block(i.BlockName) == nil {
					ready = false
				}
			}
			if !ready {
				next = append(next, b)
				continue
			}
			if err := tx.DefineBlock(b); err != nil {
				return nil, err
			}
			plan = append(plan, b)
			progress = true
		}
		if !progress {
			return nil, errors.New("library has cyclic or missing definitions")
		}
		pending = next
	}
	tx.Abort()
	return plan, nil
}
func validateLibrary(d, src *doc.Drawing, name string) error {
	_, err := libraryPlan(d, src, name)
	return err
}
func importLibrary(ctx *cmdsys.Context, src *doc.Drawing, name string) error {
	plan, err := libraryPlan(ctx.Doc, src, name)
	if err != nil {
		return err
	}
	for _, b := range plan {
		if err := ctx.Tx().DefineBlock(b); err != nil {
			ctx.Tx().Abort()
			return err
		}
	}
	ctx.Tx().ImportBlockSupport(src)
	return nil
}

func (c *blockCmd) place(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.library != nil {
		if err := importLibrary(ctx, c.library, c.block); err != nil {
			return symbolError(ctx, err)
		}
	}
	if h := ctx.Tx().Add(c.insert); h == 0 {
		ctx.Tx().Abort()
		return symbolError(ctx, errors.New("invalid insertion transform"))
	}
	return nil
}
