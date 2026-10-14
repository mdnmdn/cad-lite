package commands

import "cad-lite/cmdsys"

// registerSysvar adds SETVAR. The command, the value prompts and the error sentences live in
// cmdsys/sysvars.go next to the variable registry; a variable typed by name at Command: needs no
// registration, the session resolves it after the command table (Session.Run).
func registerSysvar(r *cmdsys.Registry) {
	r.Register("SETVAR", cmdsys.NewSetvar)
	r.Alias("SETV", "SETVAR")
}
