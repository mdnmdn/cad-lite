package acp

import (
	"encoding/json"
	"strings"
	"testing"

	"agentmanager"
)

func TestOpenCodeMCPFallbackPreservesInheritedConfiguration(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"provider":{"ollama":{}},"model":"ollama/gpt-oss:120b"}`)
	env, err := opencodeConfigEnv([]agentmanager.MCPServer{{Name: "cad", URL: "http://localhost/mcp"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimPrefix(env[0], "OPENCODE_CONFIG_CONTENT=")), &config); err != nil {
		t.Fatal(err)
	}
	if config["provider"] == nil || config["mcp"] == nil || config["model"] == nil {
		t.Fatal("inherited config was not merged")
	}
}

func TestOpenCodeMCPFallbackConfigurationErrorsArePrivate(t *testing.T) {
	for _, content := range []string{`{"private-test-value`, `null`, `{"mcp":"private-test-value"}`} {
		_, err := opencodeConfigEnv(nil, []string{"OPENCODE_CONFIG_CONTENT=" + content})
		if err == nil {
			t.Fatal("invalid configuration accepted")
		}
		if strings.Contains(err.Error(), "private-test-value") {
			t.Fatal("private configuration entered error")
		}
	}
}
