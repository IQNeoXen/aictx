package config

// Context represents a named AI tool configuration.
type Context struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Targets     []TargetEntry `yaml:"targets"`
}

// TargetEntry specifies a target and its configuration.
type TargetEntry struct {
	ID            string            `yaml:"id"`
	Provider      Provider          `yaml:"provider,omitempty"`
	Options       Options           `yaml:"options,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
	HasKeyringKey bool              `yaml:"hasKeyringKey,omitempty"`
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

// GetTarget returns the TargetEntry for the given ID, or nil.
func (c *Context) GetTarget(targetID string) *TargetEntry {
	for i := range c.Targets {
		if c.Targets[i].ID == targetID {
			return &c.Targets[i]
		}
	}
	return nil
}

// HasTarget returns true if this context includes the given target ID.
func (c *Context) HasTarget(targetID string) bool {
	return c.GetTarget(targetID) != nil
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
	Current         string              `yaml:"current"`
	Previous        string              `yaml:"previous,omitempty"`
	AppliedEnvKeys  map[string][]string `yaml:"appliedEnvKeys,omitempty"`
}

// Config is the top-level aictx configuration.
type Config struct {
	State    State     `yaml:"state"`
	Contexts []Context `yaml:"contexts"`
}
