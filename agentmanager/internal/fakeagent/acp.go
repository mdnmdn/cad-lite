package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RunACP plays sc as the acp protocol over in/out and returns the exit code.
//
// With no builtin it executes sc.Steps. Builtins are "echo-turn" and "tool-turn", optionally
// followed by comma-separated options, e.g. "echo-turn,nohttp":
//
//	echo-turn   every prompt is dispatched on its text: "tool..." runs the tool flow, "hang..." waits
//	            for session/cancel, "crash" exits 3 after writing "boom" to stderr, "fail" answers a
//	            JSON-RPC error, "big" streams a 1 MiB chunk, anything else is echoed back
//	tool-turn   every prompt runs the tool flow: tool_call, session/request_permission, then (when
//	            the permission is granted) an optional MCP tools/call over HTTP, tool_call_update
//	            and end_turn. The MCP call is made when FAKEAGENT_MCP_TOOL names a tool (arguments
//	            from FAKEAGENT_MCP_ARGS, default {}); the server comes from session/new mcpServers
//	            or, failing that, OPENCODE_CONFIG_CONTENT. The result text becomes the update output.
//
// Options: nohttp (no MCP capability), badversion (protocolVersion 2), authdefault (advertise
// _meta.defaultAuthMethodId and require authenticate), authreq (session/new fails -32000),
// noload (loadSession false), probe (send an unknown request terminal/create during the first turn).
func RunACP(sc *Scenario, in io.Reader, out io.Writer) int {
	r := NewRunner(in, out)
	if sc.Builtin == "" {
		return r.RunSteps(sc.Steps)
	}
	parts := strings.Split(sc.Builtin, ",")
	a := &acpFake{r: r, kind: parts[0], opts: map[string]bool{}, sid: "sess_fake"}
	for _, o := range parts[1:] {
		a.opts[o] = true
	}
	return a.run()
}

type acpFake struct {
	r      *Runner
	kind   string
	opts   map[string]bool
	sid    string
	mcp    []acpMCPTarget
	probed bool
}

type acpMCPTarget struct{ url, auth string }

func (a *acpFake) reply(id, result any) {
	a.r.WriteFrame(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (a *acpFake) replyErr(id any, code int, msg string) {
	a.r.WriteFrame(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func (a *acpFake) update(u map[string]any) {
	a.r.WriteFrame(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": a.sid, "update": u}})
}

func (a *acpFake) chunk(kind, text string) {
	a.update(map[string]any{"sessionUpdate": kind, "content": map[string]any{"type": "text", "text": text}})
}

func (a *acpFake) run() int {
	for {
		_, f, err := a.r.ReadFrame()
		if err != nil {
			return 0
		}
		if f == nil {
			continue
		}
		method, _ := f["method"].(string)
		id, hasID := f["id"]
		params, _ := f["params"].(map[string]any)
		switch method {
		case "initialize":
			ver := 1
			if a.opts["badversion"] {
				ver = 2
			}
			res := map[string]any{"protocolVersion": ver,
				"agentCapabilities": map[string]any{
					"loadSession":        !a.opts["noload"],
					"promptCapabilities": map[string]any{"image": true, "audio": false, "embeddedContext": true},
					"mcpCapabilities":    map[string]any{"http": !a.opts["nohttp"], "sse": !a.opts["nohttp"]},
				},
				"authMethods": []any{map[string]any{"id": "default", "name": "Login"}}}
			if a.opts["authdefault"] {
				res["_meta"] = map[string]any{"defaultAuthMethodId": "default"}
			}
			a.reply(id, res)
		case "authenticate":
			a.reply(id, map[string]any{})
		case "session/new", "session/load":
			if a.opts["authreq"] {
				a.replyErr(id, -32000, "auth required")
				continue
			}
			a.mcp = acpMCPTargets(params["mcpServers"])
			res := map[string]any{}
			if method == "session/new" {
				res["sessionId"] = a.sid
				res["configOptions"] = []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "fake-1"}}
			} else {
				a.sid, _ = params["sessionId"].(string)
				a.update(map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]any{"type": "text", "text": "old question"}})
				a.chunk("agent_message_chunk", "old answer")
			}
			a.reply(id, res)
		case "session/set_config_option", "session/set_model":
			a.reply(id, map[string]any{})
		case "session/prompt":
			if code := a.turn(id, params); code >= 0 {
				return code
			}
		default:
			if method != "" && hasID {
				a.replyErr(id, -32601, "method not found")
			}
		}
	}
}

func acpPromptText(params map[string]any) string {
	list, _ := params["prompt"].([]any)
	var b strings.Builder
	for _, it := range list {
		if m, ok := it.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// turn runs one prompt; a result >= 0 is an exit code.
func (a *acpFake) turn(id any, params map[string]any) int {
	text := acpPromptText(params)
	if a.opts["probe"] && !a.probed {
		a.probed = true
		a.r.WriteFrame(map[string]any{"jsonrpc": "2.0", "id": "probe1", "method": "terminal/create", "params": map[string]any{}})
	}
	end := func(reason string) { a.reply(id, map[string]any{"stopReason": reason}) }
	switch {
	case a.kind == "tool-turn" || strings.HasPrefix(text, "tool"):
		a.toolFlow(id, end)
	case strings.HasPrefix(text, "hang"):
		a.chunk("agent_message_chunk", "working")
		if !a.waitCancel(nil) {
			return 0
		}
		a.chunk("agent_message_chunk", " stopped")
		end("cancelled")
	case text == "crash":
		a.chunk("agent_message_chunk", "about to crash")
		fmt.Fprint(os.Stderr, "boom")
		return 3
	case text == "fail":
		a.replyErr(id, -32603, "model exploded")
	case text == "big":
		a.chunk("agent_message_chunk", strings.Repeat("x", 1<<20))
		end("end_turn")
	default:
		a.chunk("agent_thought_chunk", "thinking")
		a.chunk("agent_message_chunk", "echo: ")
		a.chunk("agent_message_chunk", text)
		end("end_turn")
	}
	return -1
}

// waitCancel reads frames until session/cancel (true) or, when match is non-nil, until match
// returns true for a frame (false). It reports false too when stdin closes.
func (a *acpFake) waitCancel(match func(map[string]any) bool) bool {
	for {
		_, f, err := a.r.ReadFrame()
		if err != nil {
			return false
		}
		if f == nil {
			continue
		}
		if f["method"] == "session/cancel" {
			return true
		}
		if match != nil && match(f) {
			return false
		}
	}
}

func (a *acpFake) toolFlow(id any, end func(string)) {
	title := "cad_add_line"
	a.update(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "call_1", "title": title, "kind": "other",
		"status": "pending", "rawInput": map[string]any{"x1": 0, "y1": 0, "x2": 10, "y2": 10}})
	a.r.WriteFrame(map[string]any{"jsonrpc": "2.0", "id": "p1", "method": "session/request_permission", "params": map[string]any{
		"sessionId": a.sid,
		"toolCall":  map[string]any{"toolCallId": "call_1", "title": title, "kind": "other", "status": "pending"},
		"options": []any{
			map[string]any{"optionId": "once", "name": "Allow once", "kind": "allow_once"},
			map[string]any{"optionId": "always", "name": "Always allow", "kind": "allow_always"},
			map[string]any{"optionId": "reject", "name": "Reject", "kind": "reject_once"},
		}}})
	var outcome map[string]any
	gotReply := false
	cancelled := a.waitCancel(func(f map[string]any) bool {
		if _, isReq := f["method"]; !isReq && fmt.Sprint(f["id"]) == "p1" {
			outcome, _ = f["result"].(map[string]any)["outcome"].(map[string]any)
			gotReply = true
			return true
		}
		return false
	})
	if !gotReply && !cancelled {
		return // stdin closed
	}
	if cancelled || outcome["outcome"] == "cancelled" {
		a.update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_1", "status": "failed"})
		end("cancelled")
		return
	}
	if outcome["optionId"] == "reject" {
		a.update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_1", "status": "failed"})
		a.chunk("agent_message_chunk", "rejected")
		end("end_turn")
		return
	}
	a.update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_1", "status": "in_progress"})
	done := map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "call_1", "status": "completed",
		"content": []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "ok"}}}}
	if tool := os.Getenv("FAKEAGENT_MCP_TOOL"); tool != "" {
		res, err := a.callMCP(tool, os.Getenv("FAKEAGENT_MCP_ARGS"))
		if err != nil {
			res = "mcp error: " + err.Error()
			done["status"] = "failed"
		}
		done["content"] = []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": res}}}
		done["rawOutput"] = map[string]any{"text": res}
	}
	a.update(done)
	a.chunk("agent_message_chunk", "done")
	end("end_turn")
}

func acpMCPTargets(v any) []acpMCPTarget {
	list, _ := v.([]any)
	var out []acpMCPTarget
	for _, e := range list {
		m, _ := e.(map[string]any)
		t := acpMCPTarget{}
		t.url, _ = m["url"].(string)
		hs, _ := m["headers"].([]any)
		for _, h := range hs {
			hm, _ := h.(map[string]any)
			if hm["name"] == "Authorization" {
				t.auth, _ = hm["value"].(string)
			}
		}
		if t.url != "" {
			out = append(out, t)
		}
	}
	return out
}

func (a *acpFake) callMCP(tool, args string) (string, error) {
	targets := a.mcp
	if len(targets) == 0 {
		var cfg struct {
			MCP map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcp"`
		}
		if json.Unmarshal([]byte(os.Getenv("OPENCODE_CONFIG_CONTENT")), &cfg) == nil {
			for _, e := range cfg.MCP {
				targets = append(targets, acpMCPTarget{e.URL, e.Headers["Authorization"]})
			}
		}
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("no MCP server configured")
	}
	t := targets[0]
	if args == "" {
		args = "{}"
	}
	post := func(body string) (string, error) {
		req, err := http.NewRequest("POST", t.url, bytes.NewReader([]byte(body)))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if t.auth != "" {
			req.Header.Set("Authorization", t.auth)
		}
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("http %d", resp.StatusCode)
		}
		return string(b), nil
	}
	if _, err := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fakeagent","version":"0"}}}`); err != nil {
		return "", err
	}
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": json.RawMessage(args)}})
	body, err := post(string(call))
	if err != nil {
		return "", err
	}
	var rsp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &rsp); err != nil {
		return "", err
	}
	var texts []string
	for _, c := range rsp.Result.Content {
		texts = append(texts, c.Text)
	}
	if rsp.Result.IsError {
		return "", fmt.Errorf("%s", strings.Join(texts, "\n"))
	}
	return strings.Join(texts, "\n"), nil
}
