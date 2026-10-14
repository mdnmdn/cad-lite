package commands

import "cad-lite/cmdsys"

// optionsCmd opens the host's settings panel.
type optionsCmd struct{ base }

func (*optionsCmd) Name() string { return "OPTIONS" }

func (*optionsCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !ctx.HasSettings() {
		ctx.Say("cmd.options.msg.unavailable")
		return nil
	}
	ctx.OpenSettings()
	return nil
}

func (*optionsCmd) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt { return nil }

func registerSettings(r *cmdsys.Registry) {
	r.Register("OPTIONS", func() cmdsys.Command { return &optionsCmd{} })
	r.Alias("OP", "OPTIONS")
}
