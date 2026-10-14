package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// RunCodex plays sc as the codex app-server protocol over in/out and returns the exit code.
//
// Explicit scenario steps run through the shared Runner. Builtins answer the handshake themselves
// (initialize, thread/start or thread/resume) and then serve turn/start requests:
//
//	echo-turn         reasoning and text deltas, completed items (no duplicated text), turn/completed
//	tool-turn         command item, exec approval request, reply decision echoed as text, turn/completed
//	mcp-turn          plan, mcpToolCall and fileChange items, a foreign-thread notification, an error
//	                  notification and an unknown server request (the client must answer -32601)
//	mcp-call-turn     mcpToolCall item for $FAKEAGENT_MCP_TOOL (arguments FAKEAGENT_MCP_ARGS) with a real
//	                  MCP tools/call to the server named by the -c mcp_servers override, completed with
//	                  its result (or error), then text and turn/completed
//	hold-turn         one delta, then wait for turn/interrupt and end interrupted
//	late-ack-turn     as hold-turn but the turn/start ack is delayed 300 ms
//	cancel-ask-turn   an approval request is parked; waits for the cancel reply and turn/interrupt
//	crash-turn        writes "boom" to stderr and exits 3 on turn/start
//	no-handshake      never answers initialize
func RunCodex(sc *Scenario, in io.Reader, out io.Writer) int {
	r := NewRunner(in, out)
	if len(sc.Steps) > 0 {
		return r.RunSteps(sc.Steps)
	}
	b := sc.Builtin
	if b == "" {
		b = "echo-turn"
	}
	if b == "no-handshake" {
		io.Copy(io.Discard, in)
		return 1
	}
	f := &fakeCodex{r: r, builtin: b}
	return f.serve()
}

type fakeCodex struct {
	r       *Runner
	builtin string
	thread  string
	turns   int
}

func (f *fakeCodex) result(id any, res any) {
	f.r.WriteFrame(map[string]any{"id": id, "result": res})
}

func (f *fakeCodex) notify(method string, params map[string]any) {
	if _, ok := params["threadId"]; !ok {
		params["threadId"] = f.thread
	}
	f.r.WriteFrame(map[string]any{"method": method, "params": params})
}

func (f *fakeCodex) serve() int {
	f.thread = "t-1"
	for {
		_, m, err := f.r.ReadFrame()
		if err != nil {
			return 0
		}
		method, _ := m["method"].(string)
		switch method {
		case "initialize":
			f.result(m["id"], map[string]any{"userAgent": "fakecodex/0"})
		case "thread/start":
			f.result(m["id"], map[string]any{"thread": map[string]any{"id": f.thread}, "model": "fake-model"})
		case "thread/resume":
			if p, ok := m["params"].(map[string]any); ok {
				if id, ok := p["threadId"].(string); ok {
					f.thread = id
				}
			}
			f.result(m["id"], map[string]any{"thread": map[string]any{"id": f.thread}, "model": "fake-model"})
		case "model/list":
			if os.Getenv("FAKEAGENT_MODELS") == "" {
				f.r.WriteFrame(map[string]any{"id": m["id"], "error": map[string]any{"code": -32601, "message": "method not found"}})
				continue
			}
			f.result(m["id"], map[string]any{"data": []any{
				map[string]any{"id": "fake-model", "model": "fake-model", "displayName": "Fake Model", "isDefault": true},
				map[string]any{"id": "fake-mini", "model": "fake-mini", "displayName": "Fake Mini"}}, "nextCursor": nil})
		case "turn/start":
			if code, done := f.turn(m); done {
				return code
			}
		case "turn/interrupt":
			f.r.WriteFrame(map[string]any{"id": m["id"], "error": map[string]any{"code": -32000, "message": "no active turn to interrupt"}})
		}
	}
}

func (f *fakeCodex) ack(m map[string]any) string {
	f.turns++
	id := fmt.Sprintf("turn-%d", f.turns)
	f.result(m["id"], map[string]any{"turn": map[string]any{"id": id}})
	return id
}

func (f *fakeCodex) completed(turn, status string) {
	f.notify("turn/completed", map[string]any{"turn": map[string]any{"id": turn, "status": status}})
}

// waitInterrupt reads until turn/interrupt, replies {} and ends the turn interrupted.
func (f *fakeCodex) waitInterrupt(turn string) (int, bool) {
	for {
		_, m, err := f.r.ReadFrame()
		if err != nil {
			return 1, true
		}
		if m["method"] == "turn/interrupt" {
			f.result(m["id"], map[string]any{})
			f.completed(turn, "interrupted")
			return 0, false
		}
	}
}

func (f *fakeCodex) turn(m map[string]any) (int, bool) {
	if f.builtin == "late-ack-turn" {
		time.Sleep(300 * time.Millisecond)
	}
	if f.builtin == "crash-turn" {
		fmt.Fprint(os.Stderr, "boom")
		return 3, true
	}
	id := f.ack(m)
	switch f.builtin {
	case "tool-turn":
		f.notify("item/started", map[string]any{"turnId": id, "item": map[string]any{
			"id": "c1", "type": "commandExecution", "command": "ls -la", "cwd": "/work", "status": "inProgress"}})
		f.r.WriteFrame(map[string]any{"id": 100, "method": "item/commandExecution/requestApproval", "params": map[string]any{
			"threadId": f.thread, "turnId": id, "itemId": "c1", "command": "ls -la", "cwd": "/work"}})
		reply, code, done := f.waitReply(100)
		if done {
			return code, true
		}
		res, _ := reply["result"].(map[string]any)
		dec := fmt.Sprint(res["decision"])
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": "decision=" + dec})
		exit, status := 0, "completed"
		if dec != "accept" && dec != "acceptForSession" {
			exit, status = 1, "declined"
		}
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{
			"id": "c1", "type": "commandExecution", "command": "ls -la", "status": status,
			"aggregatedOutput": "total 0\n", "exitCode": exit}})
		f.completed(id, "completed")
	case "mcp-turn":
		f.notify("turn/plan/updated", map[string]any{"turnId": id, "plan": []any{
			map[string]any{"step": "draw", "status": "inProgress"}, map[string]any{"step": "save", "status": "pending"}}})
		f.notify("item/agentMessage/delta", map[string]any{"threadId": "sub-1", "itemId": "x", "delta": "SUBAGENT"})
		f.notify("error", map[string]any{"error": map[string]any{"message": "transient"}, "willRetry": true})
		f.notify("item/started", map[string]any{"turnId": id, "item": map[string]any{
			"id": "mc1", "type": "mcpToolCall", "server": "cad", "tool": "add_line",
			"arguments": map[string]any{"x1": 0, "y1": 0}, "status": "inProgress"}})
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{
			"id": "mc1", "type": "mcpToolCall", "server": "cad", "tool": "add_line", "status": "completed",
			"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok line 1"}}}}})
		f.notify("item/started", map[string]any{"turnId": id, "item": map[string]any{
			"id": "fc1", "type": "fileChange", "status": "inProgress",
			"changes": []any{map[string]any{"path": "a.txt", "diff": "+hi"}}}})
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{
			"id": "fc1", "type": "fileChange", "status": "failed",
			"changes": []any{map[string]any{"path": "a.txt", "diff": "+hi"}}}})
		f.r.WriteFrame(map[string]any{"id": 900, "method": "item/tool/call", "params": map[string]any{"threadId": f.thread}})
		if _, code, done := f.waitReply(900); done {
			return code, true
		}
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{
			"id": "m2", "type": "agentMessage", "text": "done"}})
		f.completed(id, "failed")
	case "mcp-call-turn":
		tool, args := os.Getenv("FAKEAGENT_MCP_TOOL"), os.Getenv("FAKEAGENT_MCP_ARGS")
		if args == "" {
			args = "{}"
		}
		f.notify("item/started", map[string]any{"turnId": id, "item": map[string]any{
			"id": "mc1", "type": "mcpToolCall", "server": "cad", "tool": tool,
			"arguments": json.RawMessage(args), "status": "inProgress"}})
		res, err := callMCPTool(firstTarget(codexTargets(os.Args)), tool, args)
		item := map[string]any{"id": "mc1", "type": "mcpToolCall", "server": "cad", "tool": tool, "status": "completed",
			"arguments": json.RawMessage(args)}
		if err != nil {
			item["status"] = "failed"
			item["error"] = map[string]any{"message": err.Error()}
		} else {
			item["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": res}}}
		}
		f.notify("item/completed", map[string]any{"turnId": id, "item": item})
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": "done"})
		f.completed(id, "completed")
	case "hold-turn", "late-ack-turn":
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": "working"})
		return f.waitInterrupt(id)
	case "cancel-ask-turn":
		f.r.WriteFrame(map[string]any{"id": 100, "method": "item/commandExecution/requestApproval", "params": map[string]any{
			"threadId": f.thread, "turnId": id, "itemId": "c1", "command": "rm -rf /"}})
		reply, code, done := f.waitReply(100)
		if done {
			return code, true
		}
		res, _ := reply["result"].(map[string]any)
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": "decision=" + fmt.Sprint(res["decision"])})
		return f.waitInterrupt(id)
	default:
		f.notify("item/reasoning/summaryTextDelta", map[string]any{"turnId": id, "itemId": "r1", "delta": "thinking"})
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": "Hello"})
		f.notify("item/agentMessage/delta", map[string]any{"turnId": id, "itemId": "m1", "delta": " world"})
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{"id": "m1", "type": "agentMessage", "text": "Hello world"}})
		f.notify("item/completed", map[string]any{"turnId": id, "item": map[string]any{
			"id": "r2", "type": "reasoning", "summary": []any{"plan it"}}})
		f.completed(id, "completed")
	}
	return 0, false
}

// waitReply reads frames until the response to the server request id arrives.
func (f *fakeCodex) waitReply(id int) (map[string]any, int, bool) {
	for {
		_, m, err := f.r.ReadFrame()
		if err != nil {
			return nil, 1, true
		}
		if _, hasMethod := m["method"]; !hasMethod && fmt.Sprint(m["id"]) == fmt.Sprint(id) {
			return m, 0, false
		}
	}
}
