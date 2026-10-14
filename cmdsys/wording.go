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

	// MsgPointOrKeyword is a bad answer at a point prompt that offers keywords.
	MsgPointOrKeyword = "input.err.point_or_keyword"
	// MsgNeedDistance is a bad answer at a distance prompt.
	MsgNeedDistance = "input.err.need_distance"
	// MsgNeedAngle is a bad answer at an angle prompt.
	MsgNeedAngle = "input.err.need_angle"
	// MsgNeedInteger is a bad answer at an integer prompt.
	MsgNeedInteger = "input.err.need_integer"
	// MsgInvalidKeyword is a bad answer at a prompt that takes only keywords.
	MsgInvalidKeyword = "input.err.invalid_keyword"
	// MsgNotTransparent is a command typed with ' at a prompt that cannot run transparently
	// (argument: the command name).
	MsgNotTransparent = "session.not_transparent"
)

// InputErrorKey returns the catalog key of the sentence the command line prints when ParseInput
// fails with err at prompt p, worded as AutoCAD words it (spec 1.4 and 1.12, verify V9):
//
//	a point prompt that offers keywords     Point or option keyword required.
//	a point prompt                          Invalid point.
//	a distance prompt (a point too)         Requires numeric distance or second point.
//	an angle prompt                         Requires numeric angle or second point.
//	an integer prompt                       Requires an integer value.
//	a keyword-only prompt                   Invalid option keyword.
//
// Anything else keeps the generic sentence. A nil p (no prompt) gets the generic ones.
func InputErrorKey(err error, p *Prompt) string {
	switch {
	case errors.Is(err, ErrInvalidPoint), errors.Is(err, ErrInvalidValue):
		if p != nil {
			switch {
			case p.Accepts&InputDistance != 0:
				return MsgNeedDistance
			case p.Accepts&InputAngle != 0:
				return MsgNeedAngle
			case p.Accepts&InputInteger != 0:
				return MsgNeedInteger
			case p.Accepts&InputPoint != 0 && p.Accepts&InputKeyword != 0 && len(p.Keywords) > 0:
				return MsgPointOrKeyword
			case p.Accepts&InputPoint != 0:
				return MsgInvalidPoint
			case p.Accepts&InputKeyword != 0:
				return MsgInvalidKeyword
			}
		}
		if errors.Is(err, ErrInvalidPoint) {
			return MsgInvalidPoint
		}
		return MsgInvalidValue
	case errors.Is(err, ErrNoBase):
		return MsgNoBase
	}
	return MsgInvalidInput
}
