package commands

import (
	"path/filepath"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
)

// --- NEW, OPEN, SAVE, QSAVE, SAVEAS, CLOSE, QUIT, EXIT ----------------------

// fileCmd is every file command. The ones that discard the drawing (NEW, OPEN, CLOSE, QUIT, EXIT)
// ask "Save changes to NAME? [Yes/No/Cancel]" first when the drawing is dirty; the ones that save
// ask for a path when the drawing has none. With a document host (one tab per drawing) OPEN and NEW
// open another tab instead and discard nothing, and CLOSE closes the tab. A host that implements
// cmdsys.DiscardConfirmer asks CLOSE's and QUIT's question with a dialog instead, outside scripts.
type fileCmd struct {
	base
	name  string
	stage int
	// then is what the command does once the unsaved changes are dealt with.
	then func(ctx *cmdsys.Context)
	// opened is the drawing OPEN has read, installed by then.
	opened *doc.Drawing
	warns  []string
	// version is the file format SAVEAS's format prompt chose; pinned says one was chosen (a path
	// typed at that prompt keeps the drawing's own).
	version string
	pinned  bool
}

const (
	fileOpenPath = iota // OPEN: the path to read
	fileSavePath        // SAVE, SAVEAS, QSAVE: the path to write
	fileAsk             // Save changes to NAME?
	fileAskPath         // the path to save to before discarding the drawing
	fileFormat          // SAVEAS: the file format, or a path saved in the drawing's own format
)

// fileFormats is what a file handler offers when its writers write several versions
// (fileio.Registry does); SAVEAS asks for the format only through it.
type fileFormats interface {
	SaveVersions(path string) []string
	SaveVersion(path string, d *doc.Drawing, version string) string
}

// formatKeywords are the SAVEAS format prompt's keywords by version, as AutoCAD's -SAVEAS names
// its formats.
var formatKeywords = map[string]string{"AC1009": "R12", "AC1015": "2000"}

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
		if c.name == "SAVEAS" {
			if p := c.formatPrompt(ctx); p != nil {
				return p
			}
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
		if h := confirmer(ctx); h != nil && c.name == "CLOSE" && ctx.Dirty() && h.ConfirmClose(func() { c.then(ctx) }) {
			return nil
		}
	default: // QUIT, EXIT
		if h := confirmer(ctx); h != nil && h.ConfirmQuit() {
			return nil
		}
		c.then = func(ctx *cmdsys.Context) {
			if !ctx.Quit() {
				ctx.Say("cmd.file.msg.quit_unavailable")
			}
		}
	}
	return c.discard(ctx)
}

// confirmer returns the document host when it asks about unsaved changes with a dialog of its own,
// and nil when the command line asks: no such host, or a script feeds the session (its next lines
// answer the question).
func confirmer(ctx *cmdsys.Context) cmdsys.DiscardConfirmer {
	h, ok := ctx.Documents().(cmdsys.DiscardConfirmer)
	if !ok || ctx.Scripted() {
		return nil
	}
	return h
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

// formatPrompt prints the drawing's file format and asks for the one to save in, as AutoCAD's
// -SAVEAS does: "Enter file format [R12/2000] <2000>:", the default being the format a save writes
// now. It returns nil when the handler offers no choice of format.
func (c *fileCmd) formatPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	ff, ok := ctx.Files.(fileFormats)
	if !ok {
		return nil
	}
	path := ""
	if p := ctx.FilePath(); p != "" {
		path = writablePath(ctx, p)
	}
	var kws []string
	for _, v := range ff.SaveVersions(path) {
		if kw := formatKeywords[v]; kw != "" {
			kws = append(kws, kw)
		}
	}
	current := ff.SaveVersion(path, ctx.Doc, ctx.Doc.Header.Version)
	if len(kws) < 2 || formatKeywords[current] == "" {
		return nil
	}
	if ctx.Doc.Header.Version == "" {
		ctx.Say("cmd.file.msg.format_auto", FormatName(ctx.Translator(), current))
	} else {
		ctx.Say("cmd.file.msg.format", FormatName(ctx.Translator(), current))
	}
	c.stage = fileFormat
	return &cmdsys.Prompt{Message: ctx.T("cmd.file.prompt.format"), Accepts: cmdsys.InputKeyword | cmdsys.InputText, NoOr: true,
		Keywords: kws, Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: formatKeywords[current]}}
}

// FormatName is the name of a file format version in the language of t ("AutoCAD 2000/LT2000
// DXF" for AC1015), or the version itself when it has none.
func FormatName(t i18n.Translator, version string) string {
	key := "dxf.version." + strings.ToLower(version)
	if n := t.T(key); n != key {
		return n
	}
	return version
}

// FormatKeyword is the SAVEAS format prompt's keyword for version ("2000" for AC1015), or "".
func FormatKeyword(version string) string { return formatKeywords[version] }

// save writes the drawing to path, in the format the format prompt chose when it chose one; the
// drawing keeps that format for later saves (QSAVE), and keeps its old one when the save fails.
func (c *fileCmd) save(ctx *cmdsys.Context, path string) bool {
	if !c.pinned {
		return saveTo(ctx, path)
	}
	prev := ctx.Doc.Header.Version
	ctx.Doc.Header.Version = c.version
	if !saveTo(ctx, path) {
		ctx.Doc.Header.Version = prev
		return false
	}
	return true
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
	case fileFormat:
		if in.Kind == cmdsys.InputKeyword {
			for v, kw := range formatKeywords {
				if kw == in.Keyword {
					c.version, c.pinned = v, true
				}
			}
			return c.savePathPrompt(ctx, fileSavePath)
		}
		// A path at the format prompt (a script's "SAVEAS plan.dxf") saves in the drawing's format.
		c.save(ctx, in.Text)
		return nil
	case fileSavePath:
		c.save(ctx, in.Text)
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
