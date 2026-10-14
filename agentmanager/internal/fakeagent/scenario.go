package main

// Scenario is a scripted fake-agent behaviour loaded from FAKEAGENT_SCENARIO or --scenario.
type Scenario struct {
	Builtin string `json:"builtin,omitempty"` // "echo-turn", "tool-turn" or "" for explicit Steps
	Steps   []Step `json:"steps,omitempty"`
}

// Step is one scenario action: expect, send, wait-for-reply, sleep or exit.
type Step struct {
	Op    string         `json:"op"`
	Match map[string]any `json:"match,omitempty"`
	Frame map[string]any `json:"frame,omitempty"`
	Var   string         `json:"var,omitempty"`
	MS    int            `json:"ms,omitempty"`
	Code  int            `json:"code,omitempty"`
}

// LoadScenario reads a scenario file; "" yields the empty scenario.
func LoadScenario(path string) (*Scenario, error) { panic("todo: WP1") }
