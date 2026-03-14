package config

import (
	"os"
	"path/filepath"
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
	}
	for _, c := range cases {
		got := c.p.IsEmpty()
		if got != c.want {
			t.Errorf("IsEmpty(%+v) = %v, want %v", c.p, got, c.want)
		}
	}
}

func TestDeepCopy(t *testing.T) {
	orig := &Config{
		State: State{Current: "ctx1", Previous: "ctx0"},
		Contexts: []Context{
			{
				Name: "ctx1",
				Targets: []TargetEntry{
					{
						ID: "tgt1",
						Provider: Provider{
							APIKey:  "secret",
							Headers: map[string]string{"X-H": "v"},
						},
					},
				},
			},
		},
	}

	cp := deepCopy(orig)

	// Mutate copy — original must be unaffected.
	cp.State.Current = "changed"
	cp.Contexts[0].Name = "changed"
	cp.Contexts[0].Targets[0].Provider.APIKey = "changed"
	cp.Contexts[0].Targets[0].Provider.Headers["X-H"] = "changed"

	if orig.State.Current != "ctx1" {
		t.Errorf("State.Current changed in original")
	}
	if orig.Contexts[0].Name != "ctx1" {
		t.Errorf("Contexts[0].Name changed in original")
	}
	if orig.Contexts[0].Targets[0].Provider.APIKey != "secret" {
		t.Errorf("APIKey changed in original")
	}
	if orig.Contexts[0].Targets[0].Provider.Headers["X-H"] != "v" {
		t.Errorf("Headers changed in original")
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
				Targets: []TargetEntry{
					{
						ID: "claude-code-cli",
						Provider: Provider{
							Endpoint: "https://api.example.com",
							Model:    "claude-3",
						},
					},
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
	if len(ctx.Targets) != 1 {
		t.Fatalf("len(Targets) = %d, want 1", len(ctx.Targets))
	}
	te := ctx.Targets[0]
	if te.Provider.Endpoint != "https://api.example.com" {
		t.Errorf("Endpoint = %q", te.Provider.Endpoint)
	}
	if te.Provider.Model != "claude-3" {
		t.Errorf("Model = %q", te.Provider.Model)
	}
}

func TestSaveAPIKeyScrubbed(t *testing.T) {
	setupConfigDir(t)

	cfg := &Config{
		Contexts: []Context{
			{
				Name: "ctx",
				Targets: []TargetEntry{
					{ID: "claude-code-cli", Provider: Provider{APIKey: "sk-secret"}},
				},
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
	if cfg.Contexts[0].Targets[0].Provider.APIKey != "sk-secret" {
		t.Error("Save() cleared APIKey from caller's in-memory config")
	}
}

func TestLoadKeyringMigration(t *testing.T) {
	setupConfigDir(t)

	// Write a YAML file with a plain-text API key (old format).
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

	// First Load: migrates the plain-text key to keyring, clears it from YAML.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// HasKeyringKey should be set after migration.
	if !cfg.Contexts[0].Targets[0].HasKeyringKey {
		t.Error("HasKeyringKey not set after migration")
	}
	// YAML on disk should no longer have the plain key.
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("ReadFile after migration: %v", err)
	}
	if contains(data, "plain-text-key") {
		t.Error("plain-text API key still present in YAML after migration")
	}

	// Second Load: HasKeyringKey=true → key is loaded from keyring into memory.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if cfg2.Contexts[0].Targets[0].Provider.APIKey != "plain-text-key" {
		t.Errorf("APIKey = %q on second load, want plain-text-key", cfg2.Contexts[0].Targets[0].Provider.APIKey)
	}
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
