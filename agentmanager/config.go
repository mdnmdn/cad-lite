package agentmanager

// Harness names a supported agent CLI.
type Harness string

// Supported harnesses.
const (
	OpenCode Harness = "opencode"
	Claude   Harness = "claude"
	Codex    Harness = "codex"
)

// Config is everything needed to start one agent session.
type Config struct {
	Harness      Harness
	Binary       string           // executable; "" = harness default ("opencode","claude","codex")
	Args         []string         // extra argv appended after the managed flags
	Cwd          string           // absolute project dir
	Model        string           // "" = harness default
	ResumeID     string           // backend session/thread id to resume, "" = new
	SystemPrompt string           // claude: --append-system-prompt; codex: -c developer_instructions; acp: prefixed to first prompt
	MCPServers   []MCPServer      // injected per harness (section 3 of the plan)
	Env          []string         // extra KEY=VALUE for the child
	AutoApprove  PermissionPolicy // optional; nil = every request goes to the UI
	Logger       Logger           // optional; nil = discard
}

// MCPServer is a streamable-HTTP MCP server to inject into the agent.
type MCPServer struct{ Name, URL, Token string }

// Logger receives trace output (raw frames at Debug; never log tokens).
type Logger interface {
	Debugf(format string, args ...any)
}
