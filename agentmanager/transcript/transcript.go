// Package transcript folds agentmanager events into a flat list of chat blocks. It is pure: no
// goroutines, no I/O, not goroutine-safe.
package transcript

import "agentmanager"

// Block is one chat item; it is one of the concrete types in this package.
type Block interface{}

// UserMsg is text the user sent, with the images attached to it.
type UserMsg struct {
	Text        string
	Attachments []Attachment
}

// Attachment is an image sent with a user message: its file name ("" when it had none, such as a
// pasted image), MIME type, size in pixels and a small PNG preview the host chose (nil for none).
type Attachment struct {
	Name, MIME    string
	Width, Height int
	Thumb         []byte
}

// AgentMsg is an agent message being streamed.
type AgentMsg struct{ ID, Text string }

// Thought is agent reasoning text.
type Thought struct{ ID, Text string }

// ToolBlock is a tool call with its latest status and output.
type ToolBlock struct {
	Call   agentmanager.ToolCall
	Status agentmanager.ToolStatus
	Output string
}

// PermissionBlock is a permission ask; Answered is set by ResolvePermission.
type PermissionBlock struct {
	RequestID string
	Call      agentmanager.ToolCallPatch
	Options   []agentmanager.PermissionOption
	Answered  bool
}

// Plan is the agent's current plan.
type Plan struct{ Entries []agentmanager.PlanEntry }

// Note is a diagnostic or status line; Level is "debug", "info", "warn" or "error".
type Note struct{ Level, Text string }

// T is a transcript. The zero value is ready to use.
//
// Merging rules: a TextDelta or ThoughtDelta with a non-empty MessageID extends the latest block of
// that kind with the same id, otherwise starts a new one; with an empty id it extends the last block
// when that block has the same kind, otherwise starts a new one. A tool call is one block per call
// id: repeated starts and updates patch it in place, and an update for an unknown id creates it. A
// PlanUpdate replaces the existing Plan block in place. TurnEnd and Exited stop the running turn and
// mark still-open permission asks answered, since nobody can answer them any more.
type T struct {
	blocks  []Block
	version uint64
	running bool
}

func (t *T) bump() { t.version++ }

// AddUser appends a UserMsg and marks a turn as running.
func (t *T) AddUser(text string) { t.AddUserWith(text, nil) }

// AddUserWith appends a UserMsg with attachments and marks a turn as running.
func (t *T) AddUserWith(text string, a []Attachment) {
	t.blocks = append(t.blocks, UserMsg{Text: text, Attachments: a})
	t.running = true
	t.bump()
}

// Blocks returns the blocks in order. The slice must not be modified.
func (t *T) Blocks() []Block { return t.blocks }

// Version increases on every change, for cheap redraw checks.
func (t *T) Version() uint64 { return t.version }

// Running reports whether a turn is in progress.
func (t *T) Running() bool { return t.running }

// ResolvePermission marks the PermissionBlock with this request id as answered.
func (t *T) ResolvePermission(requestID string) {
	for i, b := range t.blocks {
		if p, ok := b.(PermissionBlock); ok && p.RequestID == requestID {
			if !p.Answered {
				p.Answered = true
				t.blocks[i] = p
				t.bump()
			}
			return
		}
	}
}

// Apply folds one event into the transcript.
func (t *T) Apply(ev agentmanager.Event) {
	switch e := ev.(type) {
	case agentmanager.TextDelta:
		if e.Text == "" {
			return
		}
		t.text(e.MessageID, e.Text, false)
	case agentmanager.ThoughtDelta:
		if e.Text == "" {
			return
		}
		t.text(e.MessageID, e.Text, true)
	case agentmanager.ToolCallStart:
		t.toolStart(e.Call)
	case agentmanager.ToolCallUpdate:
		t.toolUpdate(e.Update)
	case agentmanager.PermissionRequest:
		for _, b := range t.blocks {
			if p, ok := b.(PermissionBlock); ok && p.RequestID == e.RequestID {
				return
			}
		}
		t.blocks = append(t.blocks, PermissionBlock{
			RequestID: e.RequestID, Call: e.Call, Options: append([]agentmanager.PermissionOption(nil), e.Options...),
		})
		t.bump()
	case agentmanager.PlanUpdate:
		plan := Plan{Entries: append([]agentmanager.PlanEntry(nil), e.Entries...)}
		for i := len(t.blocks) - 1; i >= 0; i-- {
			if _, ok := t.blocks[i].(Plan); ok {
				t.blocks[i] = plan
				t.bump()
				return
			}
		}
		t.blocks = append(t.blocks, plan)
		t.bump()
	case agentmanager.TurnEnd:
		t.running = false
		t.closePermissions()
		switch e.Reason {
		case agentmanager.StopFailed:
			msg := e.Err
			if msg == "" {
				msg = "turn failed"
			}
			t.note("error", msg)
		case agentmanager.StopCancelled:
			t.note("info", "turn cancelled")
		case agentmanager.StopMaxTokens:
			t.note("warn", "stopped: token limit reached")
		case agentmanager.StopMaxTurns:
			t.note("warn", "stopped: turn request limit reached")
		case agentmanager.StopRefusal:
			t.note("warn", "stopped: the model refused")
		}
		t.bump()
	case agentmanager.Log:
		t.note(e.Level, e.Message)
		t.bump()
	case agentmanager.Exited:
		t.running = false
		t.closePermissions()
		if e.Err != "" {
			t.note("error", "agent exited: "+e.Err)
		} else {
			t.note("info", "agent exited")
		}
		t.bump()
	}
}

func (t *T) note(level, text string) {
	t.blocks = append(t.blocks, Note{Level: level, Text: text})
}

func (t *T) closePermissions() {
	for i, b := range t.blocks {
		if p, ok := b.(PermissionBlock); ok && !p.Answered {
			p.Answered = true
			t.blocks[i] = p
		}
	}
}

func (t *T) text(id, s string, thought bool) {
	idx := -1
	for i := len(t.blocks) - 1; i >= 0; i-- {
		var bid string
		var match bool
		if thought {
			var b Thought
			if b, match = t.blocks[i].(Thought); match {
				bid = b.ID
			}
		} else {
			var b AgentMsg
			if b, match = t.blocks[i].(AgentMsg); match {
				bid = b.ID
			}
		}
		if id == "" {
			if match {
				idx = i
			}
			break
		}
		if match && bid == id {
			idx = i
			break
		}
	}
	switch {
	case idx < 0 && thought:
		t.blocks = append(t.blocks, Thought{ID: id, Text: s})
	case idx < 0:
		t.blocks = append(t.blocks, AgentMsg{ID: id, Text: s})
	case thought:
		b := t.blocks[idx].(Thought)
		b.Text += s
		t.blocks[idx] = b
	default:
		b := t.blocks[idx].(AgentMsg)
		b.Text += s
		t.blocks[idx] = b
	}
	t.bump()
}

func (t *T) findTool(id string) int {
	if id == "" {
		return -1
	}
	for i := len(t.blocks) - 1; i >= 0; i-- {
		if b, ok := t.blocks[i].(ToolBlock); ok && b.Call.ID == id {
			return i
		}
	}
	return -1
}

func (t *T) toolStart(c agentmanager.ToolCall) {
	if c.Status == "" {
		c.Status = agentmanager.ToolPending
	}
	i := t.findTool(c.ID)
	if i < 0 {
		t.blocks = append(t.blocks, ToolBlock{Call: c, Status: c.Status})
		t.bump()
		return
	}
	b := t.blocks[i].(ToolBlock)
	old := b.Call
	if c.Title == "" {
		c.Title = old.Title
	}
	if c.Kind == "" {
		c.Kind = old.Kind
	}
	if len(c.RawInput) == 0 {
		c.RawInput = old.RawInput
	}
	if c.MCPServer == "" {
		c.MCPServer, c.MCPTool = old.MCPServer, old.MCPTool
	}
	b.Call = c
	b.Status = c.Status
	t.blocks[i] = b
	t.bump()
}

func (t *T) toolUpdate(p agentmanager.ToolCallPatch) {
	i := t.findTool(p.ID)
	var b ToolBlock
	if i < 0 {
		b = ToolBlock{Call: agentmanager.ToolCall{ID: p.ID, Status: agentmanager.ToolPending}, Status: agentmanager.ToolPending}
	} else {
		b = t.blocks[i].(ToolBlock)
	}
	old := b
	if p.Title != nil {
		b.Call.Title = *p.Title
	}
	if p.Kind != nil {
		b.Call.Kind = *p.Kind
	}
	if p.Status != nil {
		b.Status = *p.Status
		b.Call.Status = *p.Status
	}
	if p.Output != nil {
		b.Output = *p.Output
	}
	if len(p.RawInput) > 0 {
		b.Call.RawInput = p.RawInput
	}
	if i < 0 {
		t.blocks = append(t.blocks, b)
		t.bump()
		return
	}
	if b.Call.Title == old.Call.Title && b.Call.Kind == old.Call.Kind && b.Status == old.Status &&
		b.Output == old.Output && string(b.Call.RawInput) == string(old.Call.RawInput) {
		return
	}
	t.blocks[i] = b
	t.bump()
}
