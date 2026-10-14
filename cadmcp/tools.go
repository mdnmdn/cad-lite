package cadmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"go-draw/cmdsys"
	"go-draw/doc"

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
	// Snapshot renders the drawing to PNG bytes (see SnapshotRequest). It is
	// called on the UI goroutine. When nil, render_png reports that
	// rendering is not available.
	Snapshot func(*cmdsys.Session, SnapshotRequest) ([]byte, error)
}

// Register adds every drawing tool to srv.
func Register(srv *mcp.Server, e Env) {
	for _, t := range Tools(e) {
		srv.Register(t)
	}
}

// Instructions returns the text for mcp.Options.Instructions.
func Instructions() string {
	return `You control a 2D CAD drawing (go-draw, AutoCAD-like). Model space is Y-up; coordinates are in drawing units (millimetres by default); angles are in degrees, counter-clockwise from +X. Entities are identified by hexadecimal handle strings such as "1A2". Every tool is one undo step. Prefer the structured tools (add_line, add_circle, ...); use run_script for anything else: AutoCAD script text where each single space or newline separates tokens and an empty token (two separators in a row, or a trailing separator) is Enter, for example "LAYER walls=4 LINE 0,0 100,0 100,50 " (the trailing space is the Enter that ends LINE; an Enter pressed while idle repeats the last command, so do not add more). Points are "x,y" or relative "@dx,dy" or polar "@dist<angle". Commands still waiting for input after a script are cancelled. Use get_drawing first to see layers and extents. Use render_png to look at the drawing (an image plus the pixel-to-model mapping) and check your work.`
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
		def(e, "list_entities", "List entities (paginated, in draw order) with handle, class, layer, bounds and geometry. Filters combine with AND.", schemaListEntities, false, listEntities),
		def(e, "get_selection", "Return the handles of the selected entities.", schemaEmpty, false, getSelection),
		def(e, "set_selection", "Replace the selection with the given handles (empty list clears it), or add to it.", schemaSetSelection, true, setSelection),
		def(e, "add_line", "Add a LINE entity on the current layer (or on layer).", schemaAddLine, true, addLine),
		def(e, "add_circle", "Add a CIRCLE entity.", schemaAddCircle, true, addCircle),
		def(e, "add_arc", "Add an ARC through three points (start, a point on the arc, end).", schemaAddArc, true, addArc),
		def(e, "add_polyline", "Add an LWPOLYLINE through the points, optionally closed.", schemaAddPolyline, true, addPolyline),
		def(e, "add_rectangle", "Add a closed rectangular polyline between two opposite corners.", schemaAddRect, true, addRect),
		def(e, "erase", "Erase entities by handle.", schemaHandles, true, erase),
		def(e, "move", "Translate entities by (dx, dy).", schemaMove, true, move),
		def(e, "rotate", "Rotate entities counter-clockwise by angle degrees about (cx, cy).", schemaRotate, true, rotate),
		def(e, "scale", "Scale entities by factor about (cx, cy).", schemaScale, true, scale),
		def(e, "set_entity_layer", "Move entities to an existing layer.", schemaEntityLayer, true, setEntityLayer),
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

	schemaListEntities = `{"type":"object","properties":{"layer":{"type":"string"},"class":{"type":"string","description":"LINE, CIRCLE, ARC or LWPOLYLINE"},"bbox":{"type":"array","items":{"type":"number"},"minItems":4,"maxItems":4,"description":"[minx,miny,maxx,maxy]: entities whose bounds intersect it"},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":200,"description":"default 50"}},"additionalProperties":false}`
	schemaSetSelection = `{"type":"object","properties":{"handles":{"type":"array","items":{"type":"string"}},"add":{"type":"boolean","description":"add to the selection instead of replacing it"}},"required":["handles"],"additionalProperties":false}`

	pointSchema = `{"type":"array","items":{"type":"number"},"minItems":2,"maxItems":2}`

	schemaAddLine     = `{"type":"object","properties":{"x1":{"type":"number"},"y1":{"type":"number"},"x2":{"type":"number"},"y2":{"type":"number"},"layer":{"type":"string"}},"required":["x1","y1","x2","y2"],"additionalProperties":false}`
	schemaAddCircle   = `{"type":"object","properties":{"cx":{"type":"number"},"cy":{"type":"number"},"r":{"type":"number","exclusiveMinimum":0},"layer":{"type":"string"}},"required":["cx","cy","r"],"additionalProperties":false}`
	schemaAddArc      = `{"type":"object","properties":{"p1":` + pointSchema + `,"p2":` + pointSchema + `,"p3":` + pointSchema + `,"layer":{"type":"string"}},"required":["p1","p2","p3"],"additionalProperties":false}`
	schemaAddPolyline = `{"type":"object","properties":{"points":{"type":"array","items":` + pointSchema + `,"minItems":2},"closed":{"type":"boolean"},"layer":{"type":"string"}},"required":["points"],"additionalProperties":false}`
	schemaAddRect     = `{"type":"object","properties":{"x1":{"type":"number"},"y1":{"type":"number"},"x2":{"type":"number"},"y2":{"type":"number"},"layer":{"type":"string"}},"required":["x1","y1","x2","y2"],"additionalProperties":false}`

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
		h, err := doc.ParseHandle(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(str), "0x"), "0X"))
		if err != nil || h == 0 {
			return nil, fmt.Errorf("invalid handle %q (want hexadecimal)", str)
		}
		if s.Drawing().Entity(h) == nil {
			return nil, fmt.Errorf("no entity with handle %s", h)
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out, nil
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

func finite(vs ...float64) error {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("numbers must be finite")
		}
	}
	return nil
}
