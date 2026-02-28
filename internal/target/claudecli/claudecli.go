package claudecli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/IQNeoXen/aictx/internal/config"
)

const ID = "claude-code-cli"

var trailingCommaRe = regexp.MustCompile(`,(\s*[\]}])`)

// Target implements the claudecli target.
// PrevEnvKeys holds the env keys written by the previous switch for this
// target so that Apply() can remove stale entries before writing new ones.
// Populate this field before calling Apply().
type Target struct {
	PrevEnvKeys []string
}

func New() *Target { return &Target{} }

func (t *Target) ID() string   { return ID }
func (t *Target) Name() string { return "Claude Code CLI" }

func (t *Target) claudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func (t *Target) settingsPath() string {
	return filepath.Join(t.claudeDir(), "settings.json")
}

func (t *Target) credentialsPath() string {
	return filepath.Join(t.claudeDir(), ".credentials.json")
}

func (t *Target) Detect() bool {
	// Installed if either credentials or settings exist
	if _, err := os.Stat(t.credentialsPath()); err == nil {
		return true
	}
	if _, err := os.Stat(t.settingsPath()); err == nil {
		return true
	}
	return false
}

func (t *Target) Apply(te config.TargetEntry) error {
	// --- 1. Read existing settings (read-modify-write) ---
	settings := map[string]interface{}{}

	path := t.settingsPath()
	if raw, err := os.ReadFile(path); err == nil {
		// Strip trailing commas before parsing (VSCode-style JSON may have them)
		cleaned := trailingCommaRe.ReplaceAll(raw, []byte("$1"))
		if jsonErr := json.Unmarshal(cleaned, &settings); jsonErr != nil {
			// If parsing fails, fall back to an empty map to avoid clobbering
			// a partially-corrupt file with our own error handling; we still
			// write out a clean merged result.
			settings = map[string]interface{}{}
		}
	}
	// If the file doesn't exist yet, settings stays empty — that's fine.

	// --- 2. Obtain the existing env sub-map ---
	existingEnv, _ := settings["env"].(map[string]interface{})
	if existingEnv == nil {
		existingEnv = map[string]interface{}{}
	}

	// --- 3. Remove keys that were written by the previous switch ---
	for _, k := range t.PrevEnvKeys {
		delete(existingEnv, k)
	}

	// --- 4. Build the new env entries from the TargetEntry ---
	newEnv := map[string]string{}
	if te.Provider.Endpoint != "" {
		newEnv["ANTHROPIC_BASE_URL"] = te.Provider.Endpoint
	}
	if te.Provider.APIKey != "" {
		newEnv["ANTHROPIC_AUTH_TOKEN"] = te.Provider.APIKey
	}
	if te.Provider.Model != "" {
		newEnv["ANTHROPIC_MODEL"] = te.Provider.Model
	}
	if te.Provider.SmallModel != "" {
		newEnv["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = te.Provider.SmallModel
	}
	if te.Options.DisableTelemetry != nil && *te.Options.DisableTelemetry {
		newEnv["DISABLE_TELEMETRY"] = "1"
	}
	if te.Options.DisableBetas != nil && *te.Options.DisableBetas {
		newEnv["CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS"] = "1"
	}
	// Merge custom env vars from the TargetEntry
	for k, v := range te.Env {
		newEnv[k] = v
	}

	// Merge new keys into the existing env map
	for k, v := range newEnv {
		existingEnv[k] = v
	}

	// --- 5. Write env back (or remove the key if nothing remains) ---
	if len(existingEnv) > 0 {
		settings["env"] = existingEnv
	} else {
		delete(settings, "env")
	}

	// --- 6. Handle alwaysThinkingEnabled ---
	if te.Options.AlwaysThinking != nil {
		settings["alwaysThinkingEnabled"] = *te.Options.AlwaysThinking
	}

	// --- 7. Marshal and atomic-write ---
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling claude settings: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing claude settings: %w", err)
	}
	return os.Rename(tmp, path)
}

func (t *Target) Discover() (*config.TargetEntry, error) {
	data, err := os.ReadFile(t.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			// No settings.json but target is detected (OAuth mode)
			return &config.TargetEntry{ID: ID}, nil
		}
		return nil, fmt.Errorf("reading claude settings: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing claude settings: %w", err)
	}

	te := &config.TargetEntry{ID: ID}

	if envRaw, ok := raw["env"].(map[string]interface{}); ok {
		for k, v := range envRaw {
			s, ok := v.(string)
			if !ok {
				continue
			}
			switch k {
			case "ANTHROPIC_BASE_URL":
				te.Provider.Endpoint = s
			case "ANTHROPIC_AUTH_TOKEN":
				te.Provider.APIKey = s
			case "ANTHROPIC_MODEL":
				te.Provider.Model = s
			case "ANTHROPIC_DEFAULT_HAIKU_MODEL":
				te.Provider.SmallModel = s
			case "DISABLE_TELEMETRY":
				if s == "1" {
					b := true
					te.Options.DisableTelemetry = &b
				}
			case "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS":
				if s == "1" {
					b := true
					te.Options.DisableBetas = &b
				}
			default:
				// Collect unrecognized keys as custom env vars
				if te.Env == nil {
					te.Env = map[string]string{}
				}
				te.Env[k] = s
			}
		}
	}

	if thinking, ok := raw["alwaysThinkingEnabled"].(bool); ok {
		te.Options.AlwaysThinking = &thinking
	}

	return te, nil
}
