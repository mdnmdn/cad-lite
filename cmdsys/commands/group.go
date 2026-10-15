package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"fmt"
	"strings"
)

func registerGroup(r *cmdsys.Registry) {
	for _, name := range []string{"GROUP", "-GROUP", "UNGROUP"} {
		n := name
		r.Register(n, func() cmdsys.Command { return &groupCmd{name: n} })
	}
}

var groupOptions = []string{"List", "REname", "Add", "Remove", "Selectable", "Delete"}

type groupCmd struct {
	base
	selecting
	name, group, action string
	stage               int
}

func (c *groupCmd) Name() string { return c.name }
func (c *groupCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.name == "UNGROUP" {
		c.stage = 3
		if c.begin(ctx) {
			return c.apply(ctx)
		}
		return selectPrompt(ctx)
	}
	p := symbolPrompt(ctx, "group_name", cmdsys.InputText|cmdsys.InputKeyword|cmdsys.InputEnter)
	p.Keywords = groupOptions
	return p
}
func (c *groupCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case 0:
		s := strings.TrimSpace(in.Text)
		if in.Kind == cmdsys.InputKeyword {
			s = in.Keyword
		}
		switch strings.ToUpper(s) {
		case "LIST":
			for _, g := range ctx.Doc.Groups() {
				ctx.Say("cmd.symbol.msg.group", g.Name, len(g.Members))
			}
			return nil
		case "RENAME", "ADD", "REMOVE", "SELECTABLE", "DELETE":
			c.action = strings.ToUpper(s)
			c.stage = 1
			return symbolPrompt(ctx, "existing_group", cmdsys.InputText)
		}
		c.group = s
		c.action = "CREATE"
		c.stage = 3
		if c.begin(ctx) {
			return c.apply(ctx)
		}
		return selectPrompt(ctx)
	case 1:
		c.group = strings.TrimSpace(in.Text)
		g := ctx.Doc.Group(c.group)
		if g == nil {
			symbolError(ctx, fmt.Errorf("group does not exist"))
			return nil
		}
		switch c.action {
		case "DELETE":
			ctx.Tx().EraseGroup(g.H)
			return nil
		case "SELECTABLE":
			return symbolError(ctx, ctx.Tx().ModifyGroup(g.H, func(g *doc.Group) { g.Selectable = !g.Selectable }))
		case "RENAME":
			c.stage = 2
			return symbolPrompt(ctx, "new_name", cmdsys.InputText)
		default:
			c.stage = 3
			if c.begin(ctx) {
				return c.apply(ctx)
			}
			return selectPrompt(ctx)
		}
	case 2:
		g := ctx.Doc.Group(c.group)
		return symbolError(ctx, ctx.Tx().ModifyGroup(g.H, func(g *doc.Group) { g.Name = strings.TrimSpace(in.Text); g.Unnamed = false }))
	case 3:
		if !c.take(ctx, in) {
			return nil
		}
		return c.apply(ctx)
	}
	return nil
}
func (c *groupCmd) apply(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.name == "UNGROUP" {
		seen := map[doc.Handle]bool{}
		for _, h := range c.hs {
			for _, g := range ctx.Doc.GroupsFor(h) {
				if !seen[g.H] {
					ctx.Tx().EraseGroup(g.H)
					seen[g.H] = true
				}
			}
		}
		return nil
	}
	if c.action == "CREATE" {
		_, err := ctx.Tx().AddGroup(&doc.Group{Name: c.group, Selectable: true, Members: c.hs})
		return symbolError(ctx, err)
	}
	g := ctx.Doc.Group(c.group)
	return symbolError(ctx, ctx.Tx().ModifyGroup(g.H, func(g *doc.Group) {
		if c.action == "ADD" {
			g.Members = append(g.Members, c.hs...)
		} else {
			var keep []doc.Handle
			for _, h := range g.Members {
				remove := false
				for _, r := range c.hs {
					if h == r {
						remove = true
					}
				}
				if !remove {
					keep = append(keep, h)
				}
			}
			g.Members = keep
		}
	}))
}
func symbolPrompt(ctx *cmdsys.Context, key string, kind cmdsys.InputKind) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.symbol.prompt." + key), Accepts: kind}
}
func symbolError(ctx *cmdsys.Context, err error) *cmdsys.Prompt {
	if err != nil {
		message := err.Error()
		if key := symbolErrors[message]; key != "" {
			message = ctx.T("cmd.symbol.error." + key)
		} else if strings.HasPrefix(message, "block ") && strings.HasSuffix(message, " does not exist") {
			message = ctx.T("cmd.symbol.error.block_does_not_exist")
		}
		ctx.Say("cmd.symbol.msg.error", message)
	}
	return nil
}

var symbolErrors = map[string]string{
	"attribute tag must be a nonempty word":                 "attribute_tag_must_be_a_nonempty_word",
	"block already exists; use BLOCKREDEFINE":               "block_already_exists_use_blockredefine",
	"block definition does not exist":                       "block_definition_does_not_exist",
	"block does not exist":                                  "block_does_not_exist",
	"block does not exist; a library path must end in .dxf": "block_does_not_exist_a_library_path_must_end_in_dxf",
	"block is referenced":                                   "block_is_referenced",
	"cannot explode a nonuniformly scaled reference":        "cannot_explode_a_nonuniformly_scaled_reference",
	"cannot explode unsupported block objects":              "cannot_explode_unsupported_block_objects",
	"cyclic block definition":                               "cyclic_block_definition",
	"editable attribute tag does not exist":                 "editable_attribute_tag_does_not_exist",
	"file service unavailable":                              "file_service_unavailable",
	"group does not exist":                                  "group_does_not_exist",
	"group member does not exist":                           "group_member_does_not_exist",
	"group name already exists":                             "group_name_already_exists",
	"group name is empty":                                   "group_name_is_empty",
	"invalid attribute height":                              "invalid_attribute_height",
	"invalid block base point":                              "invalid_block_base_point",
	"invalid block name":                                    "invalid_block_name",
	"invalid block reference":                               "invalid_block_reference",
	"invalid insertion transform":                           "invalid_insertion_transform",
	"library block name conflicts":                          "library_block_name_conflicts",
	"library definition name conflicts":                     "library_definition_name_conflicts",
	"library drawing is empty":                              "library_drawing_is_empty",
	"library has cyclic or missing definitions":             "library_has_cyclic_or_missing_definitions",
	"library has unsupported objects":                       "library_has_unsupported_objects",
	"no block reference selected":                           "no_block_reference_selected",
	"object is not a block reference":                       "object_is_not_a_block_reference",
	"object is on a locked or hidden layer":                 "object_is_on_a_locked_or_hidden_layer",
	"scale must be finite and positive":                     "scale_must_be_finite_and_positive",
	"unsupported group member":                              "unsupported_group_member",
}
