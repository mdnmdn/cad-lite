package jsonrpc

import (
	"encoding/json"
	"errors"
)

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements error.
func (e *Error) Error() string { panic("todo: WP1") }

// Standard JSON-RPC error codes.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// ErrClosed is returned by calls on a Conn whose reader has exited or whose writer was closed.
var ErrClosed = errors.New("jsonrpc: connection closed")
