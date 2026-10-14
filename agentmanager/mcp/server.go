package mcp

// Options configures a Server.
type Options struct {
	Name, Version, Instructions string
	Serialize                   *bool // serialise handler calls; nil = true
	Logger                      func(string)
}

// Endpoint is where a started Server listens; URL is http://127.0.0.1:P/mcp.
type Endpoint struct{ URL, Token string }

// Server is an in-process streamable-HTTP MCP server (loopback, bearer token).
type Server struct{ _ struct{} }

// NewServer returns a stopped Server.
func NewServer(o Options) *Server { panic("todo: WP2") }

// Register adds or replaces a tool, before or after Start; tools/list reflects the current set.
func (s *Server) Register(t Tool) { panic("todo: WP2") }

// Start binds 127.0.0.1:0, generates the token and returns the endpoint.
func (s *Server) Start() (Endpoint, error) { panic("todo: WP2") }

// Close shuts the server down with a 2 s grace and waits.
func (s *Server) Close() error { panic("todo: WP2") }
