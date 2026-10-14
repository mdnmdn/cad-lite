package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements error.
func (e *Error) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

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

// ErrLineTooLong is Conn.Err when an inbound line exceeded the cap (WithMaxLine).
var ErrLineTooLong = errors.New("jsonrpc: inbound line too long")
