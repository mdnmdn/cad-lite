// Package doc is the document database. It mirrors the DWG/DXF object
// model: every object has a stable 64-bit handle, objects refer to each
// other by handle, and every change goes through a transaction that the
// journal can undo. It never imports a graphics or UI package.
package doc

import (
	"fmt"
	"strconv"

	"cad-lite/geom"
)

// Handle is an object's unique, persistent identifier (DXF group code 5).
// Zero means "no object".
type Handle uint64

// String formats h as DXF does: upper-case hexadecimal.
func (h Handle) String() string { return fmt.Sprintf("%X", uint64(h)) }

// ParseHandle parses a hexadecimal DXF handle.
func ParseHandle(s string) (Handle, error) {
	v, err := strconv.ParseUint(s, 16, 64)
	return Handle(v), err
}

// Object is anything stored in the database.
type Object interface {
	// Handle returns the object's handle.
	Handle() Handle
	// OwnerHandle returns the owning object's handle (0 for none).
	OwnerHandle() Handle
	// Class returns the DXF class name, e.g. "LINE".
	Class() string
	// Clone returns a deep copy, handle included.
	Clone() Object
}

// Entity is a graphical object in a space.
type Entity interface {
	Object
	// Common returns the properties every entity carries.
	Common() *EntityCommon
	// Bounds returns the model-space bounding box.
	Bounds() geom.Box2
	// Geometry returns the curves used for snapping, picking and display.
	Geometry() []geom.Curve
	// Transform applies the similarity transform m in place.
	Transform(m geom.Mat3)
}

// ByLayer is the colour index meaning "use the layer's colour" (ACI 256).
const ByLayer = 256

// EntityCommon holds the properties shared by every entity. Its zero value is ByLayer for the
// linetype, the linetype scale (which reads as 1) and the lineweight; Txn.Add gives the zero
// fields the drawing's current properties (see Txn.Add for the colour). Drawing.Resolve gives the
// properties an entity draws with.
type EntityCommon struct {
	// Handle is the entity's handle.
	H Handle
	// Owner is the owning block record's handle.
	Owner Handle
	// Layer is the layer name.
	Layer string
	// Color is an AutoCAD Color Index: 1–255, ByLayer (256) or ByBlock (0, DXF group 62 0).
	Color int
	// Linetype is the linetype name (DXF group 6): "" or LinetypeByLayer for the layer's,
	// LinetypeByBlock, or a name in the drawing's linetype table (Drawing.Linetype).
	Linetype string
	// LinetypeScale is the entity's linetype scale (DXF group 48, CELTSCALE); 0 reads as 1
	// (LtScale).
	LinetypeScale float64
	// Lineweight is the entity's lineweight (DXF group 370); the zero value is ByLayer.
	Lineweight Lineweight
}

// Handle implements Object.
func (c *EntityCommon) Handle() Handle { return c.H }

// OwnerHandle implements Object.
func (c *EntityCommon) OwnerHandle() Handle { return c.Owner }

// Common implements Entity.
func (c *EntityCommon) Common() *EntityCommon { return c }
