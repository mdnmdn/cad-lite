package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// RunClaude plays sc as the claude stream-json protocol over in/out and returns the exit code.
//
// Builtins (Scenario.Builtin); each serves prompts until stdin closes, emitting system/init before
// the first answer (session id fake-session-1, or the --resume argument):
//
//	echo-turn            text "echo: <prompt>", result success; each image block of the user line
//	                     adds " [image <mime> <n> bytes]" to <prompt>
//	partial-turn         stream_event deltas (thinking, text) followed by the complete assistant line
//	tool-turn            tool_use of $FAKEAGENT_CLAUDE_TOOL (default mcp__cad__add_line), a can_use_tool
//	                     control_request, then tool_result (error when denied), text, result. With
//	                     FAKEAGENT_MCP_TOOL=<tool> (arguments FAKEAGENT_MCP_ARGS) the tool is
//	                     mcp__cad__<tool> and, once allowed, a real MCP tools/call goes to the server
//	                     named in the --mcp-config file; its text is the tool_result.
//	interrupt-turn       first prompt streams a delta and waits for an interrupt control_request, then
//	                     answers it and ends with error_during_execution; later prompts echo
//	permission-interrupt as tool-turn, but waits for an interrupt after the permission answer
//
// Between turns it answers the control requests set_model (an error for the model "bad-model")
// and, when FAKEAGENT_MODELS is set, initialize with two models; without it initialize goes
// unanswered, like an older CLI. system/init reports the --model argument or the last set_model.
//
// With no builtin the explicit steps run through Runner.RunSteps.
func RunClaude(sc *Scenario, in io.Reader, out io.Writer) int {
	r := NewRunner(in, out)
	var turn func(c *claudeFake, n int, prompt string) int
	switch sc.Builtin {
	case "":
		return r.RunSteps(sc.Steps)
	case "echo-turn":
		turn = (*claudeFake).echoTurn
	case "partial-turn":
		turn = (*claudeFake).partialTurn
	case "tool-turn":
		turn = func(c *claudeFake, n int, p string) int { return c.toolTurn(p, false) }
	case "permission-interrupt":
		turn = func(c *claudeFake, n int, p string) int {
			if n > 1 {
				return c.echoTurn(n, p)
			}
			return c.toolTurn(p, true)
		}
	case "interrupt-turn":
		turn = func(c *claudeFake, n int, p string) int {
			if n > 1 {
				return c.echoTurn(n, p)
			}
			return c.interruptTurn()
		}
	default:
		fmt.Fprintf(os.Stderr, "fakeagent: unknown claude builtin %q\n", sc.Builtin)
		return 2
	}
	c := &claudeFake{r: r, session: "fake-session-1", model: "fake-model"}
	for i, a := range os.Args {
		if a == "--resume" && i+1 < len(os.Args) {
			c.session = os.Args[i+1]
		}
		if a == "--model" && i+1 < len(os.Args) {
			c.model = os.Args[i+1]
		}
	}
	n := 0
	for {
		_, f, err := r.ReadFrame()
		if err != nil {
			return 0
		}
		if f["type"] == "control_request" {
			c.control(f)
			continue
		}
		if f["type"] != "user" {
			continue
		}
		n++
		if !c.inited {
			c.inited = true
			c.send(map[string]any{"type": "system", "subtype": "init", "session_id": c.session, "model": c.model, "tools": []any{}})
		}
		if code := turn(c, n, promptText(f)); code != 0 {
			return code
		}
	}
}

type claudeFake struct {
	r       *Runner
	session string
	model   string
	inited  bool
	seq     int
}

// control answers the control requests the fake knows between turns.
func (c *claudeFake) control(f map[string]any) {
	req, _ := f["request"].(map[string]any)
	ok := func(resp map[string]any) {
		r := map[string]any{"subtype": "success", "request_id": f["request_id"]}
		if resp != nil {
			r["response"] = resp
		}
		c.send(map[string]any{"type": "control_response", "response": r})
	}
	switch req["subtype"] {
	case "initialize":
		if os.Getenv("FAKEAGENT_MODELS") == "" {
			return
		}
		ok(map[string]any{"commands": []any{}, "models": []any{
			map[string]any{"value": "default", "displayName": "Default (recommended)"},
			map[string]any{"value": "fake-fast", "displayName": "Fake Fast"},
		}})
	case "set_model":
		m, _ := req["model"].(string)
		if m == "bad-model" {
			c.send(map[string]any{"type": "control_response", "response": map[string]any{
				"subtype": "error", "request_id": f["request_id"], "error": "Model 'bad-model' not found"}})
			return
		}
		c.model = m
		ok(nil)
	}
}

func (c *claudeFake) send(v map[string]any) { _ = c.r.WriteFrame(v) }

// promptText is the first text block of a user line followed by one imageNote per base64 image block.
func promptText(f map[string]any) string {
	msg, _ := f["message"].(map[string]any)
	parts, _ := msg["content"].([]any)
	text, notes := "", ""
	for _, p := range parts {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "text":
			if text == "" {
				text, _ = m["text"].(string)
			}
		case "image":
			src, _ := m["source"].(map[string]any)
			mime, _ := src["media_type"].(string)
			data, _ := src["data"].(string)
			notes += imageNote(mime, data)
		}
	}
	return text + notes
}

func (c *claudeFake) msgID() string {
	c.seq++
	return fmt.Sprintf("msg_%d", c.seq)
}

func assistant(id string, blocks ...any) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{"id": id, "role": "assistant", "content": blocks}}
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func (c *claudeFake) result(subtype string, isErr bool, res string) {
	c.send(map[string]any{
		"type": "result", "subtype": subtype, "is_error": isErr, "result": res,
		"session_id": c.session, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
}

func (c *claudeFake) echoTurn(_ int, prompt string) int {
	c.send(assistant(c.msgID(), text("echo: "+prompt)))
	c.result("success", false, "echo: "+prompt)
	return 0
}

func (c *claudeFake) stream(ev map[string]any) {
	c.send(map[string]any{"type": "stream_event", "event": ev})
}

func delta(kind, key, s string) map[string]any {
	return map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": kind, key: s}}
}

func (c *claudeFake) partialTurn(_ int, prompt string) int {
	id := c.msgID()
	c.stream(map[string]any{"type": "message_start", "message": map[string]any{"id": id, "role": "assistant"}})
	c.stream(delta("thinking_delta", "thinking", "hmm "))
	c.stream(delta("text_delta", "text", "Hel"))
	c.stream(delta("text_delta", "text", "lo "+prompt))
	c.stream(delta("input_json_delta", "partial_json", `{"a"`))
	c.send(assistant(id,
		map[string]any{"type": "thinking", "thinking": "hmm ", "signature": "sig"},
		text("Hello "+prompt)))
	c.stream(map[string]any{"type": "message_stop"})
	c.result("success", false, "Hello "+prompt)
	return 0
}

func (c *claudeFake) toolTurn(prompt string, waitInterrupt bool) int {
	tool := os.Getenv("FAKEAGENT_CLAUDE_TOOL")
	mcpTool := os.Getenv("FAKEAGENT_MCP_TOOL")
	if tool == "" {
		tool = "mcp__cad__add_line"
		if mcpTool != "" {
			tool = "mcp__cad__" + mcpTool
		}
	}
	input := map[string]any{"x1": 0, "y1": 0, "x2": 10, "y2": 10, "command": "ls"}
	if a := os.Getenv("FAKEAGENT_MCP_ARGS"); mcpTool != "" && a != "" {
		input = nil
		_ = json.Unmarshal([]byte(a), &input)
	}
	c.send(assistant(c.msgID(), text("calling tool"),
		map[string]any{"type": "tool_use", "id": "toolu_1", "name": tool, "input": input}))
	c.send(map[string]any{
		"type": "control_request", "request_id": "req-1",
		"request": map[string]any{
			"subtype": "can_use_tool", "tool_name": tool, "input": input, "tool_use_id": "toolu_1",
			"permission_suggestions": []any{map[string]any{"type": "setMode", "mode": "acceptEdits", "destination": "session"}},
		},
	})
	var behavior string
	for {
		_, f, err := c.r.ReadFrame()
		if err != nil {
			return 1
		}
		if f["type"] == "control_response" {
			resp, _ := f["response"].(map[string]any)
			inner, _ := resp["response"].(map[string]any)
			behavior, _ = inner["behavior"].(string)
			break
		}
	}
	if waitInterrupt {
		return c.waitInterrupt()
	}
	said, isErr := "tool said "+behavior, behavior != "allow"
	if mcpTool != "" && behavior == "allow" {
		args, _ := json.Marshal(input)
		res, err := callMCPTool(firstTarget(claudeTargets(os.Args)), mcpTool, string(args))
		if err != nil {
			res, isErr = "mcp error: "+err.Error(), true
		}
		said = res
	}
	c.send(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "is_error": isErr,
			"content": []any{text(said)}},
	}}})
	c.send(assistant(c.msgID(), text("done: "+behavior)))
	c.result("success", false, "done: "+behavior)
	return 0
}

func (c *claudeFake) interruptTurn() int {
	id := c.msgID()
	c.stream(map[string]any{"type": "message_start", "message": map[string]any{"id": id}})
	c.stream(delta("text_delta", "text", "working"))
	return c.waitInterrupt()
}

func (c *claudeFake) waitInterrupt() int {
	for {
		_, f, err := c.r.ReadFrame()
		if err != nil {
			return 1
		}
		req, _ := f["request"].(map[string]any)
		if f["type"] == "control_request" && req["subtype"] == "interrupt" {
			c.send(map[string]any{"type": "control_response", "response": map[string]any{
				"subtype": "success", "request_id": f["request_id"],
			}})
			c.result("error_during_execution", true, "interrupted")
			return 0
		}
	}
}
