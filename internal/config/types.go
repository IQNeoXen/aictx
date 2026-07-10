package config

import "time"

// CopilotLogin holds persisted metadata about the Copilot login session.
// Only non-sensitive fields are stored here; the OAuth token lives in the OS keychain.
type CopilotLogin struct {
	Username   string    `yaml:"username,omitempty"`
	LoggedInAt time.Time `yaml:"loggedInAt,omitempty"`
}

// Context represents a named AI tool configuration.
type Context struct {
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Command       string        `yaml:"command,omitempty"`
	Provider      Provider      `yaml:"provider,omitempty"`
	Options       Options       `yaml:"options,omitempty"`
	HasKeyringKey bool          `yaml:"hasKeyringKey,omitempty"`
	HasOAuthKey   bool          `yaml:"hasOAuthKey,omitempty"`
	Targets       []TargetEntry `yaml:"targets"`
}

// TargetEntry specifies a target included in a context.
// Provider, Options, and HasKeyringKey are context-level in the new format and will
// be empty here; they are kept on the struct so that Apply() callers can pass an
// effective merged entry without changing the Apply signature.
type TargetEntry struct {
	ID            string            `yaml:"id"`
	Provider      Provider          `yaml:"provider,omitempty"`
	Options       Options           `yaml:"options,omitempty"`
	HasKeyringKey bool              `yaml:"hasKeyringKey,omitempty"`
	Env           map[string]string `yaml:"env,omitempty"`
}

// DiscoveryResult holds the configuration discovered from a target's current settings.
type DiscoveryResult struct {
	ID       string
	Provider Provider
	Env      map[string]string
	IsOAuth  bool
}

// Provider holds abstract connection settings that each target translates
// into its own config format.
type Provider struct {
	Endpoint     string            `yaml:"endpoint,omitempty"`
	APIKey       string            `yaml:"apiKey,omitempty"`
	Model        string            `yaml:"model,omitempty"`
	SmallModel   string            `yaml:"smallModel,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty"`
	ProviderType string            `yaml:"providerType,omitempty"` // e.g. "anthropic" (default), "openai"

	// Name optionally overrides the derived provider name shown in tools
	// (e.g. pi's /model badge). Ignored for Copilot endpoints. Only takes
	// effect when an Endpoint is set.
	Name string `yaml:"name,omitempty"`
}

// Clone returns a deep copy of the Provider (the Headers map is copied).
func (p Provider) Clone() Provider {
	cp := p
	if p.Headers != nil {
		cp.Headers = make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			cp.Headers[k] = v
		}
	}
	return cp
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
	Current        string              `yaml:"current"`
	Previous       string              `yaml:"previous,omitempty"`
	AppliedEnvKeys map[string][]string `yaml:"appliedEnvKeys,omitempty"`
}

// Config is the top-level aictx configuration.
type Config struct {
	State        State        `yaml:"state"`
	CopilotLogin CopilotLogin `yaml:"copilotLogin,omitempty"`
	Contexts     []Context    `yaml:"contexts"`
}
