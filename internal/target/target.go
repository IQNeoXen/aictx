package target

import "github.com/fschneidewind/aictx/internal/config"

// Target represents a tool whose configuration is managed by aictx.
type Target interface {
	// ID returns the unique identifier for this target (e.g. "claude-code-cli").
	ID() string

	// Name returns the human-readable name of this target.
	Name() string

	// Detect returns true if this target is installed / config exists.
	Detect() bool

	// Apply translates abstract provider/options into this target's config format.
	Apply(ctx config.Context) error

	// Discover reads the target's current config and returns an abstract Context.
	// Returns nil if nothing useful is found.
	Discover() (*config.Context, error)
}
