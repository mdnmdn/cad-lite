package cmdsys

import "errors"

// Catalog keys of the messages the session itself prints, so a package that words a failure the
// AutoCAD way (section 1.12 of _docs/inbox/autocad-commands.md) picks the key here and adds the
// text to both locale files, instead of spelling a key literal that drifts.
const (
	MsgInvalidPoint = "input.err.invalid_point"
	MsgInvalidValue = "input.err.invalid_value"
	MsgNoBase       = "input.err.no_base"
	MsgInvalidInput = "input.err.invalid_input"
	MsgCancel       = "session.cancel"
	MsgUnknownCmd   = "session.unknown_command"
)

// InputErrorKey returns the catalog key of the sentence the command line prints when ParseInput
// fails with err at prompt p. It is the one place that chooses the wording: today every prompt
// kind gets the same sentence per error; the wording work makes it depend on p (bad point with
// keywords offered, bad distance, bad angle, bad keyword) without touching ParseInput's callers.
func InputErrorKey(err error, p *Prompt) string {
	switch {
	case errors.Is(err, ErrInvalidPoint):
		return MsgInvalidPoint
	case errors.Is(err, ErrInvalidValue):
		return MsgInvalidValue
	case errors.Is(err, ErrNoBase):
		return MsgNoBase
	}
	return MsgInvalidInput
}
