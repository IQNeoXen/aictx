package claudecli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fschneidewind/aictx/internal/config"
)

const ID = "claude-code-cli"

// Target implements the Claude Code CLI target.
type Target struct{}

func New() *Target { return &Target{} }

func (t *Target) ID() string   { return ID }
func (t *Target) Name() string { return "Claude Code CLI" }

func (t *Target) settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func (t *Target) Detect() bool {
	_, err := os.Stat(filepath.Dir(t.settingsPath()))
	return err == nil
}

func (t *Target) Apply(ctx config.Context) error {
	prov := ctx.EffectiveProvider(ID)

	env := make(map[string]string)

	// Translate abstract provider → Claude Code env vars
	if prov.Endpoint != "" {
		env["ANTHROPIC_BASE_URL"] = prov.Endpoint
	}
	if prov.APIKey != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = prov.APIKey
	}
	if prov.Model != "" {
		env["ANTHROPIC_MODEL"] = prov.Model
	}
	if prov.SmallModel != "" {
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = prov.SmallModel
	}

	// Translate options → env vars
	if ctx.Options.DisableTelemetry != nil && *ctx.Options.DisableTelemetry {
		env["DISABLE_TELEMETRY"] = "1"
	}
	if ctx.Options.DisableBetas != nil && *ctx.Options.DisableBetas {
		env["CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS"] = "1"
	}

	// Build settings object
	settings := map[string]interface{}{}
	if len(env) > 0 {
		settings["env"] = env
	}
	if ctx.Options.AlwaysThinking != nil {
		settings["alwaysThinkingEnabled"] = *ctx.Options.AlwaysThinking
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling claude settings: %w", err)
	}
	data = append(data, '\n')

	path := t.settingsPath()
	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing claude settings: %w", err)
	}
	return os.Rename(tmp, path)
}

func (t *Target) Discover() (*config.Context, error) {
	data, err := os.ReadFile(t.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading claude settings: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing claude settings: %w", err)
	}

	ctx := &config.Context{
		Targets: []config.TargetEntry{{ID: ID}},
	}

	// Reverse-map env vars → abstract provider
	if envRaw, ok := raw["env"].(map[string]interface{}); ok {
		for k, v := range envRaw {
			s, ok := v.(string)
			if !ok {
				continue
			}
			switch k {
			case "ANTHROPIC_BASE_URL":
				ctx.Provider.Endpoint = s
			case "ANTHROPIC_AUTH_TOKEN":
				ctx.Provider.APIKey = s
			case "ANTHROPIC_MODEL":
				ctx.Provider.Model = s
			case "ANTHROPIC_DEFAULT_HAIKU_MODEL":
				ctx.Provider.SmallModel = s
			case "DISABLE_TELEMETRY":
				if s == "1" {
					t := true
					ctx.Options.DisableTelemetry = &t
				}
			case "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS":
				if s == "1" {
					t := true
					ctx.Options.DisableBetas = &t
				}
			}
		}
	}

	if thinking, ok := raw["alwaysThinkingEnabled"].(bool); ok {
		ctx.Options.AlwaysThinking = &thinking
	}

	return ctx, nil
}
