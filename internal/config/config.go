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

// Load reads the config from disk. Returns an empty config if the file doesn't exist.
// After loading, API keys stored as plain text are automatically migrated to the OS
// keychain. API keys stored in the keychain are populated into memory (not persisted
// to YAML).
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

	// Migrate plain-text API keys to keyring, and populate in-memory keys from keyring.
	needsSave := false
	for ci := range cfg.Contexts {
		ctx := &cfg.Contexts[ci]
		for ti := range ctx.Targets {
			te := &ctx.Targets[ti]

			if te.Provider.APIKey != "" {
				// Old plain-text format: migrate to keyring.
				if kerr := keyring.Set(ctx.Name, te.ID, te.Provider.APIKey); kerr != nil {
					fmt.Fprintf(os.Stderr, "aictx: warning: could not migrate API key to keychain for %s/%s: %v\n", ctx.Name, te.ID, kerr)
					// Leave the key in place so it still works.
					continue
				}
				te.HasKeyringKey = true
				te.Provider.APIKey = "" // will be cleared from YAML on save
				needsSave = true
			} else if te.HasKeyringKey {
				// Load from keyring into memory only.
				apiKey, kerr := keyring.Get(ctx.Name, te.ID)
				if kerr != nil {
					fmt.Fprintf(os.Stderr, "aictx: warning: could not read API key from keychain for %s/%s: %v\n", ctx.Name, te.ID, kerr)
					continue
				}
				te.Provider.APIKey = apiKey
			}
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
		for ti := range ctx.Targets {
			te := &ctx.Targets[ti]
			if te.Provider.APIKey != "" {
				if kerr := keyring.Set(ctx.Name, te.ID, te.Provider.APIKey); kerr != nil {
					fmt.Fprintf(os.Stderr, "aictx: warning: could not store API key in keychain for %s/%s: %v\n", ctx.Name, te.ID, kerr)
					// Fall through: key stays in YAML to avoid data loss.
					continue
				}
				te.HasKeyringKey = true
				te.Provider.APIKey = "" // scrub from disk representation
			}
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
		State: cfg.State,
	}
	cp.Contexts = make([]Context, len(cfg.Contexts))
	for ci, ctx := range cfg.Contexts {
		cCtx := Context{
			Name:        ctx.Name,
			Description: ctx.Description,
		}
		cCtx.Targets = make([]TargetEntry, len(ctx.Targets))
		for ti, te := range ctx.Targets {
			cTe := TargetEntry{
				ID:            te.ID,
				HasKeyringKey: te.HasKeyringKey,
				Options:       te.Options,
				Provider: Provider{
					Endpoint:   te.Provider.Endpoint,
					APIKey:     te.Provider.APIKey,
					Model:      te.Provider.Model,
					SmallModel: te.Provider.SmallModel,
				},
			}
			if te.Provider.Headers != nil {
				cTe.Provider.Headers = make(map[string]string, len(te.Provider.Headers))
				for k, v := range te.Provider.Headers {
					cTe.Provider.Headers[k] = v
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
