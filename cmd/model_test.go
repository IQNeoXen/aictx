package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IQNeoXen/aictx/internal/config"
	zalkeyring "github.com/zalando/go-keyring"
)

func setupModelCmdEnv(t *testing.T) string {
	t.Helper()
	zalkeyring.MockInit()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

func TestModel_CopilotGuard(t *testing.T) {
	setupModelCmdEnv(t)
	cfg := &config.Config{
		State: config.State{Current: "cop"},
		Contexts: []config.Context{
			{
				Name:     "cop",
				Provider: config.Provider{ProviderType: "copilot", Model: "gpt-4o"},
				Targets:  []config.TargetEntry{{ID: "pi-cli"}},
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err := modelRun(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Copilot") {
		t.Errorf("expected Copilot guard error, got %v", err)
	}

	// Config unchanged.
	reloaded, _ := config.Load()
	if reloaded.FindContext("cop").Provider.Model != "gpt-4o" {
		t.Error("config model should be unchanged after copilot guard")
	}
}

func TestModel_V1EndpointFallbackPersistsAndRegenerates(t *testing.T) {
	home := setupModelCmdEnv(t)

	// pi-cli must be detected: create ~/.pi/agent.
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755); err != nil {
		t.Fatalf("mkdir pi: %v", err)
	}

	seenFirst := false
	seenFallback := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/v1/models":
			seenFirst = true
			w.WriteHeader(http.StatusNotFound)
		case "/v1/models":
			seenFallback = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"meta/muse-glimmer"},{"id":"text-embedding-nomic-embed-text-v1.5"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		State: config.State{Current: "local"},
		Contexts: []config.Context{
			{
				Name: "local",
				Provider: config.Provider{
					Endpoint:     srv.URL + "/v1",
					ProviderType: "openai",
					Name:         "lmstudio",
					Model:        "stale/model",
				},
				Targets: []config.TargetEntry{{ID: "pi-cli"}},
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Non-TTY: feed selection via stdin.
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	w.WriteString("meta/muse-glimmer\n")
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	out := captureStdout(func() {
		if err := modelRun(nil, nil); err != nil {
			t.Errorf("modelRun: %v", err)
		}
	})

	if !seenFirst {
		t.Error("expected initial /v1/v1/models request")
	}
	if !seenFallback {
		t.Error("expected fallback /v1/models request")
	}
	if !strings.Contains(out, "Available models: meta/muse-glimmer, text-embedding-nomic-embed-text-v1.5") {
		t.Errorf("output should list fallback models; got %q", out)
	}

	reloaded, _ := config.Load()
	if got := reloaded.FindContext("local").Provider.Model; got != "meta/muse-glimmer" {
		t.Errorf("persisted model = %q, want meta/muse-glimmer", got)
	}

	extPath := filepath.Join(home, ".pi", "agent", "extensions", "aictx-provider.ts")
	data, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatalf("read extension: %v", err)
	}
	ext := string(data)
	if !strings.Contains(ext, "meta/muse-glimmer") {
		t.Errorf("extension should reference selected model; got:\n%s", ext)
	}
	if !strings.Contains(ext, `apiKey: "aictx-local"`) {
		t.Errorf("keyless extension should include placeholder apiKey; got:\n%s", ext)
	}
}

func TestModel_EmptyEndpointGuard(t *testing.T) {
	setupModelCmdEnv(t)
	cfg := &config.Config{
		State: config.State{Current: "native"},
		Contexts: []config.Context{
			{
				Name:     "native",
				Provider: config.Provider{Model: "claude-opus-4.8"},
				Targets:  []config.TargetEntry{{ID: "pi-cli"}},
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err := modelRun(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no provider endpoint") {
		t.Errorf("expected empty-endpoint guard error, got %v", err)
	}
}

func TestModel_NoCurrentContext(t *testing.T) {
	setupModelCmdEnv(t)
	cfg := &config.Config{}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	err := modelRun(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no current context") {
		t.Errorf("expected no-current-context error, got %v", err)
	}
}

func TestModel_PersistsAndRegenerates(t *testing.T) {
	home := setupModelCmdEnv(t)

	// pi-cli must be detected: create ~/.pi/agent.
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755); err != nil {
		t.Fatalf("mkdir pi: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"claude-opus-4.8"},{"id":"claude-sonnet-5"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.Config{
		State: config.State{Current: "tz"},
		Contexts: []config.Context{
			{
				Name: "tz",
				Provider: config.Provider{
					Endpoint: srv.URL,
					APIKey:   "sk-test",
					Model:    "claude-opus-4.8",
				},
				Targets: []config.TargetEntry{{ID: "pi-cli"}},
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Re-store the API key in the mock keyring so Load() repopulates it.
	zalkeyring.Set("aictx", "tz", "sk-test")

	// Non-TTY: feed selection via stdin.
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	w.WriteString("claude-sonnet-5\n")
	w.Close()
	defer func() { os.Stdin = oldStdin }()

	out := captureStdout(func() {
		if err := modelRun(nil, nil); err != nil {
			t.Errorf("modelRun: %v", err)
		}
	})

	if !strings.Contains(out, "claude-sonnet-5") {
		t.Errorf("output should mention selected model; got %q", out)
	}

	// Config persisted the new model.
	reloaded, _ := config.Load()
	if got := reloaded.FindContext("tz").Provider.Model; got != "claude-sonnet-5" {
		t.Errorf("persisted model = %q, want claude-sonnet-5", got)
	}

	// Extension regenerated referencing the model.
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "aictx-provider.ts")
	data, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatalf("read extension: %v", err)
	}
	if !strings.Contains(string(data), "claude-sonnet-5") {
		t.Errorf("extension should reference claude-sonnet-5; got:\n%s", string(data))
	}
}
