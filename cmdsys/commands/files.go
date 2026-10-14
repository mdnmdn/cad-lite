package commands

import (
	"path/filepath"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// --- NEW, OPEN, SAVE, QSAVE, SAVEAS, CLOSE, QUIT, EXIT ----------------------

// fileCmd is every file command. The ones that discard the drawing (NEW, OPEN, CLOSE, QUIT, EXIT)
// ask "Save changes to NAME? [Yes/No/Cancel]" first when the drawing is dirty; the ones that save
// ask for a path when the drawing has none. With a document host (one tab per drawing) OPEN and NEW
// open another tab instead and discard nothing, and CLOSE closes the tab.
type fileCmd struct {
	base
	name  string
	stage int
	// then is what the command does once the unsaved changes are dealt with.
	then func(ctx *cmdsys.Context)
	// opened is the drawing OPEN has read, installed by then.
	opened *doc.Drawing
	warns  []string
}

const (
	fileOpenPath = iota // OPEN: the path to read
	fileSavePath        // SAVE, SAVEAS, QSAVE: the path to write
	fileAsk             // Save changes to NAME?
	fileAskPath         // the path to save to before discarding the drawing
)

func (c *fileCmd) Name() string { return c.name }

// HumanOnly marks the file commands as unavailable to agents.
func (c *fileCmd) HumanOnly() bool { return true }

func (c *fileCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.name {
	case "OPEN":
		if ctx.Files == nil {
			ctx.Say("cmd.file.msg.unavailable")
			return nil
		}
		c.stage = fileOpenPath
		return &cmdsys.Prompt{Message: ctx.T("cmd.file.prompt.open_path"), Accepts: cmdsys.InputText}
	case "SAVE", "SAVEAS", "QSAVE":
		if ctx.Files == nil {
			ctx.Say("cmd.file.msg.unavailable")
			return nil
		}
		if c.name == "QSAVE" && ctx.FilePath() != "" && writablePath(ctx, ctx.FilePath()) == ctx.FilePath() {
			saveTo(ctx, ctx.FilePath())
			return nil
		}
		return c.savePathPrompt(ctx, fileSavePath)
	case "NEW", "CLOSE":
		if h := ctx.Documents(); h != nil && c.name == "NEW" && h.New() {
			return nil
		}
		c.then = func(ctx *cmdsys.Context) {
			if h := ctx.Documents(); h != nil && c.name == "CLOSE" && h.Close() {
				return
			}
			ctx.ReplaceDrawing(doc.New())
			ctx.SetFilePath("")
		}
	default: // QUIT, EXIT
		c.then = func(ctx *cmdsys.Context) {
			if !ctx.Quit() {
				ctx.Say("cmd.file.msg.quit_unavailable")
			}
		}
	}
	return c.discard(ctx)
}

// savePathPrompt asks where to save; the current path, when there is one, is the default.
func (c *fileCmd) savePathPrompt(ctx *cmdsys.Context, stage int) *cmdsys.Prompt {
	c.stage = stage
	p := &cmdsys.Prompt{Message: ctx.T("cmd.file.prompt.save_path"), Accepts: cmdsys.InputText}
	if path := ctx.FilePath(); path != "" {
		p.Default = &cmdsys.Input{Kind: cmdsys.InputText, Text: writablePath(ctx, path)}
	}
	return p
}

// writablePath is path, or, when the file handler cannot write its format (a DWG the drawing was
// opened from), the same name with an extension it can write.
func writablePath(ctx *cmdsys.Context, path string) string {
	if w, ok := ctx.Files.(interface{ WritablePath(string) string }); ok {
		return w.WritablePath(path)
	}
	return path
}

// discard runs then, after asking about unsaved changes when there are some and a way to save.
func (c *fileCmd) discard(ctx *cmdsys.Context) *cmdsys.Prompt {
	if !ctx.Dirty() || ctx.Files == nil {
		c.then(ctx)
		return nil
	}
	c.stage = fileAsk
	return &cmdsys.Prompt{Message: ctx.T("cmd.file.prompt.save_changes", drawingName(ctx)), Accepts: cmdsys.InputKeyword, NoOr: true,
		Keywords: []string{"Yes", "No", "Cancel"}, Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Yes"}}
}

func (c *fileCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case fileOpenPath:
		host := ctx.Documents()
		if host != nil && host.ShowOpen(in.Text) {
			return nil
		}
		d, warns, err := ctx.Files.Open(in.Text)
		if err != nil {
			ctx.Say("cmd.file.msg.open_failed", err)
			return nil
		}
		path := in.Text
		if host != nil {
			if s := host.OpenNew(path, d); s != nil {
				// The drawing went to a tab of its own; the current one stays as it is.
				printWarnings(s.Say, warns)
				ZoomExtents(d, s.View())
				s.Say("cmd.file.msg.opened", path, d.Len())
				return nil
			}
		}
		c.opened, c.warns = d, warns
		c.then = func(ctx *cmdsys.Context) {
			printWarnings(ctx.Say, c.warns)
			ctx.ReplaceDrawing(c.opened)
			ctx.SetFilePath(path)
			ZoomExtents(c.opened, ctx.View)
			ctx.Say("cmd.file.msg.opened", path, c.opened.Len())
		}
		return c.discard(ctx)
	case fileSavePath:
		saveTo(ctx, in.Text)
		return nil
	case fileAsk:
		switch in.Keyword {
		case "Cancel":
			return nil
		case "No":
			c.then(ctx)
			return nil
		}
		if path := ctx.FilePath(); path != "" && writablePath(ctx, path) == path {
			if saveTo(ctx, path) {
				c.then(ctx)
			}
			return nil
		}
		return c.savePathPrompt(ctx, fileAskPath)
	default: // fileAskPath
		if saveTo(ctx, in.Text) {
			c.then(ctx)
		}
		return nil
	}
}

// drawingName is the name "Save changes to NAME?" shows: the file's name, or the untitled name.
func drawingName(ctx *cmdsys.Context) string {
	if p := ctx.FilePath(); p != "" {
		return filepath.Base(p)
	}
	return ctx.T("cmd.file.msg.untitled")
}

// saveTo writes the drawing to path, records the path and marks the drawing saved. It reports
// whether the save worked.
func saveTo(ctx *cmdsys.Context, path string) bool {
	warns, err := ctx.Files.Save(path, ctx.Doc)
	if err != nil {
		ctx.Say("cmd.file.msg.save_failed", err)
		return false
	}
	printWarnings(ctx.Say, warns)
	ctx.SetFilePath(path)
	ctx.MarkSaved()
	ctx.Say("cmd.file.msg.saved", path, ctx.Doc.Len())
	return true
}

// printWarnings writes a reader's or writer's warnings through say, at most five.
func printWarnings(say func(key string, args ...any), warns []string) {
	const shown = 5
	for i, w := range warns {
		if i == shown {
			say("cmd.file.msg.more_warnings", len(warns)-shown)
			break
		}
		say("cmd.file.msg.warning", w)
	}
}

// registerFiles adds NEW, OPEN, SAVE, QSAVE, SAVEAS, CLOSE, QUIT and EXIT.
func registerFiles(r *cmdsys.Registry) {
	for _, n := range []string{"NEW", "OPEN", "SAVE", "QSAVE", "SAVEAS", "CLOSE", "QUIT", "EXIT"} {
		r.Register(n, func() cmdsys.Command { return &fileCmd{name: n} })
	}
}
