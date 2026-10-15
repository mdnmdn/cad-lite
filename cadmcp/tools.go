package cadmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"

	"agentmanager/mcp"
)

// Env connects the tools to the application.
type Env struct {
	// Dispatch runs tool bodies on the UI goroutine.
	Dispatch Runner
	// Session returns the current session (OPEN and NEW replace the drawing,
	// the session itself stays).
	Session func() *cmdsys.Session
	// Changed is the UI refresh hook, called on the UI goroutine after every
	// tool that may have modified the drawing, selection or view.
	Changed func()
	// Snapshot renders the drawing to an image (see SnapshotRequest). It is
	// called on the UI goroutine; the image must not be shared with later
	// renders, because render_png PNG-encodes it on the MCP handler's
	// goroutine. When nil, render_png reports that rendering is not available.
	Snapshot func(*cmdsys.Session, SnapshotRequest) (image.Image, error)
}

// Register adds every drawing tool to srv.
func Register(srv *mcp.Server, e Env) {
	for _, t := range Tools(e) {
		srv.Register(t)
	}
}

// Instructions returns the text for mcp.Options.Instructions.
func Instructions() string {
	return `You control a 2D CAD drawing (Cad-lite, AutoCAD-like). Model space is Y-up; coordinates are in drawing units (millimetres by default); angles are in degrees, counter-clockwise from +X. Entities are identified by hexadecimal handle strings such as "1A2". Every tool is one undo step. Prefer the structured tools (add_line, add_circle, ...), and batch to make many of them in one call and one undo step (refer to entities created earlier in the batch as "$id"); use run_script for anything else: AutoCAD script text where each single space or newline separates tokens and an empty token (two separators in a row, or a trailing separator) is Enter, for example "LAYER walls=4 LINE 0,0 100,0 100,50 " (the trailing space is the Enter that ends LINE; an Enter pressed while idle repeats the last command, so do not add more). Points are "x,y" or relative "@dx,dy" or polar "@dist<angle". Commands still waiting for input after a script are cancelled. Use add_text, add_mtext and update_text for text, and list_entities with "text" to find it. Use get_drawing first to see layers and extents. Use render_png to look at the drawing (an image plus the pixel-to-model mapping) and check your work.`
}

type toolFunc[A any] func(s *cmdsys.Session, a A) (any, error)

// def builds a tool whose arguments decode into A (unknown fields are
// rejected) before the body runs on the UI goroutine. mutates tools trigger
// Env.Changed afterwards.
func def[A any](e Env, name, desc, schema string, mutates bool, fn toolFunc[A]) mcp.Tool {
	required := requiredKeys(schema)
	return mcp.Tool{
		Name:        name,
		Description: desc,
		InputSchema: json.RawMessage(schema),
		Handler: func(ctx context.Context, raw json.RawMessage) (mcp.Result, error) {
			var a A
			if err := decodeArgs(raw, &a, required); err != nil {
				return errResult("invalid arguments: %v", err), nil
			}
			var out any
			var ferr error
			err := e.Dispatch.Do(ctx, func() {
				s := e.Session()
				if s == nil {
					ferr = errors.New("no drawing is open")
					return
				}
				out, ferr = call(s, a, fn)
				if mutates && e.Changed != nil {
					e.Changed()
				}
			})
			switch {
			case err != nil:
				return errResult("%v", err), nil
			case ferr != nil:
				return errResult("%v", ferr), nil
			}
			return jsonResult(out), nil
		},
	}
}

func requiredKeys(schema string) []string {
	var sc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal([]byte(schema), &sc); err != nil {
		panic("cadmcp: bad schema: " + err.Error())
	}
	return sc.Required
}

func decodeArgs(raw json.RawMessage, v any, required []string) error {
	if t := bytes.TrimSpace(raw); len(t) == 0 || string(t) == "null" {
		raw = []byte("{}")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return errors.New("arguments must be a JSON object")
	}
	for _, k := range required {
		if _, ok := keys[k]; !ok {
			return fmt.Errorf("missing required field %q", k)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the JSON object")
	}
	return nil
}

func jsonResult(v any) mcp.Result {
	b, err := json.Marshal(v)
	if err != nil {
		return errResult("encoding result: %v", err)
	}
	return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(b)}}}
}

func errResult(format string, a ...any) mcp.Result {
	return mcp.Result{IsError: true, Content: []mcp.Content{{Type: "text", Text: fmt.Sprintf(format, a...)}}}
}

// Tools returns every drawing tool bound to e.
func Tools(e Env) []mcp.Tool {
	return []mcp.Tool{
		def(e, "run_script", "Run AutoCAD script text (single spaces or newlines separate tokens; a double space is Enter). A running command is cancelled first, and one still waiting for input afterwards is cancelled too. Returns the history lines it printed, new entity handles and the prompt.", schemaRunScript, true, runScript),
		def(e, "run_command", "Send one response line to the command line: a command name when idle, otherwise an answer to the current prompt. Does not cancel anything. Returns the lines printed, the prompt and the active command.", schemaRunCommand, true, runCommand),
		def(e, "list_commands", "List the command names the script accepts.", schemaEmpty, false, listCommands),
		def(e, "get_drawing", "Summary of the drawing: layers, current layer, entity counts per class, extents, undo/redo availability, selection size, active command and prompt.", schemaEmpty, false, getDrawing),
		def(e, "list_entities", "List entities (paginated, in ascending handle/storage order) with handle, class, layer, bounds and geometry. Filters combine with AND.", schemaListEntities, false, listEntities),
		def(e, "get_selection", "Return the handles of the selected entities.", schemaEmpty, false, getSelection),
		def(e, "set_selection", "Replace the selection with the given handles (empty list clears it), or add to it.", schemaSetSelection, true, setSelection),
		def(e, "add_line", "Add a LINE entity on the current layer (or on layer).", schemaAddLine, true, addLine),
		def(e, "add_circle", "Add a CIRCLE entity.", schemaAddCircle, true, addCircle),
		def(e, "add_arc", "Add an ARC through three points (p1 start, p2 a point on the arc, p3 end), or from center, radius, start_angle and end_angle (degrees, counter-clockwise from +X; the arc runs counter-clockwise from start to end, as in AutoCAD).", schemaAddArc, true, addArc),
		def(e, "add_polyline", "Add an LWPOLYLINE through the points, optionally closed, with optional arc segments given as DXF bulges (one per point).", schemaAddPolyline, true, addPolyline),
		def(e, "add_rectangle", "Add a closed rectangular polyline (like RECTANG) from two opposite corners (x1, y1, x2, y2) or from a corner plus a signed size (origin, width along X, height along Y), with optional fillet radius or chamfer distances.", schemaAddRect, true, addRect),
		def(e, "add_polygon", "Add a regular polygon (a closed LWPOLYLINE) as AutoCAD's POLYGON draws it: sides 3-1024, then either center and radius, the radius of the circle the polygon is inscribed in (mode inscribed, the default: radius to the vertices) or circumscribed about (radius to the edge midpoints), with the bottom edge horizontal turned by rotation degrees counter-clockwise; or edge, its first edge [[x1,y1],[x2,y2]] with the polygon to the left of it.", schemaAddPolygon, true, addPolygon),
		def(e, "add_text", "Add single-line TEXT, as AutoCAD's TEXT command places it: point is the insertion point its justify refers to (default Left: the baseline start), or for Align and Fit the baseline start with second_point its end. Height is the capital height; rotation is in degrees counter-clockwise. The value keeps AutoCAD %% codes (%%d degree, %%c diameter, %%p plus-minus) and must be one line; use add_mtext for several.", schemaAddText, true, addText),
		def(e, "add_mtext", "Add multiline MTEXT placed by its attachment point (TL default, ... BR) at point, wrapping at width (0 does not wrap). Line breaks in value become paragraphs; \\, { and } show as typed unless raw is true, which passes MTEXT format codes through (\\P paragraph and the like).", schemaAddMText, true, addMText),
		def(e, "update_text", "Change a TEXT or MTEXT by handle: any of value, point (the insertion point), second_point (Align and Fit), height, rotation, justify, style and width_factor (TEXT), width and attachment (MTEXT). Fields that do not apply to the entity's class are an error. Changing justify keeps the insertion point, so the text moves around it, as TEXT's Justify does. Returns the updated entity.", schemaUpdateText, true, updateText),
		def(e, "erase", "Erase entities by handle.", schemaHandles, true, erase),
		def(e, "move", "Translate entities by (dx, dy).", schemaMove, true, move),
		def(e, "rotate", "Rotate entities counter-clockwise by angle degrees about (cx, cy).", schemaRotate, true, rotate),
		def(e, "scale", "Scale entities by factor about (cx, cy).", schemaScale, true, scale),
		def(e, "set_entity_layer", "Move entities to an existing layer.", schemaEntityLayer, true, setEntityLayer),
		def(e, "batch", "Apply many add_* and edit operations (add_line, add_circle, add_arc, add_polyline, add_rectangle, add_polygon, add_text, add_mtext, erase, move, rotate, scale, set_entity_layer, update_text) in one call and ONE undo step, like an AutoCAD UNDO BEGIN ... UNDO END group. Each op is {op, id?, args} where args are exactly the single tool's arguments. Give an add_* op an id (e.g. \"door1\") and later ops can name its entity as \"$door1\" in handles. All or nothing: every op is checked first and the batch runs in one transaction; any failure changes nothing and names the failing ops[i]. Returns handles (id -> handle), created, modified and erased. At most 500 ops and 200000 polyline points per call.", schemaBatch, true, runBatch),
		def(e, "set_layer", "Make an existing layer current.", schemaSetLayer, true, setLayer),
		def(e, "create_layer", "Create a layer (or update an existing one's color, visibility and lock); optionally make it current. Layer table edits are not undoable.", schemaCreateLayer, true, createLayer),
		def(e, "undo", "Undo the last steps (default 1).", schemaSteps, true, undo),
		def(e, "redo", "Redo undone steps (default 1).", schemaSteps, true, redo),
		def(e, "zoom_extents", "Fit the view to the drawing's extents.", schemaEmpty, true, zoomExtents),
		renderPNG(e),
		def(e, "measure", "Measure entities (length/perimeter and area) and/or the distance between two points.", schemaMeasure, false, measure),
	}
}

const (
	schemaEmpty      = `{"type":"object","properties":{},"additionalProperties":false}`
	schemaRunScript  = `{"type":"object","properties":{"script":{"type":"string","description":"AutoCAD script text, e.g. \"CIRCLE 50,50 20\""}},"required":["script"],"additionalProperties":false}`
	schemaRunCommand = `{"type":"object","properties":{"input":{"type":"string","description":"one line: a command name, or a response to the current prompt (empty string = Enter)"}},"required":["input"],"additionalProperties":false}`

	schemaListEntities = `{"type":"object","properties":{"layer":{"type":"string"},"class":{"type":"string","description":"LINE, CIRCLE, ARC, LWPOLYLINE, TEXT or MTEXT"},"text":{"type":"string","description":"only TEXT and MTEXT whose shown text contains this, case-insensitive (MTEXT format codes are ignored)"},"bbox":{"type":"array","items":{"type":"number"},"minItems":4,"maxItems":4,"description":"[minx,miny,maxx,maxy]: entities whose bounds intersect it"},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":200,"description":"default 50"}},"additionalProperties":false}`
	schemaSetSelection = `{"type":"object","properties":{"handles":{"type":"array","items":{"type":"string"}},"add":{"type":"boolean","description":"add to the selection instead of replacing it"}},"required":["handles"],"additionalProperties":false}`

	pointSchema = `{"type":"array","items":{"type":"number"},"minItems":2,"maxItems":2}`

	schemaAddLine   = `{"type":"object","properties":{"x1":{"type":"number"},"y1":{"type":"number"},"x2":{"type":"number"},"y2":{"type":"number"},"layer":{"type":"string"}},"required":["x1","y1","x2","y2"],"additionalProperties":false}`
	schemaAddCircle = `{"type":"object","properties":{"cx":{"type":"number"},"cy":{"type":"number"},"r":{"type":"number","exclusiveMinimum":0},"layer":{"type":"string"}},"required":["cx","cy","r"],"additionalProperties":false}`
	schemaAddArc    = `{"type":"object","properties":{` +
		`"p1":` + pointSchema + `,"p2":` + pointSchema + `,"p3":` + pointSchema + `,` +
		`"center":{"type":"array","items":{"type":"number"},"minItems":2,"maxItems":2,"description":"centre form: [x,y]"},` +
		`"radius":{"type":"number","exclusiveMinimum":0,"description":"centre form"},` +
		`"start_angle":{"type":"number","description":"centre form: degrees, counter-clockwise from +X"},` +
		`"end_angle":{"type":"number","description":"centre form: degrees; the arc runs counter-clockwise from start_angle to end_angle"},` +
		`"layer":{"type":"string"}},"additionalProperties":false,` +
		`"description":"either p1, p2, p3 (start, a point on the arc, end) or center, radius, start_angle, end_angle"}`
	schemaAddPolyline = `{"type":"object","properties":{"points":{"type":"array","items":` + pointSchema + `,"minItems":2},` +
		`"bulges":{"type":"array","items":{"type":"number"},"description":"optional, one per point (DXF group 42): the bulge of the segment leaving that point, tan(included angle/4), positive counter-clockwise, 0 straight; the last applies only when closed"},` +
		`"closed":{"type":"boolean"},"layer":{"type":"string"}},"required":["points"],"additionalProperties":false}`
	schemaAddRect = `{"type":"object","properties":{` +
		`"x1":{"type":"number","description":"corner form: first corner"},"y1":{"type":"number"},"x2":{"type":"number","description":"corner form: opposite corner"},"y2":{"type":"number"},` +
		`"origin":{"type":"array","items":{"type":"number"},"minItems":2,"maxItems":2,"description":"size form: [x,y] of one corner"},` +
		`"width":{"type":"number","description":"size form: signed extent along X from origin"},` +
		`"height":{"type":"number","description":"size form: signed extent along Y from origin"},` +
		`"fillet":{"type":"number","minimum":0,"description":"optional corner radius (RECTANG Fillet); rounded corners become bulged vertices"},` +
		`"chamfer":{"type":"array","items":{"type":"number","minimum":0},"minItems":2,"maxItems":2,"description":"optional [d1,d2] (RECTANG Chamfer): d1 off the edges along X, d2 off those along Y"},` +
		`"layer":{"type":"string"}},"additionalProperties":false,` +
		`"description":"either x1, y1, x2, y2 (opposite corners) or origin, width, height; optionally fillet or chamfer, not both"}`

	schemaAddPolygon = `{"type":"object","properties":{` +
		`"sides":{"type":"integer","minimum":3,"maximum":1024},` +
		`"center":` + pointSchema + `,"radius":{"type":"number","exclusiveMinimum":0},` +
		`"mode":{"type":"string","enum":["inscribed","circumscribed"],"description":"default inscribed: radius to the vertices; circumscribed: radius to the edge midpoints"},` +
		`"rotation":{"type":"number","description":"degrees counter-clockwise from a horizontal bottom edge"},` +
		`"edge":{"type":"array","items":` + pointSchema + `,"minItems":2,"maxItems":2,"description":"edge form: the first edge, polygon to its left"},` +
		`"layer":{"type":"string"}},"required":["sides"],"additionalProperties":false,` +
		`"description":"sides plus either center and radius (mode, rotation optional) or edge"}`

	justifyDesc = `Left (default), Center, Right, Align, Middle, Fit, or TL TC TR ML MC MR BL BC BR`
	attachDesc  = `TL (default), TC, TR, ML, MC, MR, BL, BC or BR`

	schemaAddText = `{"type":"object","properties":{` +
		`"value":{"type":"string","description":"one line; AutoCAD %% codes allowed"},` +
		`"point":` + pointSchema + `,"second_point":` + pointSchema + `,` +
		`"height":{"type":"number","exclusiveMinimum":0,"description":"capital height; required except for Align"},` +
		`"rotation":{"type":"number","description":"degrees, counter-clockwise; not for Align and Fit"},` +
		`"justify":{"type":"string","description":"` + justifyDesc + `"},` +
		`"style":{"type":"string","description":"an existing text style, default Standard"},` +
		`"width_factor":{"type":"number","minimum":0.01,"maximum":100,"description":"default 1; not for Fit"},` +
		`"layer":{"type":"string"}},"required":["value","point"],"additionalProperties":false}`
	schemaAddMText = `{"type":"object","properties":{` +
		`"value":{"type":"string","description":"line breaks become paragraphs"},` +
		`"point":` + pointSchema + `,` +
		`"height":{"type":"number","exclusiveMinimum":0,"description":"capital height"},` +
		`"width":{"type":"number","minimum":0,"description":"wrapping width, 0 (default) does not wrap"},` +
		`"attachment":{"type":"string","description":"` + attachDesc + `"},` +
		`"rotation":{"type":"number","description":"degrees, counter-clockwise"},` +
		`"style":{"type":"string","description":"an existing text style, default Standard"},` +
		`"raw":{"type":"boolean","description":"value holds MTEXT format codes; default false escapes \\, { and }"},` +
		`"layer":{"type":"string"}},"required":["value","point","height"],"additionalProperties":false}`
	schemaUpdateText = `{"type":"object","properties":{` +
		`"handle":{"type":"string","description":"hexadecimal handle of a TEXT or MTEXT (in a batch, also \"$id\")"},` +
		`"value":{"type":"string"},"raw":{"type":"boolean","description":"MTEXT: value holds format codes"},` +
		`"point":` + pointSchema + `,"second_point":` + pointSchema + `,` +
		`"height":{"type":"number","exclusiveMinimum":0},"rotation":{"type":"number","description":"degrees"},` +
		`"justify":{"type":"string","description":"TEXT: ` + justifyDesc + `"},` +
		`"style":{"type":"string"},"width_factor":{"type":"number","minimum":0.01,"maximum":100,"description":"TEXT"},` +
		`"width":{"type":"number","minimum":0,"description":"MTEXT"},"attachment":{"type":"string","description":"MTEXT: ` + attachDesc + `"}` +
		`},"required":["handle"],"additionalProperties":false}`

	handlesProp       = `"handles":{"type":"array","items":{"type":"string"},"minItems":1,"description":"hexadecimal entity handles"}`
	schemaHandles     = `{"type":"object","properties":{` + handlesProp + `},"required":["handles"],"additionalProperties":false}`
	schemaMove        = `{"type":"object","properties":{` + handlesProp + `,"dx":{"type":"number"},"dy":{"type":"number"}},"required":["handles","dx","dy"],"additionalProperties":false}`
	schemaRotate      = `{"type":"object","properties":{` + handlesProp + `,"cx":{"type":"number"},"cy":{"type":"number"},"angle":{"type":"number","description":"degrees, counter-clockwise"}},"required":["handles","cx","cy","angle"],"additionalProperties":false}`
	schemaScale       = `{"type":"object","properties":{` + handlesProp + `,"cx":{"type":"number"},"cy":{"type":"number"},"factor":{"type":"number","exclusiveMinimum":0}},"required":["handles","cx","cy","factor"],"additionalProperties":false}`
	schemaEntityLayer = `{"type":"object","properties":{` + handlesProp + `,"layer":{"type":"string"}},"required":["handles","layer"],"additionalProperties":false}`

	schemaSetLayer    = `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`
	schemaCreateLayer = `{"type":"object","properties":{"name":{"type":"string"},"color":{"type":"integer","minimum":1,"maximum":255,"description":"AutoCAD color index, default 7"},"off":{"type":"boolean"},"locked":{"type":"boolean"},"make_current":{"type":"boolean"}},"required":["name"],"additionalProperties":false}`
	schemaSteps       = `{"type":"object","properties":{"steps":{"type":"integer","minimum":1,"maximum":100,"description":"default 1"}},"additionalProperties":false}`
	schemaMeasure     = `{"type":"object","properties":{` + handlesProp + `,"points":{"type":"array","items":` + pointSchema + `,"minItems":2,"maxItems":2,"description":"two points to measure the distance between"}},"additionalProperties":false}`
)

// idle cancels a running command so a tool starts from a clean state, and
// returns its name.
func idle(s *cmdsys.Session) string {
	name := s.Active()
	if name != "" {
		s.Escape()
	}
	return name
}

func parseHandles(s *cmdsys.Session, in []string) ([]doc.Handle, error) {
	if len(in) == 0 {
		return nil, errors.New("handles must not be empty")
	}
	seen := map[doc.Handle]bool{}
	var out []doc.Handle
	for _, str := range in {
		h, err := existingHandle(s.Drawing(), str)
		if err != nil {
			return nil, err
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out, nil
}

// existingHandle parses one hexadecimal handle and checks that it names an entity a tool can act
// on: one that exists and is of a class the app models. A *doc.Unknown (an entity read from a file
// whose class the app does not model) is refused, because transactions silently skip it.
func existingHandle(d *doc.Drawing, str string) (doc.Handle, error) {
	h, err := doc.ParseHandle(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(str), "0x"), "0X"))
	if err != nil || h == 0 {
		return 0, fmt.Errorf("invalid handle %q (want hexadecimal)", str)
	}
	e := d.Entity(h)
	if e == nil {
		return 0, fmt.Errorf("no entity with handle %s", h)
	}
	if u, ok := e.(*doc.Unknown); ok {
		return 0, fmt.Errorf("entity %s is a %s, a class Cad-lite does not model; it can be listed but not edited, selected, measured or highlighted", h, u.Type)
	}
	return h, nil
}

// call runs a tool body as an agent: HumanOnly commands are refused, and a panic aborts any
// open transaction and becomes an error so the session stays usable.
func call[A any](s *cmdsys.Session, a A, fn toolFunc[A]) (out any, err error) {
	s.SetAgentCaller(true)
	defer s.SetAgentCaller(false)
	defer func() {
		if r := recover(); r != nil {
			s.Drawing().AbortOpen()
			s.Escape()
			out, err = nil, fmt.Errorf("tool panicked: %v", r)
		}
	}()
	return fn(s, a)
}

// MaxCoord caps the magnitude of every number that places or sizes geometry: coordinates,
// lengths, radii, offsets, angles in degrees and scale factors.
const MaxCoord = 1e12

// MaxBulge caps the magnitude of a polyline bulge (a bulge of 1e6 is a near-full circle).
const MaxBulge = 1e6

// MaxExtent caps the bounds of any entity a tool creates or changes; a result past it (possible
// with in-range inputs, e.g. a large scale far from the centre) is refused.
const MaxExtent = 1e15

// inRange checks that every value is finite and within ±MaxCoord.
func inRange(vs ...float64) error {
	if err := finite(vs...); err != nil {
		return err
	}
	for _, v := range vs {
		if math.Abs(v) > MaxCoord {
			return fmt.Errorf("numbers must be within ±%g", float64(MaxCoord))
		}
	}
	return nil
}

// checkBounds refuses an entity whose bounds are not finite or exceed ±MaxExtent. Erased handles
// are skipped.
func checkBounds(d *doc.Drawing, hs ...doc.Handle) error {
	for _, h := range hs {
		e := d.Entity(h)
		if e == nil {
			continue
		}
		b := e.Bounds()
		if b.IsEmpty() {
			continue
		}
		for _, v := range []float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
			if !(math.Abs(v) <= MaxExtent) {
				return fmt.Errorf("the resulting %s would be out of range (bounds beyond ±%g or not finite)", e.Class(), float64(MaxExtent))
			}
		}
	}
	return nil
}

func finite(vs ...float64) error {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("numbers must be finite")
		}
	}
	return nil
}
