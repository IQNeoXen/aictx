package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IQNeoXen/aictx/internal/keyring"
	"gopkg.in/yaml.v3"
)

// Dir returns the aictx config directory path.
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aictx")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aictx")
}

// Path returns the full path to config.yaml.
func Path() string {
	return filepath.Join(Dir(), "config.yaml")
}

// optionsHasFields returns true if any Options field is non-nil.
func optionsHasFields(o Options) bool {
	return o.AlwaysThinking != nil || o.DisableTelemetry != nil || o.DisableBetas != nil
}

// migrateV1 detects and lifts per-target Provider/Options/HasKeyringKey fields to the
// context level (the new canonical location). It also migrates legacy per-target keyring
// entries to the new context-level keyring account.
//
// Returns true if the config was modified and needs to be saved.
func migrateV1(cfg *Config) bool {
	needsSave := false

	for ci := range cfg.Contexts {
		ctx := &cfg.Contexts[ci]

		// Check whether any target carries legacy per-target fields.
		needsLift := false
		for _, te := range ctx.Targets {
			if !te.Provider.IsEmpty() || optionsHasFields(te.Options) || te.HasKeyringKey {
				needsLift = true
				break
			}
		}
		if !needsLift {
			continue
		}

		needsSave = true

		// Lift first non-empty Provider to context level (if not already set).
		if ctx.Provider.IsEmpty() {
			for _, te := range ctx.Targets {
				if !te.Provider.IsEmpty() {
					ctx.Provider = te.Provider
					break
				}
			}
		}

		// Lift first non-nil Options to context level (if not already set).
		if !optionsHasFields(ctx.Options) {
			for _, te := range ctx.Targets {
				if optionsHasFields(te.Options) {
					ctx.Options = te.Options
					break
				}
			}
		}

		// Warn if multiple targets had differing non-empty API keys.
		var nonEmptyKeys []string
		for _, te := range ctx.Targets {
			if te.Provider.APIKey != "" {
				nonEmptyKeys = append(nonEmptyKeys, te.Provider.APIKey)
			}
		}
		if len(nonEmptyKeys) > 1 {
			first := nonEmptyKeys[0]
			for _, k := range nonEmptyKeys[1:] {
				if k != first {
					fmt.Fprintf(os.Stderr, "aictx: warning: context %q had different API keys per target; using first non-empty key\n", ctx.Name)
					break
				}
			}
		}

		// Migrate legacy per-target keyring entries to the new context-level account.
		for _, te := range ctx.Targets {
			if !te.HasKeyringKey {
				continue
			}
			apiKey, kerr := keyring.GetLegacy(ctx.Name, te.ID)
			if kerr != nil {
				fmt.Fprintf(os.Stderr, "aictx: warning: could not read old keyring entry for %s/%s: %v\n", ctx.Name, te.ID, kerr)
				continue
			}
			// Use the first non-empty key found from legacy keyring (prefer in-memory key).
			if apiKey != "" && ctx.Provider.APIKey == "" {
				ctx.Provider.APIKey = apiKey
			}
			// Remove the old keyring entry.
			if derr := keyring.DeleteLegacy(ctx.Name, te.ID); derr != nil {
				fmt.Fprintf(os.Stderr, "aictx: warning: could not delete old keyring entry for %s/%s: %v\n", ctx.Name, te.ID, derr)
			}
		}

		// Slim down per-target entries: clear Provider/Options/HasKeyringKey.
		for ti := range ctx.Targets {
			ctx.Targets[ti].Provider = Provider{}
			ctx.Targets[ti].Options = Options{}
			ctx.Targets[ti].HasKeyringKey = false
		}
	}

	return needsSave
}

// Load reads the config from disk. Returns an empty config if the file doesn't exist.
// On first load, old per-target provider/options fields are automatically lifted to the
// context level (transparent migration). API keys stored as plain text are migrated to
// the OS keychain. API keys stored in the keychain are populated into memory (not
// persisted to YAML).
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Run legacy migration (per-target → context-level Provider/Options).
	needsSave := migrateV1(&cfg)

	// Handle context-level keyring.
	for ci := range cfg.Contexts {
		ctx := &cfg.Contexts[ci]

		if ctx.Provider.APIKey != "" {
			// Plain-text API key: migrate to keyring. Keep in memory for Apply().
			if kerr := keyring.Set(ctx.Name, ctx.Provider.APIKey); kerr != nil {
				fmt.Fprintf(os.Stderr, "aictx: warning: could not migrate API key to keychain for %s: %v\n", ctx.Name, kerr)
				// Leave the key in place so it still works.
				continue
			}
			ctx.HasKeyringKey = true
			// Keep ctx.Provider.APIKey populated in memory; Save() will scrub it from disk.
			needsSave = true
		} else if ctx.HasKeyringKey {
			// Load from keyring into memory only.
			apiKey, kerr := keyring.Get(ctx.Name)
			if kerr != nil {
				fmt.Fprintf(os.Stderr, "aictx: warning: could not read API key from keychain for %s: %v\n", ctx.Name, kerr)
				continue
			}
			ctx.Provider.APIKey = apiKey
		}
	}

	if needsSave {
		if err := Save(&cfg); err != nil {
			return nil, fmt.Errorf("saving migrated config: %w", err)
		}
	}

	return &cfg, nil
}

// Save writes the config to disk.
// API keys are stored in the OS keychain; the YAML file will never contain a
// non-empty apiKey field after a save. Save operates on a deep copy so that the
// caller's in-memory Config (including APIKey values needed for Apply()) is unchanged.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	// Work on a deep copy so we can clear APIKey without affecting the caller.
	disk := deepCopy(cfg)

	for ci := range disk.Contexts {
		ctx := &disk.Contexts[ci]
		if ctx.Provider.APIKey != "" {
			if kerr := keyring.Set(ctx.Name, ctx.Provider.APIKey); kerr != nil {
				fmt.Fprintf(os.Stderr, "aictx: warning: could not store API key in keychain for %s: %v\n", ctx.Name, kerr)
				// Fall through: key stays in YAML to avoid data loss.
				continue
			}
			ctx.HasKeyringKey = true
			ctx.Provider.APIKey = "" // scrub from disk representation
		}
	}

	data, err := yaml.Marshal(disk)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return os.Rename(tmp, Path())
}

// deepCopy returns a deep copy of cfg so callers can mutate the copy freely.
func deepCopy(cfg *Config) *Config {
	cp := &Config{
		State:        cfg.State,
		CopilotLogin: cfg.CopilotLogin, // value copy — safe, no nested references
	}
	cp.Contexts = make([]Context, len(cfg.Contexts))
	for ci, ctx := range cfg.Contexts {
		cCtx := Context{
			Name:          ctx.Name,
			Description:   ctx.Description,
			Command:       ctx.Command,
			HasKeyringKey: ctx.HasKeyringKey,
			HasOAuthKey:   ctx.HasOAuthKey,
			Options:       ctx.Options,
			Provider:      ctx.Provider.Clone(),
		}
		cCtx.Targets = make([]TargetEntry, len(ctx.Targets))
		for ti, te := range ctx.Targets {
			cTe := TargetEntry{
				ID:            te.ID,
				HasKeyringKey: te.HasKeyringKey,
				Options:       te.Options,
				Provider:      te.Provider.Clone(),
			}
			if te.Env != nil {
				cTe.Env = make(map[string]string, len(te.Env))
				for k, v := range te.Env {
					cTe.Env[k] = v
				}
			}
			cCtx.Targets[ti] = cTe
		}
		cp.Contexts[ci] = cCtx
	}
	return cp
}

// FindContext returns the context with the given name, or nil.
func (c *Config) FindContext(name string) *Context {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i]
		}
	}
	return nil
}

// ContextNames returns a list of all context names.
func (c *Config) ContextNames() []string {
	names := make([]string, len(c.Contexts))
	for i, ctx := range c.Contexts {
		names[i] = ctx.Name
	}
	return names
}

// RenameContext renames oldName to newName in the context list and updates
// State.Current/Previous if they referenced oldName.
// Returns false if oldName is not found or newName already exists.
func (c *Config) RenameContext(oldName, newName string) bool {
	if oldName == newName {
		return false
	}
	idx := -1
	for i := range c.Contexts {
		switch c.Contexts[i].Name {
		case oldName:
			idx = i
		case newName:
			return false // newName already taken
		}
	}
	if idx == -1 {
		return false // oldName not found
	}
	c.Contexts[idx].Name = newName
	if c.State.Current == oldName {
		c.State.Current = newName
	}
	if c.State.Previous == oldName {
		c.State.Previous = newName
	}
	return true
}

// RemoveContext removes a context by name. Returns false if not found.
func (c *Config) RemoveContext(name string) bool {
	for i, ctx := range c.Contexts {
		if ctx.Name == name {
			c.Contexts = append(c.Contexts[:i], c.Contexts[i+1:]...)
			return true
		}
	}
	return false
}
