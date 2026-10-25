// Package props is the property model behind the Properties panel and structured agent edits:
// it builds a sheet of typed, categorised properties for a target (the selection, a layer, or the
// drawing's defaults), parses typed values in the drawing's units, and applies an edit as one
// undoable transaction. It is UI-free: it imports cmdsys, doc, geom and i18n, never gpui.
//
// Inheritance follows doc.Drawing.Resolve: entity, then the enclosing block (ByBlock), then the
// layer (ByLayer), then the global defaults; a dimension follows its DIMSTYLE record except for
// the variables it overrides. A Property says where its value comes from (Source, From) and
// whether the level can drop its own value (Clearable, applied with Value.Clear).
package props

import (
	"errors"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
)

// Host is what the model needs from the application: a cmdsys.Session satisfies it.
type Host interface {
	// Drawing returns the drawing the target lives in.
	Drawing() *doc.Drawing
	// Vars returns the system variables: units for Parse and the display, and the defaults.
	Vars() *cmdsys.Vars
	// Active returns the running command's name, or "" when the session is idle.
	Active() string
}

// TargetKind is what a sheet describes.
type TargetKind int

// The targets.
const (
	// TargetSelection is a set of entities (Target.Handles), optionally filtered by class.
	TargetSelection TargetKind = iota
	// TargetLayer is one layer record (Target.Layer).
	TargetLayer
	// TargetDefaults is the drawing's current properties and global defaults, the current
	// DIMSTYLE among them: what the panel shows when nothing is selected.
	TargetDefaults
)

// Target names what a sheet shows and an edit changes.
type Target struct {
	// Kind says what the target is: a selection, a layer or the defaults.
	Kind TargetKind
	// Handles is the selection (TargetSelection). Handles that vanished are ignored.
	Handles []doc.Handle
	// Class filters the selection to one DXF class ("LINE"); "" is every object. Geometry
	// categories appear only when the filtered objects share one class.
	Class string
	// Layer names the layer (TargetLayer).
	Layer string
	// Vertex is the LWPOLYLINE vertex the vertex properties show and edit, from 0; it is
	// clamped to the first polyline's vertices. The "vertex" property selects it.
	Vertex int
}

// Kind is the type of a property's value and the editor it wants.
type Kind int

// The property kinds. Value says which field each uses.
const (
	// KindNumber is a plain real number (Num): a scale, a factor.
	KindNumber Kind = iota
	// KindInt is an integer (Int).
	KindInt
	// KindLength is a distance or coordinate in drawing units (Num), shown and parsed in LUNITS.
	KindLength
	// KindAngle is an angle in radians (Num), shown and parsed in AUNITS.
	KindAngle
	// KindText is one line of text (Str).
	KindText
	// KindMultiText is text that may hold line breaks (Str, "\n" between lines).
	KindMultiText
	// KindBool is a switch (Bool).
	KindBool
	// KindEnum is one of Options; each option's Value is what Apply takes.
	KindEnum
	// KindColor is a colour (Int): an ACI 1–255, doc.ByLayer (256) or doc.ByBlock (0).
	KindColor
	// KindLinetype is a linetype (Str): a name, doc.LinetypeByLayer or doc.LinetypeByBlock.
	KindLinetype
	// KindLineweight is a lineweight (Int holding a doc.Lineweight: ByLayer 0, ByBlock -2,
	// Default -3, LineweightZero -4, else hundredths of a millimetre).
	KindLineweight
	// KindLayer is a layer name (Str).
	KindLayer
	// KindTextStyle is a text style name (Str).
	KindTextStyle
	// KindDimStyle is a dimension style name (Str).
	KindDimStyle
	// KindBlock is a block name (Str).
	KindBlock
	// KindArrow is a dimension arrowhead (Str): a DIMBLK value ("" closed filled, "_ArchTick", or
	// a block name).
	KindArrow
)

// Value is a property value. Which field is meaningful depends on the Kind; Varies marks a
// selection whose objects disagree (the panel shows *VARIES*).
type Value struct {
	// Varies marks a selection whose objects disagree; the other fields are then unset.
	Varies bool
	// Num holds a KindNumber, KindLength or KindAngle (radians) value.
	Num float64
	// Int holds a KindInt, KindColor or KindLineweight value, and an integer KindEnum option.
	Int int
	// Str holds the value of the name and text kinds (KindText, KindLayer, KindArrow...).
	Str string
	// Bool holds a KindBool value.
	Bool bool
	// Clear, given to Apply, drops the level's own value so the property inherits again: a
	// dimension override returns to its style's value, an entity colour, linetype or lineweight
	// becomes ByLayer, a linetype scale 1. Offered where Property.Clearable is set.
	Clear bool
}

// NumValue returns a Value holding v (Number, Length, Angle).
func NumValue(v float64) Value { return Value{Num: v} }

// IntValue returns a Value holding v (Int, Color, Lineweight).
func IntValue(v int) Value { return Value{Int: v} }

// StrValue returns a Value holding s (Text and the name kinds).
func StrValue(s string) Value { return Value{Str: s} }

// BoolValue returns a Value holding b.
func BoolValue(b bool) Value { return Value{Bool: b} }

// ClearValue returns the Value that clears a property back to what it inherits (Value.Clear).
func ClearValue() Value { return Value{Clear: true} }

// Sample is what a swatch beside a colour, linetype, lineweight or arrow shows, already resolved
// through ByLayer and ByBlock: what the object actually gets.
type Sample struct {
	// ACI is a colour 1–255 to paint; 0 means no colour swatch.
	ACI int
	// Linetype is the effective linetype name and Pattern its dashes (doc.Linetype.Pattern: a
	// positive length a dash, negative a gap, 0 a dot; nil solid). "" means no linetype swatch.
	Linetype string
	// Pattern is the dashes of Linetype.
	Pattern []float64
	// Lineweight is the effective width: hundredths of a millimetre, or doc.LineweightZero for
	// 0.00 mm (doc.Lineweight.Hundredths). 0 means no lineweight swatch.
	Lineweight doc.Lineweight
	// Arrow is a DIMBLK value to draw with doc.LayoutArrow when HasArrow is set.
	Arrow string
	// HasArrow says the sample is an arrowhead (Arrow "" is closed filled, not "none").
	HasArrow bool
}

// Option is one choice of an Enum, Color, Linetype, Lineweight, Layer, TextStyle, DimStyle,
// Block or Arrow property. ByLayer and ByBlock are options like any other, with their resolved
// Sample.
type Option struct {
	// Value is what Apply takes for this choice.
	Value Value
	// Label is the display text, in the current language for the words (ByLayer, Default) and
	// the name for a table entry.
	Label string
	// Sample is the swatch to draw beside the label.
	Sample Sample
	// Load marks a linetype of the built-in library the drawing has not loaded yet: choosing it
	// loads it in the same undo step.
	Load bool
}

// Source is where a property's value comes from.
type Source int

// The sources of a value.
const (
	// SourceOwn is a value the object (or record) sets itself.
	SourceOwn Source = iota
	// SourceLayer is ByLayer: the layer (From) supplies it.
	SourceLayer
	// SourceBlock is ByBlock: the enclosing block reference supplies it (CONTINUOUS, colour 7
	// and the default lineweight in model space).
	SourceBlock
	// SourceStyle is a dimension variable the dimension does not override: its DIMSTYLE (From)
	// supplies it.
	SourceStyle
	// SourceDefault is the global default (LTSCALE, LWDEFAULT) or a derived value.
	SourceDefault
)

// Property is one row of a sheet.
type Property struct {
	// ID is the stable identifier Apply takes ("color", "radius", "dimasz"...).
	ID string
	// Label and Tip are i18n catalog keys (Tip "" when there is none). Name, when set, is the
	// label to show as is (an attribute's tag); Label is then the generic key.
	Label, Tip string
	Name       string
	// Kind is the value's type, which picks the panel's editor and the Value field it uses.
	Kind Kind
	// ReadOnly rows are shown, not edited (derived values, the handle).
	ReadOnly bool
	// Value is the current value; Display renders it in the drawing's units and the current
	// language (*VARIES* for Value.Varies).
	Value Value
	// Display is Value as the panel shows it.
	Display string
	// Sample is the swatch of the current value (resolved: ByLayer shows the layer's colour).
	Sample Sample
	// Options are the choices of a choice kind, in display order.
	Options []Option
	// Min and Max bound a Number, Int, Length or Angle when Max > Min; Positive requires a value
	// above zero.
	Min, Max float64
	// Positive requires a value above zero.
	Positive bool
	// Var names the system variable the row edits (Defaults target, and the DIM* variable a
	// dimension row overrides); "" for none.
	Var string
	// Source and From say where the value comes from: the layer or style name for SourceLayer
	// and SourceStyle. Inherited is the ready tip ("From layer WALLS", ui.props.inherited_tip),
	// "" for SourceOwn.
	Source Source
	// From is the layer or style name the value comes from, for SourceLayer and SourceStyle.
	From string
	// Inherited is the ready tip saying where the value comes from.
	Inherited string
	// Overridden marks a dimension variable the dimension overrides (or, in a selection, that
	// some selected dimension overrides).
	Overridden bool
	// Clearable offers Value.Clear: the panel's "clear override" action.
	Clearable bool
	// NotUndoable marks a row whose edit does not enter the undo history (TEXTSIZE, DIMARCSYM).
	NotUndoable bool
	// Selector marks a row that chooses what other rows show instead of editing the drawing:
	// "vertex", whose new value goes to Target.Vertex (Value.Int, from 0).
	Selector bool
}

// Category is a titled group of rows: General, Geometry, Text, Lines & Arrows...
type Category struct {
	// ID is stable ("general", "geometry", "dim_lines"...); Title is an i18n key.
	ID, Title string
	// Properties are the rows, in display order.
	Properties []Property
}

// TypeCount is one entry of the object-type filter.
type TypeCount struct {
	// Class is the DXF class ("LINE") the filter sets in Target.Class; Label is its name in the
	// current language ("Line", or the class itself for a type the catalog does not name).
	Class, Label string
	// Count is the number of selected objects of the class.
	Count int
}

// Sheet is everything the panel shows for a target.
type Sheet struct {
	// Target is what the sheet shows.
	Target Target
	// Types is the selection's classes with their counts, for the filter ("All (n)" is Count).
	Types []TypeCount
	// Count is the number of objects the sheet shows (after the class filter).
	Count int
	// Title is the heading in the current language ("Layer: WALLS", the defaults title,
	// "3 selected").
	Title string
	// Editable is false while a command runs: the panel shows the rows read-only (busy_tip).
	Editable bool
	// Vertices is the number of vertices of the shown LWPOLYLINE (0 otherwise).
	Vertices int
	// Categories are the sheet's groups of rows, in display order.
	Categories []Category
}

// Find returns the row with the given id and whether the sheet has it.
func (s Sheet) Find(id string) (Property, bool) {
	for _, c := range s.Categories {
		for _, p := range c.Properties {
			if p.ID == id {
				return p, true
			}
		}
	}
	return Property{}, false
}

// Result reports what Apply did.
type Result struct {
	// Changed counts the objects or records the edit changed; Skipped the selected objects it
	// left alone: on a locked, off or frozen layer, unknown to the document, or vanished.
	Changed, Skipped int
	// Step reports that an undo step named PROPERTIES was recorded; an edit that changed nothing
	// records none.
	Step bool
	// NotUndoable reports a change made outside the journal (Property.NotUndoable).
	NotUndoable bool
}

// Code classifies an Error.
type Code int

// The error codes.
const (
	// Busy: a command is running or another transaction holds the drawing.
	Busy Code = iota + 1
	// ReadOnly: the property cannot be edited.
	ReadOnly
	// Invalid: the value is of the wrong kind, out of range or not one of the options; also an
	// unknown property id.
	Invalid
	// Stale: the target no longer exists (a layer renamed or deleted, no object left).
	Stale
	// Refused: the drawing refuses the edit (freezing the current layer).
	Refused
)

// Error is an error the panel shows: Key is an i18n catalog key and Args its arguments.
type Error struct {
	// Code classifies the error, for errors.Is against ErrBusy, ErrInvalid and the others.
	Code Code
	// Key is the i18n catalog key of the message.
	Key string
	// Args are the message's arguments.
	Args []any
}

// Error returns the message in English.
func (e *Error) Error() string { return i18n.For(i18n.English).T(e.Key, e.Args...) }

// Is matches errors of the same Code, so errors.Is(err, props.ErrBusy) works.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Code == e.Code
}

// Text returns the message in the current language.
func (e *Error) Text() string { return i18n.T(e.Key, e.Args...) }

// The sentinel errors, one per Code, for errors.Is.
var (
	ErrBusy     = &Error{Code: Busy, Key: "ui.props.err.busy"}
	ErrReadOnly = &Error{Code: ReadOnly, Key: "ui.props.err.read_only"}
	ErrInvalid  = &Error{Code: Invalid, Key: "ui.props.err.invalid"}
	ErrStale    = &Error{Code: Stale, Key: "ui.props.err.stale"}
	ErrRefused  = &Error{Code: Refused, Key: "ui.props.err.refused"}
)

func newError(c Code, key string, args ...any) *Error { return &Error{Code: c, Key: key, Args: args} }
