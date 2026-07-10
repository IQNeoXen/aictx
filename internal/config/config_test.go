package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	zalkeyring "github.com/zalando/go-keyring"
)

// ---------- pure-function tests ----------

func TestProviderIsEmpty(t *testing.T) {
	cases := []struct {
		p    Provider
		want bool
	}{
		{Provider{}, true},
		{Provider{Endpoint: "https://example.com"}, false},
		{Provider{APIKey: "key"}, false},
		{Provider{Model: "m"}, false},
		{Provider{SmallModel: "s"}, false},
		{Provider{Headers: map[string]string{"X-Foo": "bar"}}, false},
		// Name is deliberately excluded from IsEmpty: a Name-only provider
		// stays "empty" = native-auth mode.
		{Provider{Name: "x"}, true},
	}
	for _, c := range cases {
		got := c.p.IsEmpty()
		if got != c.want {
			t.Errorf("IsEmpty(%+v) = %v, want %v", c.p, got, c.want)
		}
	}
}

func TestProviderClone(t *testing.T) {
	orig := Provider{
		Endpoint:     "https://example.com",
		APIKey:       "sk-secret",
		Model:        "claude-opus-4-6",
		SmallModel:   "claude-haiku-4-5",
		Headers:      map[string]string{"X-H": "v"},
		ProviderType: "openai",
		Name:         "custom",
	}

	// Guard: every Provider field must be set to a non-zero value above, so
	// this test fails when a new field is added without updating it (and
	// forces a check that Clone() still copies everything).
	v := reflect.ValueOf(orig)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Errorf("Provider field %s not covered by TestProviderClone; set it above and verify Clone()", v.Type().Field(i).Name)
		}
	}

	cp := orig.Clone()
	if !reflect.DeepEqual(cp, orig) {
		t.Errorf("Clone() = %+v, want %+v", cp, orig)
	}

	// Headers map must be independent of the original.
	cp.Headers["X-H"] = "changed"
	if orig.Headers["X-H"] != "v" {
		t.Error("Clone() shares the Headers map with the original")
	}
}

func TestDeepCopy(t *testing.T) {
	orig := &Config{
		State: State{Current: "ctx1", Previous: "ctx0"},
		Contexts: []Context{
			{
				Name: "ctx1",
				Provider: Provider{
					APIKey:  "secret",
					Headers: map[string]string{"X-H": "v"},
				},
				Targets: []TargetEntry{
					{
						ID:  "tgt1",
						Env: map[string]string{"MY_VAR": "original"},
					},
				},
			},
		},
	}

	cp := deepCopy(orig)

	// Mutate copy — original must be unaffected.
	cp.State.Current = "changed"
	cp.Contexts[0].Name = "changed"
	cp.Contexts[0].Provider.APIKey = "changed"
	cp.Contexts[0].Provider.Headers["X-H"] = "changed"
	cp.Contexts[0].Targets[0].Env["MY_VAR"] = "changed"

	if orig.State.Current != "ctx1" {
		t.Errorf("State.Current changed in original")
	}
	if orig.Contexts[0].Name != "ctx1" {
		t.Errorf("Contexts[0].Name changed in original")
	}
	if orig.Contexts[0].Provider.APIKey != "secret" {
		t.Errorf("APIKey changed in original")
	}
	if orig.Contexts[0].Provider.Headers["X-H"] != "v" {
		t.Errorf("Headers changed in original")
	}
	if orig.Contexts[0].Targets[0].Env["MY_VAR"] != "original" {
		t.Errorf("Env changed in original")
	}
}

func TestFindContext(t *testing.T) {
	cfg := &Config{
		Contexts: []Context{
			{Name: "a"},
			{Name: "b"},
		},
	}
	if c := cfg.FindContext("a"); c == nil || c.Name != "a" {
		t.Errorf("FindContext(a) = %v, want non-nil with Name=a", c)
	}
	if c := cfg.FindContext("missing"); c != nil {
		t.Errorf("FindContext(missing) = %v, want nil", c)
	}
}

func TestContextNames(t *testing.T) {
	empty := &Config{}
	if names := empty.ContextNames(); len(names) != 0 {
		t.Errorf("ContextNames on empty = %v, want []", names)
	}

	cfg := &Config{
		Contexts: []Context{{Name: "x"}, {Name: "y"}},
	}
	names := cfg.ContextNames()
	if len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Errorf("ContextNames = %v, want [x y]", names)
	}
}

func TestRemoveContext(t *testing.T) {
	cfg := &Config{
		Contexts: []Context{{Name: "a"}, {Name: "b"}, {Name: "c"}},
	}

	if !cfg.RemoveContext("b") {
		t.Error("RemoveContext(b) = false, want true")
	}
	if len(cfg.Contexts) != 2 {
		t.Errorf("len(Contexts) = %d, want 2", len(cfg.Contexts))
	}
	if cfg.Contexts[0].Name != "a" || cfg.Contexts[1].Name != "c" {
		t.Errorf("Contexts = %v, want [a c]", cfg.Contexts)
	}

	if cfg.RemoveContext("missing") {
		t.Error("RemoveContext(missing) = true, want false")
	}
}

func TestRenameContext(t *testing.T) {
	t.Run("renames context and leaves others intact", func(t *testing.T) {
		cfg := &Config{
			Contexts: []Context{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		}
		if !cfg.RenameContext("b", "beta") {
			t.Fatal("RenameContext returned false, want true")
		}
		names := cfg.ContextNames()
		if len(names) != 3 || names[0] != "a" || names[1] != "beta" || names[2] != "c" {
			t.Errorf("ContextNames = %v, want [a beta c]", names)
		}
	})

	t.Run("updates State.Current", func(t *testing.T) {
		cfg := &Config{
			State:    State{Current: "old"},
			Contexts: []Context{{Name: "old"}},
		}
		cfg.RenameContext("old", "new")
		if cfg.State.Current != "new" {
			t.Errorf("State.Current = %q, want new", cfg.State.Current)
		}
	})

	t.Run("updates State.Previous", func(t *testing.T) {
		cfg := &Config{
			State:    State{Current: "x", Previous: "old"},
			Contexts: []Context{{Name: "x"}, {Name: "old"}},
		}
		cfg.RenameContext("old", "new")
		if cfg.State.Previous != "new" {
			t.Errorf("State.Previous = %q, want new", cfg.State.Previous)
		}
	})

	t.Run("returns false when old and new names are the same", func(t *testing.T) {
		cfg := &Config{Contexts: []Context{{Name: "a"}}}
		if cfg.RenameContext("a", "a") {
			t.Error("RenameContext returned true, want false")
		}
		if cfg.Contexts[0].Name != "a" {
			t.Errorf("Context name modified unexpectedly: %q", cfg.Contexts[0].Name)
		}
	})

	t.Run("returns false when oldName not found", func(t *testing.T) {
		cfg := &Config{Contexts: []Context{{Name: "a"}}}
		if cfg.RenameContext("missing", "b") {
			t.Error("RenameContext returned true, want false")
		}
	})

	t.Run("returns false when newName already exists", func(t *testing.T) {
		cfg := &Config{Contexts: []Context{{Name: "a"}, {Name: "b"}}}
		if cfg.RenameContext("a", "b") {
			t.Error("RenameContext returned true, want false")
		}
		// Neither context should be modified.
		if cfg.Contexts[0].Name != "a" || cfg.Contexts[1].Name != "b" {
			t.Errorf("Contexts modified unexpectedly: %v", cfg.ContextNames())
		}
	})
}

func TestGetTarget(t *testing.T) {
	ctx := &Context{
		Targets: []TargetEntry{{ID: "t1"}, {ID: "t2"}},
	}
	if te := ctx.GetTarget("t1"); te == nil || te.ID != "t1" {
		t.Errorf("GetTarget(t1) = %v", te)
	}
	if te := ctx.GetTarget("nope"); te != nil {
		t.Errorf("GetTarget(nope) = %v, want nil", te)
	}
}

func TestHasTarget(t *testing.T) {
	ctx := &Context{
		Targets: []TargetEntry{{ID: "t1"}},
	}
	if !ctx.HasTarget("t1") {
		t.Error("HasTarget(t1) = false")
	}
	if ctx.HasTarget("t2") {
		t.Error("HasTarget(t2) = true")
	}
}

func TestTargetIDs(t *testing.T) {
	ctx := &Context{
		Targets: []TargetEntry{{ID: "a"}, {ID: "b"}},
	}
	ids := ctx.TargetIDs()
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Errorf("TargetIDs = %v, want [a b]", ids)
	}
}

// ---------- I/O tests ----------

func setupConfigDir(t *testing.T) {
	t.Helper()
	zalkeyring.MockInit()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestLoadMissingFile(t *testing.T) {
	setupConfigDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.Contexts) != 0 {
		t.Errorf("Contexts = %v, want empty", cfg.Contexts)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	setupConfigDir(t)

	orig := &Config{
		State: State{Current: "prod"},
		Contexts: []Context{
			{
				Name:        "prod",
				Description: "production",
				Provider: Provider{
					Endpoint: "https://api.example.com",
					Model:    "claude-3",
				},
				Targets: []TargetEntry{
					{ID: "claude-code-cli"},
				},
			},
		},
	}

	if err := Save(orig); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}

	if loaded.State.Current != "prod" {
		t.Errorf("State.Current = %q, want prod", loaded.State.Current)
	}
	if len(loaded.Contexts) != 1 {
		t.Fatalf("len(Contexts) = %d, want 1", len(loaded.Contexts))
	}
	ctx := loaded.Contexts[0]
	if ctx.Name != "prod" || ctx.Description != "production" {
		t.Errorf("Context = %+v", ctx)
	}
	if ctx.Provider.Endpoint != "https://api.example.com" {
		t.Errorf("Endpoint = %q", ctx.Provider.Endpoint)
	}
	if ctx.Provider.Model != "claude-3" {
		t.Errorf("Model = %q", ctx.Provider.Model)
	}
	if len(ctx.Targets) != 1 || ctx.Targets[0].ID != "claude-code-cli" {
		t.Errorf("Targets = %v", ctx.Targets)
	}
}

func TestSaveEnvPersisted(t *testing.T) {
	setupConfigDir(t)

	cfg := &Config{
		Contexts: []Context{
			{
				Name: "ctx",
				Targets: []TargetEntry{
					{
						ID:  "claude-code-cli",
						Env: map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.example.com", "FOO": "bar"},
					},
				},
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	te := loaded.Contexts[0].Targets[0]
	if len(te.Env) != 2 {
		t.Fatalf("Env len = %d, want 2 (env vars were dropped by Save)", len(te.Env))
	}
	if te.Env["ANTHROPIC_BASE_URL"] != "https://proxy.example.com" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want https://proxy.example.com", te.Env["ANTHROPIC_BASE_URL"])
	}
	if te.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", te.Env["FOO"])
	}

	// Env must also remain intact in the caller's in-memory config.
	if cfg.Contexts[0].Targets[0].Env["FOO"] != "bar" {
		t.Error("Save() cleared Env from caller's in-memory config")
	}
}

func TestSaveAPIKeyScrubbed(t *testing.T) {
	setupConfigDir(t)

	cfg := &Config{
		Contexts: []Context{
			{
				Name:     "ctx",
				Provider: Provider{APIKey: "sk-secret"},
				Targets:  []TargetEntry{{ID: "claude-code-cli"}},
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// The YAML on disk must not contain the API key.
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if contains(data, "sk-secret") {
		t.Error("API key found in YAML file after Save — should have been scrubbed")
	}

	// The in-memory config must still have the key for Apply().
	if cfg.Contexts[0].Provider.APIKey != "sk-secret" {
		t.Error("Save() cleared APIKey from caller's in-memory config")
	}
}

func TestLoadKeyringMigration(t *testing.T) {
	setupConfigDir(t)

	// Write a YAML file with a plain-text API key (old per-target format).
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	yamlContent := `state:
  current: ctx
contexts:
  - name: ctx
    targets:
      - id: claude-code-cli
        provider:
          apiKey: plain-text-key
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// First Load: migrates the plain-text key from target to context level,
	// then stores it in the OS keyring.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// HasKeyringKey should be set at context level after migration.
	if !cfg.Contexts[0].HasKeyringKey {
		t.Error("HasKeyringKey not set at context level after migration")
	}
	// YAML on disk should no longer have the plain key.
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("ReadFile after migration: %v", err)
	}
	if contains(data, "plain-text-key") {
		t.Error("plain-text API key still present in YAML after migration")
	}

	// Second Load: HasKeyringKey=true at context level → key is loaded from keyring into memory.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if cfg2.Contexts[0].Provider.APIKey != "plain-text-key" {
		t.Errorf("APIKey = %q on second load, want plain-text-key", cfg2.Contexts[0].Provider.APIKey)
	}
}

// TestMigrateV1 covers the key migration scenarios.
func TestMigrateV1(t *testing.T) {
	t.Run("lifts provider and options from first non-empty target", func(t *testing.T) {
		setupConfigDir(t)

		dir := Dir()
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Old format: provider at target level
		yamlContent := `state:
  current: work
contexts:
  - name: work
    targets:
      - id: claude-code-cli
        provider:
          endpoint: https://api.example.com
          model: claude-opus-4-6
        options:
          alwaysThinking: true
      - id: claude-code-vscode
        provider:
          endpoint: https://api.example.com
          model: claude-opus-4-6
        options:
          alwaysThinking: true
`
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}

		ctx := cfg.Contexts[0]
		if ctx.Provider.Endpoint != "https://api.example.com" {
			t.Errorf("Provider.Endpoint = %q, want https://api.example.com", ctx.Provider.Endpoint)
		}
		if ctx.Provider.Model != "claude-opus-4-6" {
			t.Errorf("Provider.Model = %q, want claude-opus-4-6", ctx.Provider.Model)
		}
		if ctx.Options.AlwaysThinking == nil || !*ctx.Options.AlwaysThinking {
			t.Error("Options.AlwaysThinking not lifted to context level")
		}
		// Per-target provider should be cleared after migration.
		for _, te := range ctx.Targets {
			if !te.Provider.IsEmpty() {
				t.Errorf("target %s still has per-target provider after migration", te.ID)
			}
		}
	})

	t.Run("already-migrated config is a no-op", func(t *testing.T) {
		setupConfigDir(t)

		// Config already in new format (provider at context level)
		cfg := &Config{
			Contexts: []Context{
				{
					Name:     "work",
					Provider: Provider{Endpoint: "https://api.example.com", Model: "claude-opus-4-6"},
					Targets:  []TargetEntry{{ID: "claude-code-cli"}},
				},
			},
		}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}

		loaded, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if loaded.Contexts[0].Provider.Endpoint != "https://api.example.com" {
			t.Errorf("Provider lost after load of already-migrated config")
		}
		// Targets should still just have ID only.
		if !loaded.Contexts[0].Targets[0].Provider.IsEmpty() {
			t.Error("target has unexpected provider after loading already-migrated config")
		}
	})
}

func contains(data []byte, s string) bool {
	return len(data) > 0 && len(s) > 0 && indexBytes(data, []byte(s)) >= 0
}

func indexBytes(haystack, needle []byte) int {
	n := len(needle)
	for i := 0; i <= len(haystack)-n; i++ {
		match := true
		for j := 0; j < n; j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
