package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/i18n"
)

// Register adds every built-in command and the default aliases to r. Each command file owns one
// registerX function and Register calls it; a package that adds commands edits only its own
// registerX (every owned file already has one, see the ownership table in
// _docs/tech/command-system.md), so this list does not change after the seam split.
func Register(r *cmdsys.Registry) {
	registerGroup(r)
	registerBlock(r)
	registerAttribute(r)
	registerDraw(r)      // draw.go, polygon.go, point.go: CW-A
	registerModify(r)    // modify.go, array.go, oops.go: CW-B1
	registerConstruct(r) // construct.go: CW-B2
	registerTrim(r)      // trim.go: CW-B3
	registerMeasure(r)   // measure.go: CW-C
	registerInquiry(r)   // inquiry.go: CW-C
	registerUnits(r)     // units.go: CW-C
	registerView(r)      // view.go: CW-D
	registerLayer(r)     // layer.go: CW-D
	registerMatchprop(r) // matchprop.go: CW-D
	registerProps(r)     // props.go: CW-D
	registerPurge(r)     // purge.go: CW-D
	registerUndo(r)      // undo.go: CW-E
	registerFiles(r)     // files.go: CW-E
	registerSysvar(r)    // sysvar.go: CW-G
	registerScript(r)    // script.go: CW-G
	registerSelect(r)    // select.go: WP2
	registerAids(r)      // aids.go: WP3
	registerText(r)      // text.go: N9
	registerMText(r)     // mtext.go: N9
	registerTextEdit(r)  // textedit.go: N26
	registerStretch(r)   // stretch.go: WP4
	registerAgent(r)     // agent.go: host
	registerSettings(r)  // settings.go: host
	r.LoadAliases(Aliases)
	// English short aliases and the Italian names of every language, whatever the UI language.
	i18n.RegisterAliases(r.Alias)
}

// NewRegistry returns a registry with every built-in command registered.
func NewRegistry() *cmdsys.Registry {
	r := cmdsys.NewRegistry()
	Register(r)
	return r
}
