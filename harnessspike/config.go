// Package harnessspike embeds Harness as an experimental drawing agent using
// OpenAI-compatible cloud providers, independently of the CLI backends.
package harnessspike

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Harness identifies spike conversations separately from CLI conversations.
const Harness = "harness-spike"

// Profile holds one provider's credentials and model.
type Profile struct {
	// APIKey is the bearer credential, stored only in the private config file.
	APIKey string `json:"api_key"`
	// Model is the service's bare model identifier.
	Model string `json:"model"`
}

// Config keeps independent profiles for both cloud services.
type Config struct {
	// Provider selects "ollama" or "opencode-go".
	Provider string `json:"provider"`
	// Ollama holds the Ollama Cloud profile.
	Ollama Profile `json:"ollama"`
	// OpenCode holds the OpenCode Go profile.
	OpenCode Profile `json:"opencode_go"`
}

// Default returns the initial cloud configuration.
func Default() Config {
	return Config{Provider: "ollama", Ollama: Profile{Model: "qwen3-coder:480b"}, OpenCode: Profile{Model: "glm-5.3-flash"}}
}

// Selected returns the selected profile and OpenAI-compatible base URL.
func (c Config) Selected() (Profile, string, error) {
	switch c.Provider {
	case "ollama":
		return c.Ollama, "https://ollama.com/v1", nil
	case "opencode-go":
		return c.OpenCode, "https://opencode.ai/zen/go/v1", nil
	default:
		return Profile{}, "", errors.New("choose Ollama Cloud or OpenCode Go")
	}
}

// Load reads a spike configuration; a missing file yields defaults.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Default(), errors.New("invalid Harness spike configuration")
	}
	_, _, err = c.Selected()
	if err != nil {
		return Default(), err
	}
	return c, nil
}

// Save atomically saves both profiles with owner-only file permissions.
func Save(path string, c Config) error {
	if _, _, err := c.Selected(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".harness-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func validate(c Config) (Profile, string, error) {
	p, url, err := c.Selected()
	if err != nil {
		return p, url, err
	}
	p.APIKey, p.Model = strings.TrimSpace(p.APIKey), strings.TrimSpace(p.Model)
	if p.APIKey == "" || p.Model == "" {
		return p, url, fmt.Errorf("%s: save an API key and model in the Harness spike controls", c.Provider)
	}
	return p, url, nil
}
