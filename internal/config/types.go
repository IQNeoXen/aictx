package config

// Context represents a named AI tool configuration.
type Context struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Targets     []TargetEntry `yaml:"targets"`
	Provider    Provider      `yaml:"provider,omitempty"`
	Options     Options       `yaml:"options,omitempty"`
}

// TargetEntry specifies a target to configure, with optional overrides.
type TargetEntry struct {
	ID        string   `yaml:"id"`
	Overrides Provider `yaml:"overrides,omitempty"`
}

// Provider holds abstract connection settings that each target translates
// into its own config format.
type Provider struct {
	Endpoint   string            `yaml:"endpoint,omitempty"`
	APIKey     string            `yaml:"apiKey,omitempty"`
	Model      string            `yaml:"model,omitempty"`
	SmallModel string            `yaml:"smallModel,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
}

// IsEmpty returns true if no provider fields are set (i.e. native auth / OAuth).
func (p Provider) IsEmpty() bool {
	return p.Endpoint == "" && p.APIKey == "" && p.Model == "" && p.SmallModel == "" && len(p.Headers) == 0
}

// Options holds behavioral flags.
type Options struct {
	AlwaysThinking   *bool `yaml:"alwaysThinking,omitempty"`
	DisableTelemetry *bool `yaml:"disableTelemetry,omitempty"`
	DisableBetas     *bool `yaml:"disableBetas,omitempty"`
}

// EffectiveProvider merges the base provider with target-specific overrides.
// Override fields win when non-empty.
func (c *Context) EffectiveProvider(targetID string) Provider {
	p := c.Provider
	for _, te := range c.Targets {
		if te.ID == targetID {
			if te.Overrides.Endpoint != "" {
				p.Endpoint = te.Overrides.Endpoint
			}
			if te.Overrides.APIKey != "" {
				p.APIKey = te.Overrides.APIKey
			}
			if te.Overrides.Model != "" {
				p.Model = te.Overrides.Model
			}
			if te.Overrides.SmallModel != "" {
				p.SmallModel = te.Overrides.SmallModel
			}
			if len(te.Overrides.Headers) > 0 {
				merged := make(map[string]string)
				for k, v := range p.Headers {
					merged[k] = v
				}
				for k, v := range te.Overrides.Headers {
					merged[k] = v
				}
				p.Headers = merged
			}
			break
		}
	}
	return p
}

// HasTarget returns true if this context includes the given target ID.
func (c *Context) HasTarget(targetID string) bool {
	for _, te := range c.Targets {
		if te.ID == targetID {
			return true
		}
	}
	return false
}

// TargetIDs returns the list of target IDs in this context.
func (c *Context) TargetIDs() []string {
	ids := make([]string, len(c.Targets))
	for i, te := range c.Targets {
		ids[i] = te.ID
	}
	return ids
}

// State tracks which context is active.
type State struct {
	Current  string `yaml:"current"`
	Previous string `yaml:"previous,omitempty"`
}

// Config is the top-level aictx configuration.
type Config struct {
	State    State     `yaml:"state"`
	Contexts []Context `yaml:"contexts"`
}
