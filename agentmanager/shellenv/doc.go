// Package shellenv finds the PATH a user's terminal has but a GUI process
// often lacks, and uses it to locate agent executables.
//
// A desktop app started from the macOS Finder or Dock inherits launchd's
// minimal PATH (no /opt/homebrew/bin, ~/.local/bin, ~/.bun/bin, npm global
// bins, mise or asdf shims). On Windows a GUI process keeps the PATH from
// login time, so a program installed later is not seen. Some Linux desktop
// launchers skip ~/.bashrc. LoginPATH asks the login shell (Unix) or the
// registry (Windows) for the real PATH; Merge, Find and the process-wide
// Set/Effective/Env put it to use. The package imports only the standard
// library.
package shellenv
