package commands

import "cad-lite/cmdsys"

// --- AI -----------------------------------------------------------------

// aiCmd asks the host's AI agent a question. The prompt takes free text, so
// Space does not submit it; an empty response toggles the agent panel.
type aiCmd struct{ base }

func (*aiCmd) Name() string { return "AI" }

func (*aiCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !ctx.HasAgent() {
		ctx.Say("cmd.ai.msg.unavailable")
		return nil
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.ai.prompt.ask"), Accepts: cmdsys.InputText | cmdsys.InputEnter}
}

func (*aiCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputText {
		ctx.Agent(in.Text)
	} else {
		ctx.Agent("")
	}
	return nil
}

func registerAgent(r *cmdsys.Registry) {
	r.Register("AI", func() cmdsys.Command { return &aiCmd{} })
	r.Alias("AGENT", "AI")
}
