package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	return &cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return os.Rename(tmp, Path())
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
