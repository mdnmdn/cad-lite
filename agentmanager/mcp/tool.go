package mcp

import (
	"context"
	"encoding/json"
)

// Tool is one MCP tool. A Handler error becomes an isError result carrying err.Error().
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema object
	Handler     func(ctx context.Context, args json.RawMessage) (Result, error)
}
