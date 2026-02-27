package target

import (
	"github.com/fschneidewind/aictx/internal/target/claudecli"
	"github.com/fschneidewind/aictx/internal/target/claudevscode"
)

// All returns all known targets.
func All() []Target {
	return []Target{
		claudecli.New(),
		claudevscode.New(),
	}
}

// ByID returns the target with the given ID, or nil.
func ByID(id string) Target {
	for _, t := range All() {
		if t.ID() == id {
			return t
		}
	}
	return nil
}

// IDs returns the IDs of all known targets.
func IDs() []string {
	all := All()
	ids := make([]string, len(all))
	for i, t := range all {
		ids[i] = t.ID()
	}
	return ids
}
