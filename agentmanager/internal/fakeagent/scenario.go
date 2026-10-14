package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Scenario is a scripted fake-agent behaviour loaded from FAKEAGENT_SCENARIO or --scenario.
type Scenario struct {
	Builtin string `json:"builtin,omitempty"` // "echo-turn", "tool-turn" or "" for explicit Steps
	Steps   []Step `json:"steps,omitempty"`
}

// Step is one scenario action: expect, send, wait-for-reply, sleep, stderr, hang or exit.
//
// Ops (executed by Runner.RunSteps):
//
//	expect          read frames until one contains Match (recursive subset; "*" matches any present
//	                value); if Var is set, the matched frame's "id" is stored under that name
//	send            write Frame; any string value equal to "$name" is replaced by variable name
//	wait-for-reply  read frames until a response (no "method") whose id equals variable Var
//	sleep           pause MS milliseconds
//	stderr          write Frame["text"] to stderr
//	hang            ignore stdin EOF and sleep forever (exercises kill-on-close)
//	exit            stop with exit code Code
type Step struct {
	Op    string         `json:"op"`
	Match map[string]any `json:"match,omitempty"`
	Frame map[string]any `json:"frame,omitempty"`
	Var   string         `json:"var,omitempty"`
	MS    int            `json:"ms,omitempty"`
	Code  int            `json:"code,omitempty"`
}

// LoadScenario reads a scenario file; "" yields the empty scenario.
func LoadScenario(path string) (*Scenario, error) {
	sc := &Scenario{}
	if path == "" {
		return sc, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scenario: %w", err)
	}
	if err := json.Unmarshal(b, sc); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", path, err)
	}
	for i, s := range sc.Steps {
		switch s.Op {
		case "expect", "send", "wait-for-reply", "sleep", "stderr", "hang", "exit":
		default:
			return nil, fmt.Errorf("scenario %s: step %d: unknown op %q", path, i, s.Op)
		}
	}
	return sc, nil
}
