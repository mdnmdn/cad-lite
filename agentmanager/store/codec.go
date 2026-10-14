package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"agentmanager"
	"agentmanager/transcript"
)

// Line kinds that are not agent events.
const (
	kindMeta      = "meta"
	kindUser      = "user"
	kindAgentSess = "agent_session"
	kindResolved  = "permission_resolved"
	kindModel     = "model"
	kindDrawing   = "drawing"
)

// User is a stored user message with the images sent with it (their previews, not the files).
type User struct {
	Text        string
	Attachments []transcript.Attachment `json:",omitempty"`
}

// Resolved records that a permission ask was answered.
type Resolved struct{ RequestID string }

type envelope struct {
	K string          `json:"k"`
	T time.Time       `json:"t"`
	D json.RawMessage `json:"d,omitempty"`
}

type metaLine struct {
	ID      string    `json:"id"`
	Harness string    `json:"harness"`
	Created time.Time `json:"created"`
}

type agentSessLine struct {
	ID string `json:"id"`
}

type modelLine struct {
	Model string `json:"model"`
}

type drawingLine struct {
	Drawing string `json:"drawing"`
}

func marshalLine(kind string, at time.Time, payload any) ([]byte, error) {
	d, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{K: kind, T: at.UTC(), D: d})
}

func eventKind(ev agentmanager.Event) (string, error) {
	switch ev.(type) {
	case agentmanager.SessionStarted:
		return "session_started", nil
	case agentmanager.TextDelta:
		return "text", nil
	case agentmanager.ThoughtDelta:
		return "thought", nil
	case agentmanager.ToolCallStart:
		return "tool_start", nil
	case agentmanager.ToolCallUpdate:
		return "tool_update", nil
	case agentmanager.PermissionRequest:
		return "permission", nil
	case agentmanager.PlanUpdate:
		return "plan", nil
	case agentmanager.TurnEnd:
		return "turn_end", nil
	case agentmanager.Log:
		return "log", nil
	case agentmanager.Exited:
		return "exited", nil
	case agentmanager.ModelsChanged:
		return "models", nil
	}
	return "", fmt.Errorf("store: unsupported event %T", ev)
}

// MarshalEvent encodes ev as one envelope line (without the newline).
func MarshalEvent(ev agentmanager.Event) ([]byte, error) { return marshalEvent(ev, time.Now()) }

func marshalEvent(ev agentmanager.Event, at time.Time) ([]byte, error) {
	k, err := eventKind(ev)
	if err != nil {
		return nil, err
	}
	return marshalLine(k, at, ev)
}

// UnmarshalEvent decodes a line written by MarshalEvent. It fails for lines that are not events.
func UnmarshalEvent(line []byte) (agentmanager.Event, error) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, err
	}
	return decodeEvent(env)
}

var errNotEvent = errors.New("store: not an event line")

func decodeEvent(env envelope) (agentmanager.Event, error) {
	switch env.K {
	case "session_started":
		return decode[agentmanager.SessionStarted](env.D)
	case "text":
		return decode[agentmanager.TextDelta](env.D)
	case "thought":
		return decode[agentmanager.ThoughtDelta](env.D)
	case "tool_start":
		e, err := decode[agentmanager.ToolCallStart](env.D)
		e.Call.RawInput = rawOrNil(e.Call.RawInput)
		return e, err
	case "tool_update":
		e, err := decode[agentmanager.ToolCallUpdate](env.D)
		e.Update = fixPatch(e.Update)
		return e, err
	case "permission":
		e, err := decode[agentmanager.PermissionRequest](env.D)
		e.Call = fixPatch(e.Call)
		return e, err
	case "plan":
		return decode[agentmanager.PlanUpdate](env.D)
	case "turn_end":
		return decode[agentmanager.TurnEnd](env.D)
	case "log":
		return decode[agentmanager.Log](env.D)
	case "exited":
		return decode[agentmanager.Exited](env.D)
	case "models":
		return decode[agentmanager.ModelsChanged](env.D)
	}
	return nil, errNotEvent
}

func decode[T agentmanager.Event](d json.RawMessage) (T, error) {
	var v T
	err := json.Unmarshal(d, &v)
	return v, err
}

func fixPatch(p agentmanager.ToolCallPatch) agentmanager.ToolCallPatch {
	p.RawInput = rawOrNil(p.RawInput)
	p.RawOutput = rawOrNil(p.RawOutput)
	return p
}

// rawOrNil maps the JSON null an empty RawMessage encodes to back to nil.
func rawOrNil(r json.RawMessage) json.RawMessage {
	if string(r) == "null" {
		return nil
	}
	return r
}
