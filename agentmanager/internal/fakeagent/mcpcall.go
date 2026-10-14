package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// callMCPTool performs initialize, notifications/initialized and one tools/call against t over
// plain HTTP and returns the joined text content; an isError result is returned as an error.
func callMCPTool(t acpMCPTarget, tool, args string) (string, error) {
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

var codexMCPRe = regexp.MustCompile(`^mcp_servers\.[^=]+=\{url="([^"]*)", bearer_token_env_var="([^"]*)"`)

// codexTargets finds the cad server in the -c mcp_servers overrides the codex backend writes; the
// bearer token comes from the environment variable the override names.
func codexTargets(args []string) []acpMCPTarget {
	var out []acpMCPTarget
	for i, a := range args {
		if a != "-c" || i+1 >= len(args) {
			continue
		}
		if m := codexMCPRe.FindStringSubmatch(args[i+1]); m != nil {
			t := acpMCPTarget{url: m[1]}
			if tok := os.Getenv(m[2]); tok != "" {
				t.auth = "Bearer " + tok
			}
			out = append(out, t)
		}
	}
	return out
}

// claudeTargets reads the file named by --mcp-config in args.
func claudeTargets(args []string) []acpMCPTarget {
	var out []acpMCPTarget
	for i, a := range args {
		if a != "--mcp-config" || i+1 >= len(args) {
			continue
		}
		b, err := os.ReadFile(args[i+1])
		if err != nil {
			continue
		}
		var cfg struct {
			Servers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(b, &cfg) == nil {
			for _, e := range cfg.Servers {
				out = append(out, acpMCPTarget{e.URL, e.Headers["Authorization"]})
			}
		}
	}
	return out
}

// firstTarget returns the first target, or an empty one (the call then fails with a URL error).
func firstTarget(ts []acpMCPTarget) acpMCPTarget {
	if len(ts) == 0 {
		return acpMCPTarget{}
	}
	return ts[0]
}
