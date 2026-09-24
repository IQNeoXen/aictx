package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/IQNeoXen/aictx/internal/keyring"
	"github.com/IQNeoXen/aictx/internal/target/claudecli"
	zalkeyring "github.com/zalando/go-keyring"
)

func setupKeyEnv(t *testing.T) string {
	t.Helper()
	zalkeyring.MockInit()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// Cobra's Changed flags persist across executions in this package.
	t.Cleanup(func() {
		keyUpdateCmd.Flags().Set("api-key", "")
		keyUpdateCmd.Flags().Lookup("api-key").Changed = false
	})
	return home
}

func saveKeyContext(t *testing.T, cfg *config.Config) {
	t.Helper()
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func captureKeyOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	runErr := run()
	w.Close()
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	r.Close()
	return string(data), runErr
}

func TestKeyUpdateExplicitAndInactive(t *testing.T) {
	home := setupKeyEnv(t)
	cfg := &config.Config{State: config.State{Current: "other", Previous: "older"}, Contexts: []config.Context{
		{Name: "work", Description: "kept", Provider: config.Provider{APIKey: "old-secret", Model: "claude"}, Targets: []config.TargetEntry{{ID: claudecli.ID}}},
		{Name: "other"},
	}}
	saveKeyContext(t, cfg)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"old-secret"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "work", "--api-key", "new-secret") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not active") || strings.Contains(out, "old-secret") || strings.Contains(out, "new-secret") {
		t.Errorf("unexpected output %q", out)
	}
	yaml, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yaml), "old-secret") || strings.Contains(string(yaml), "new-secret") {
		t.Error("key in YAML")
	}
	target, _ := os.ReadFile(settings)
	if !strings.Contains(string(target), "old-secret") {
		t.Error("inactive target modified")
	}
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.FindContext("work").Provider.APIKey != "new-secret" || reloaded.FindContext("work").Description != "kept" || reloaded.State.Current != "other" || reloaded.State.Previous != "older" {
		t.Error("reload lost key or metadata")
	}
}

func TestKeyUpdateActiveAppliesWithoutSwitchSideEffects(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"STALE":"old","UNCHANGED":"value"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{State: config.State{Current: "work", Previous: "previous", AppliedEnvKeys: map[string][]string{claudecli.ID: {"STALE"}}}, Contexts: []config.Context{{Name: "work", Command: "touch " + filepath.Join(home, "sentinel"), Provider: config.Provider{APIKey: "old-secret", Model: "claude"}, Targets: []config.TargetEntry{{ID: claudecli.ID, Env: map[string]string{"NEW": "yes"}}}}}}
	saveKeyContext(t, cfg)
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Claude Code CLI") || strings.Contains(out, "new-secret") {
		t.Errorf("unexpected output %q", out)
	}
	var settingsJSON struct {
		Env map[string]string `json:"env"`
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &settingsJSON); err != nil {
		t.Fatal(err)
	}
	if settingsJSON.Env["ANTHROPIC_AUTH_TOKEN"] != "new-secret" || settingsJSON.Env["NEW"] != "yes" || settingsJSON.Env["UNCHANGED"] != "value" || settingsJSON.Env["STALE"] != "" {
		t.Errorf("env = %v", settingsJSON.Env)
	}
	if _, err := os.Stat(filepath.Join(home, "sentinel")); !os.IsNotExist(err) {
		t.Error("post-switch command executed")
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State.Current != "work" || loaded.State.Previous != "previous" || !strings.Contains(strings.Join(loaded.State.AppliedEnvKeys[claudecli.ID], ","), "ANTHROPIC_AUTH_TOKEN") {
		t.Errorf("state = %+v", loaded.State)
	}
}

func TestKeyUpdateGuards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context config.Context
	}{
		{"oauth", config.Context{Name: "work", HasOAuthKey: true, Provider: config.Provider{APIKey: "old-secret"}}},
		{"copilot", config.Context{Name: "work", Provider: config.Provider{ProviderType: "copilot", APIKey: "old-secret"}}},
		{"native", config.Context{Name: "work", Provider: config.Provider{Model: "claude"}}},
		{"keyless endpoint", config.Context{Name: "work", Provider: config.Provider{Endpoint: "http://localhost:1234"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupKeyEnv(t)
			saveKeyContext(t, &config.Config{State: config.State{Current: "work"}, Contexts: []config.Context{tc.context}})
			_, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
			if err == nil {
				t.Fatal("expected guard failure")
			}
			loaded, loadErr := config.Load()
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if loaded.FindContext("work").Provider.APIKey == "new-secret" {
				t.Error("guard changed key")
			}
		})
	}
	t.Run("missing or unknown", func(t *testing.T) {
		setupKeyEnv(t)
		for _, args := range [][]string{{"key", "update", "--api-key", "new-secret"}, {"key", "update", "unknown", "--api-key", "new-secret"}} {
			if err := executeCmd(args...); err == nil {
				t.Errorf("expected error for %v", args)
			}
		}
	})
	t.Run("direct Anthropic", func(t *testing.T) {
		setupKeyEnv(t)
		saveKeyContext(t, &config.Config{Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret"}}}})
		if err := executeCmd("key", "update", "work", "--api-key", "new-secret"); err != nil {
			t.Fatal(err)
		}
		loaded, _ := config.Load()
		if loaded.FindContext("work").Provider.APIKey != "new-secret" {
			t.Error("direct Anthropic key not updated")
		}
	})
}

func TestKeyUpdateInput(t *testing.T) {
	setupKeyEnv(t)
	saveKeyContext(t, &config.Config{Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret"}}}})
	original := keyInput
	t.Cleanup(func() { keyInput = original })
	reads := 0
	keyInput.isTerminal = func(int) bool { return false }
	keyInput.readPassword = func(int) ([]byte, error) { reads++; return []byte("new-secret"), nil }
	if err := executeCmd("key", "update", "work"); err == nil || !strings.Contains(err.Error(), "terminal") || reads != 0 {
		t.Errorf("nonterminal: %v, reads %d", err, reads)
	}
	if err := executeCmd("key", "update", "work", "--api-key", ""); err == nil || reads != 0 {
		t.Errorf("empty flag: %v, reads %d", err, reads)
	}
	keyUpdateCmd.Flags().Lookup("api-key").Changed = false
	keyInput.isTerminal = func(int) bool { return true }
	keyInput.readPassword = func(int) ([]byte, error) { reads++; return []byte("   "), nil }
	if err := executeCmd("key", "update", "work"); err == nil || reads != 1 {
		t.Errorf("blank interactive: %v, reads %d", err, reads)
	}
	keyInput.readPassword = func(int) ([]byte, error) { reads++; return []byte("new-secret"), nil }
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "work") })
	if err != nil || reads != 2 || strings.Contains(out, "new-secret") || !strings.Contains(out, "New API key") {
		t.Errorf("prompt: %v, reads %d, out %q", err, reads, out)
	}
}

func TestKeyUpdateFailedApplyKeepsReplacement(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(settings, 0755); err != nil {
		t.Fatal(err)
	} // Detect succeeds but atomic rename to directory fails.
	saveKeyContext(t, &config.Config{State: config.State{Current: "work"}, Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret"}, Targets: []config.TargetEntry{{ID: claudecli.ID}}}}})
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
	if err == nil || !strings.Contains(err.Error(), "saved") || !strings.Contains(err.Error(), "retry") || strings.Contains(err.Error(), "new-secret") || strings.Contains(out, "new-secret") {
		t.Errorf("failure: %v, output %q", err, out)
	}
	stored, getErr := keyring.Get("work")
	if getErr != nil || stored != "new-secret" {
		t.Errorf("stored = %q, %v", stored, getErr)
	}
}

func TestKeyUpdatePartialApplyKeepsReplacementAndTracking(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(settings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755); err != nil {
		t.Fatal(err)
	}
	saveKeyContext(t, &config.Config{State: config.State{Current: "work", Previous: "before"}, Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret", Model: "claude"}, Targets: []config.TargetEntry{{ID: claudecli.ID}, {ID: "pi-cli"}}}}})
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
	if err == nil || !strings.Contains(err.Error(), "saved") || !strings.Contains(out, "pi Coding Agent CLI") {
		t.Errorf("partial failure: %v, output %q", err, out)
	}
	loaded, loadErr := config.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.FindContext("work").Provider.APIKey != "new-secret" || loaded.State.Current != "work" || loaded.State.Previous != "before" {
		t.Errorf("state/credential lost: %+v", loaded.State)
	}
	ext, readErr := os.ReadFile(filepath.Join(home, ".pi", "agent", "extensions", "aictx-provider.ts"))
	if readErr != nil || !strings.Contains(string(ext), "new-secret") {
		t.Errorf("pi extension not refreshed: %v", readErr)
	}
}

func TestKeyUpdateFirstSaveFailureDoesNotApply(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"old-secret"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	saveKeyContext(t, &config.Config{State: config.State{Current: "work"}, Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret"}, Targets: []config.TargetEntry{{ID: claudecli.ID}}}}})
	before, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	zalkeyring.MockInitWithError(errors.New("unavailable"))
	out, runErr := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
	if runErr == nil || strings.Contains(out, "updated") {
		t.Errorf("expected save failure, got %v, %q", runErr, out)
	}
	after, _ := os.ReadFile(config.Path())
	target, _ := os.ReadFile(settings)
	if string(before) != string(after) || !strings.Contains(string(target), "old-secret") {
		t.Error("first-save failure changed YAML or target")
	}
}

func TestKeyUpdateSecondSaveFailureKeepsSavedKey(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	saveKeyContext(t, &config.Config{State: config.State{Current: "work", Previous: "before"}, Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "old-secret"}, Targets: []config.TargetEntry{{ID: claudecli.ID}}}}})
	original := saveKeyConfig
	calls := 0
	saveKeyConfig = func(cfg *config.Config) error {
		calls++
		if calls == 2 {
			return errors.New("injected second-save failure")
		}
		return config.Save(cfg)
	}
	t.Cleanup(func() { saveKeyConfig = original })
	out, err := captureKeyOutput(t, func() error { return executeCmd("key", "update", "--api-key", "new-secret") })
	if calls != 2 || err == nil || !strings.Contains(err.Error(), "saved") || !strings.Contains(err.Error(), "retry") || strings.Contains(err.Error(), "new-secret") || strings.Contains(out, "new-secret") {
		t.Errorf("second save: calls %d, err %v, out %q", calls, err, out)
	}
	stored, getErr := keyring.Get("work")
	loaded, loadErr := config.Load()
	if getErr != nil || loadErr != nil || stored != "new-secret" || loaded.FindContext("work").Provider.APIKey != "new-secret" {
		t.Errorf("replacement not persisted: %q, %v, %v", stored, getErr, loadErr)
	}
	if loaded.State.Current != "work" || loaded.State.Previous != "before" {
		t.Errorf("selection changed: %+v", loaded.State)
	}
	target, readErr := os.ReadFile(settings)
	if readErr != nil || !strings.Contains(string(target), "new-secret") {
		t.Errorf("target not updated: %v", readErr)
	}
	data, _ := os.ReadFile(config.Path())
	if strings.Contains(string(data), "new-secret") || strings.Contains(string(data), "old-secret") {
		t.Error("key leaked to YAML")
	}
}

func TestSwitchContextPartialApplyPreservesBehavior(t *testing.T) {
	home := setupKeyEnv(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(settings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Contexts: []config.Context{{Name: "work", Provider: config.Provider{APIKey: "secret"}, Targets: []config.TargetEntry{{ID: claudecli.ID}, {ID: "pi-cli"}}}}}
	_, err := captureKeyOutput(t, func() error { return switchContext(cfg, "work") })
	if err != nil || cfg.State.Current != "work" {
		t.Errorf("partial switch = %v, state %+v", err, cfg.State)
	}
}
