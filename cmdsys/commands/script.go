package commands

import (
	"os"
	"path/filepath"

	"go-draw/cmdsys"
)

// --- SCRIPT, RSCRIPT, DELAY ----------------------------------------------

// maxDelay is the largest DELAY AutoCAD accepts, in milliseconds.
const maxDelay = 32767

type scriptCmd struct {
	base
	name string
}

func (c *scriptCmd) Name() string { return c.name }

// HumanOnly marks SCRIPT as unavailable to agents: it reads a file from disk.
func (c *scriptCmd) HumanOnly() bool { return c.name == "SCRIPT" }

func (c *scriptCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.name {
	case "RSCRIPT":
		ctx.RestartScript()
		return nil
	case "DELAY":
		return &cmdsys.Prompt{Message: ctx.T("cmd.delay.prompt.time"), Accepts: cmdsys.InputInteger}
	}
	p := &cmdsys.Prompt{Message: ctx.T("cmd.script.prompt.file"), Accepts: cmdsys.InputText}
	if last := ctx.LastScriptFile(); last != "" {
		p.Default = &cmdsys.Input{Kind: cmdsys.InputText, Text: last}
	}
	return p
}

func (c *scriptCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.name == "DELAY" {
		// Scripts run synchronously, so there is nothing to wait for: the time is validated and
		// dropped.
		if in.Value < 0 || in.Value > maxDelay {
			ctx.Say("cmd.delay.msg.range", maxDelay)
			return &cmdsys.Prompt{Message: ctx.T("cmd.delay.prompt.time"), Accepts: cmdsys.InputInteger}
		}
		return nil
	}
	name := in.Text
	data, err := os.ReadFile(name)
	if err != nil && filepath.Ext(name) == "" {
		data, err = os.ReadFile(name + ".scr")
	}
	if err != nil {
		ctx.Say("cmd.script.msg.cannot_open", name)
		return nil
	}
	ctx.SetLastScriptFile(name)
	ctx.RunScript(string(data))
	return nil
}

// registerScript adds SCRIPT (alias SCR), RSCRIPT and DELAY. RESUME is not provided: a script
// runs to its end without a chance to interrupt it.
func registerScript(r *cmdsys.Registry) {
	for _, n := range []string{"SCRIPT", "RSCRIPT", "DELAY"} {
		r.Register(n, func() cmdsys.Command { return &scriptCmd{name: n} })
	}
	r.Alias("SCR", "SCRIPT")
}
