package cmdsys

// SetSettingsHandler installs the host's settings panel entry point, which the OPTIONS command
// calls. Without a handler the command reports that settings are not available.
func (s *Session) SetSettingsHandler(f func()) { s.ctx.settings = f }
