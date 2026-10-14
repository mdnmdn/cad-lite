package commands

import "cad-lite/cmdsys"

// registerSelect adds the commands of select.go.
func registerSelect(r *cmdsys.Registry) {
	r.Register("SELECT", func() cmdsys.Command { return &selectCmd{} })
}

// selectCmd is SELECT: it builds a selection with every option of the Select objects prompt
// and leaves it as the Previous selection for the next command's P option.
type selectCmd struct {
	base
	selecting
}

func (*selectCmd) Name() string { return "SELECT" }

func (c *selectCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return selectPrompt(ctx) }

func (c *selectCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	c.take(ctx, in)
	return nil
}
