package jsonrpc

import "encoding/json"

// Message is any JSON-RPC frame: request (ID+Method), notification (Method), response (ID+Result/Error).
type Message struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"` // number or string, echoed verbatim
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}
