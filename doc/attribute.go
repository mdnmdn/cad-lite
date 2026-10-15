package doc

import "cad-lite/geom"

// AttributeDefinition describes one per-instance text value in a block.
type AttributeDefinition struct {
	Text
	// Tag identifies the field; Prompt is its insertion question.
	Tag, Prompt string
	// Constant keeps the default value; Invisible suppresses inserted text.
	Constant, Invisible bool
}

// Class implements Object.
func (*AttributeDefinition) Class() string { return "ATTDEF" }

// Clone implements Object.
func (a *AttributeDefinition) Clone() Object { c := *a; c.Text = *a.Text.Clone().(*Text); return &c }

// Geometry implements Entity; attribute definitions are visible before block creation.
func (a *AttributeDefinition) Geometry() []geom.Curve {
	if a.Invisible {
		return nil
	}
	return a.Text.Geometry()
}

// Attribute is one inserted text value. Its position is in definition coordinates.
type Attribute struct {
	// Tag identifies the definition field.
	Tag string
	// Value is the instance-specific text.
	Value string
	// Text retains an imported attribute's world-space placement and formatting.
	Text *Text
	// Flags retains DXF invisibility and attribute state bits.
	Flags int
	// Extra keeps additional imported attribute fields for writing.
	Extra []Pair
}
