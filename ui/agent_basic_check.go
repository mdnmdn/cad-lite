package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"agentmanager"
	"agentmanager/harnesses"
	"agentmanager/shellenv"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

const ollamaModelsURL = "https://ollama.com/v1/models"

type basicAIState struct {
	modelCancel, checkCancel                        context.CancelFunc
	models                                          []agentmanager.ModelInfo
	modelKey                                        string
	modelResult                                     chan basicModelsResult
	modelErr                                        string
	fetch                                           func(context.Context, string) ([]agentmanager.ModelInfo, error)
	checkResult                                     chan basicCheckResult
	checkMessage                                    string
	checkOK                                         bool
	poll                                            *gpui.Timer
	availableChecked                                bool
	availablePath, availableBinary, availableSearch string
	available                                       bool
}

type basicCheckResult struct {
	opencodeKey, ollamaKey string
	err                    error
}

type basicModelsResult struct {
	key    string
	models []agentmanager.ModelInfo
	err    error
}

// Cache filesystem searches between discovery, executable and PATH changes; scrolling only reads memory.
func (ws *Workspace) openCodeAvailable() (string, bool) {
	a := &ws.prefs().aiBasic
	bin, search := harnesses.Binary(agentmanager.OpenCode), shellenv.Effective()
	if !a.availableChecked || a.availableBinary != bin || a.availableSearch != search {
		a.availablePath, a.available = harnesses.Available(agentmanager.OpenCode)
		a.availableChecked, a.availableBinary, a.availableSearch = true, bin, search
	}
	return a.availablePath, a.available
}

func fetchOllamaModels(ctx context.Context, client *http.Client, endpoint, key string) ([]agentmanager.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Ollama model request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama models: HTTP %d", res.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&body); err != nil {
		return nil, errors.New("Ollama returned an invalid model list")
	}
	var models []agentmanager.ModelInfo
	for _, m := range body.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" || slices.ContainsFunc(models, func(m agentmanager.ModelInfo) bool { return m.ID == "ollama/"+id }) {
			continue
		}
		models = append(models, agentmanager.ModelInfo{ID: "ollama/" + id, Name: id + " (Ollama Cloud)"})
	}
	if len(models) == 0 {
		return nil, errors.New("Ollama returned no models")
	}
	return models, nil
}

func (ws *Workspace) refreshOllamaModels(w *gpui.Window, force bool) {
	a := &ws.prefs().aiBasic
	key := ws.prefs().cur.OllamaKey
	if key == "" {
		a.models, a.modelKey, a.modelErr = nil, "", ""
		return
	}
	if a.modelResult != nil || (!force && a.modelKey == key) {
		return
	}
	a.modelKey, a.modelErr = key, ""
	result := make(chan basicModelsResult, 1)
	a.modelResult = result
	fetch := a.fetch
	if fetch == nil {
		fetch = func(ctx context.Context, key string) ([]agentmanager.ModelInfo, error) {
			return fetchOllamaModels(ctx, http.DefaultClient, ollamaModelsURL, key)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	a.modelCancel = cancel
	go func() {
		defer cancel()
		models, err := fetch(ctx, key)
		result <- basicModelsResult{key, models, err}
	}()
	ws.armBasicPoll(w)
}

func (ws *Workspace) armBasicPoll(w *gpui.Window) {
	a := &ws.prefs().aiBasic
	a.poll.Cancel()
	a.poll = w.After(discoveryPoll, func(w *gpui.Window, _ *gpui.App) { ws.pollBasicAI(w) })
}

func (ws *Workspace) pollBasicAI(w *gpui.Window) {
	a := &ws.prefs().aiBasic
	select {
	case r := <-a.modelResult:
		a.modelResult = nil
		if r.key == ws.prefs().cur.OllamaKey {
			if r.err != nil {
				a.modelErr = r.err.Error()
			} else {
				a.models, a.modelErr = r.models, ""
			}
		}
	default:
	}
	select {
	case result := <-a.checkResult:
		a.checkResult = nil
		err := result.err
		if result.opencodeKey != ws.prefs().cur.OpenCodeKey || result.ollamaKey != ws.prefs().cur.OllamaKey {
			a.checkMessage = ""
			break
		}
		a.checkOK = err == nil
		a.checkMessage = i18n.T("ui.settings.ai_test_ok")
		if err != nil {
			a.checkMessage = i18n.T("ui.settings.ai_test_failed", err)
		}
	default:
	}
	if a.modelResult != nil || a.checkResult != nil {
		ws.armBasicPoll(w)
	}
	w.Refresh()
}

func (ws *Workspace) basicOllamaDefault() string {
	models := ws.prefs().aiBasic.models
	if len(models) == 0 || slices.ContainsFunc(models, func(m agentmanager.ModelInfo) bool { return m.ID == basicOllamaModel }) {
		return basicOllamaModel
	}
	return models[0].ID
}

func (ws *Workspace) saveBasicKeys(w *gpui.Window) {
	fields := ws.prefs().aiKeys
	if fields == nil {
		return
	}
	ws.change(w, func(s *settings.Settings) {
		if key := strings.TrimSpace(fields.opencode.Text()); key != "" {
			s.OpenCodeKey = key
		}
		if key := strings.TrimSpace(fields.ollama.Text()); key != "" {
			s.OllamaKey = key
		}
	})
	ws.SaveSettings()
	fields.opencode.SetText("")
	fields.ollama.SetText("")
	ws.prefs().aiBasic.checkMessage = ""
	ws.refreshOllamaModels(w, false)
}

func (ws *Workspace) basicCheckControls(w *gpui.Window) *gpui.Div {
	a := &ws.prefs().aiBasic
	c := ws.prefs().cur
	_, installed := ws.openCodeAvailable()
	key := c.OpenCodeKey != "" || c.OllamaKey != ""
	if f := ws.prefs().aiKeys; f != nil {
		key = key || strings.TrimSpace(f.opencode.Text()) != "" || strings.TrimSpace(f.ollama.Text()) != ""
	}
	label := i18n.T("ui.settings.ai_test")
	if a.checkResult != nil {
		label = i18n.T("ui.settings.ai_testing")
	}
	row := gpui.NewDiv().FlexCol().Gap(6)
	row.Child(actionButton("ai-test", label, i18n.T("ui.settings.ai_test_tip"), false, installed && key && a.checkResult == nil, func(w *gpui.Window, _ *gpui.App) { ws.testBasicAI(w) }))
	if c.OllamaKey != "" {
		row.Child(actionButton("ai-models-refresh", i18n.T("ui.settings.ai_models_refresh"), i18n.T("ui.settings.ai_models_refresh"), false, a.modelResult == nil, func(w *gpui.Window, _ *gpui.App) { ws.refreshOllamaModels(w, true); w.Refresh() }))
		msg := i18n.T("ui.settings.ai_models_loaded", len(a.models))
		if a.modelResult != nil {
			msg = i18n.T("ui.settings.ai_models_loading")
		}
		if a.modelErr != "" {
			msg = a.modelErr
		}
		row.Child(gpui.NewText(msg).TextSize(11).TextColor(uiMuted))
	}
	if a.checkMessage != "" {
		color := uiErr
		if a.checkOK {
			color = uiOK
		}
		row.Child(gpui.NewText(a.checkMessage).TextSize(11).TextColor(color))
	}
	return row
}

func (ws *Workspace) testBasicAI(w *gpui.Window) {
	a := &ws.prefs().aiBasic
	if a.checkResult != nil {
		return
	}
	ws.saveBasicKeys(w)
	c := ws.prefs().cur.Clone()
	bin, ok := ws.openCodeAvailable()
	if !ok || (c.OpenCodeKey == "" && c.OllamaKey == "") {
		return
	}
	models := slices.Clone(a.models)
	result := make(chan basicCheckResult, 1)
	a.checkResult, a.checkMessage = result, ""
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	a.checkCancel = cancel
	go func() {
		defer cancel()
		result <- basicCheckResult{c.OpenCodeKey, c.OllamaKey, checkBasicAI(ctx, bin, c, models, harnesses.Open)}
	}()
	ws.armBasicPoll(w)
	w.Refresh()
}

func checkBasicAI(ctx context.Context, bin string, c settings.Settings, models []agentmanager.ModelInfo, open func(context.Context, agentmanager.Config) (agentmanager.Session, error)) error {
	dir, err := os.MkdirTemp("", "cad-lite-ai-check-")
	if err != nil {
		return errors.New("cannot create connection test directory")
	}
	defer os.RemoveAll(dir)
	var checks []string
	if c.OpenCodeKey != "" {
		checks = append(checks, "opencode/gpt-5-nano")
	}
	if c.OllamaKey != "" {
		model := basicOllamaModel
		if len(models) > 0 {
			model = models[0].ID
		}
		checks = append(checks, model)
	}
	for _, model := range checks {
		cfg := agentmanager.Config{Harness: agentmanager.OpenCode, Binary: bin, Cwd: dir, Model: model, Env: basicOpenCodeEnv(c, models...)}
		for i, value := range cfg.Env {
			if content, ok := strings.CutPrefix(value, "OPENCODE_CONFIG_CONTENT="); ok {
				var config map[string]any
				_ = json.Unmarshal([]byte(content), &config)
				config["permission"] = "deny"
				data, _ := json.Marshal(config)
				cfg.Env[i] = "OPENCODE_CONFIG_CONTENT=" + string(data)
			}
		}
		if err := checkBasicSession(ctx, cfg, open); err != nil {
			// Backend errors can echo credentials. Never surface raw provider output in settings.
			if ctx.Err() != nil {
				return errors.New("connection test timed out")
			}
			return fmt.Errorf("%s: connection failed; check the key, network and provider access", strings.SplitN(model, "/", 2)[0])
		}
	}
	return nil
}

func checkBasicSession(ctx context.Context, cfg agentmanager.Config, open func(context.Context, agentmanager.Config) (agentmanager.Session, error)) error {
	s, err := open(ctx, cfg)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Prompt(ctx, "Reply with OK only. Do not use tools."); err != nil {
		return err
	}
	text := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-s.Events():
			if !ok {
				return errors.New("session closed before reply")
			}
			switch e := ev.(type) {
			case agentmanager.TextDelta:
				text = text || strings.TrimSpace(e.Text) != ""
			case agentmanager.PermissionRequest:
				_ = s.RespondPermission(e.RequestID, agentmanager.PermissionAnswer{Cancelled: true})
			case agentmanager.TurnEnd:
				if e.Reason != agentmanager.StopEndTurn || !text {
					return errors.New("no successful reply")
				}
				return nil
			case agentmanager.Exited:
				return errors.New("OpenCode exited before reply")
			}
		}
	}
}

func (ws *Workspace) stopBasicAI() {
	a := &ws.prefs().aiBasic
	a.poll.Cancel()
	if a.modelCancel != nil {
		a.modelCancel()
		a.modelCancel = nil
	}
	if a.checkCancel != nil {
		a.checkCancel()
		a.checkCancel = nil
		if a.checkResult != nil {
			select {
			case <-a.checkResult:
			case <-time.After(installQuitWait):
			}
		}
	}
}
