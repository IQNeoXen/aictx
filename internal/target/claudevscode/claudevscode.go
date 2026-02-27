package claudevscode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const ID = "claude-code-vscode"

var trailingCommaRe = regexp.MustCompile(`,(\s*[\]}])`)

// Target implements the VSCode Claude Code extension target.
type Target struct{}

func New() *Target { return &Target{} }

func (t *Target) ID() string   { return ID }
func (t *Target) Name() string { return "Claude Code for VSCode" }

func (t *Target) settingsPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
	case "linux":
		return filepath.Join(home, ".config", "Code", "User", "settings.json")
	default: // windows
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "settings.json")
	}
}

func (t *Target) Detect() bool {
	_, err := os.Stat(t.settingsPath())
	return err == nil
}

// readSettings reads the VSCode settings file and strips trailing commas.
func (t *Target) readSettings() ([]byte, error) {
	data, err := os.ReadFile(t.settingsPath())
	if err != nil {
		return nil, err
	}
	return trailingCommaRe.ReplaceAll(data, []byte("$1")), nil
}

// writeSettings writes the settings file atomically.
func (t *Target) writeSettings(data []byte) error {
	path := t.settingsPath()
	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (t *Target) Apply(ctx config.Context) error {
	data, err := t.readSettings()
	if err != nil {
		return fmt.Errorf("reading vscode settings: %w", err)
	}

	prov := ctx.EffectiveProvider(ID)

	// Build env vars from abstract provider + options
	env := make(map[string]string)
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
	if ctx.Options.DisableTelemetry != nil && *ctx.Options.DisableTelemetry {
		env["DISABLE_TELEMETRY"] = "1"
	}
	if ctx.Options.DisableBetas != nil && *ctx.Options.DisableBetas {
		env["CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS"] = "1"
	}

	// Set or remove claudeCode.environmentVariables
	envVars := buildEnvVarsArray(env)
	if len(envVars) > 0 {
		data, err = sjson.SetBytes(data, "claudeCode\\.environmentVariables", envVars)
		if err != nil {
			return fmt.Errorf("setting env vars: %w", err)
		}
	} else {
		data, err = sjson.DeleteBytes(data, "claudeCode\\.environmentVariables")
		if err != nil {
			return fmt.Errorf("deleting env vars: %w", err)
		}
	}

	// Set or remove claudeCode.selectedModel
	if prov.Model != "" {
		data, err = sjson.SetBytes(data, "claudeCode\\.selectedModel", prov.Model)
		if err != nil {
			return fmt.Errorf("setting model: %w", err)
		}
	} else {
		data, err = sjson.DeleteBytes(data, "claudeCode\\.selectedModel")
		if err != nil {
			return fmt.Errorf("deleting model: %w", err)
		}
	}

	return t.writeSettings(data)
}

func (t *Target) Discover() (*config.Context, error) {
	data, err := t.readSettings()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading vscode settings: %w", err)
	}

	ctx := &config.Context{
		Targets: []config.TargetEntry{{ID: ID}},
	}

	// Reverse-map VSCode env vars → abstract provider
	envArr := gjson.GetBytes(data, `claudeCode\.environmentVariables`)
	if envArr.Exists() && envArr.IsArray() {
		envArr.ForEach(func(_, value gjson.Result) bool {
			name := value.Get("name").String()
			val := value.Get("value").String()
			switch name {
			case "ANTHROPIC_BASE_URL":
				ctx.Provider.Endpoint = val
			case "ANTHROPIC_AUTH_TOKEN":
				ctx.Provider.APIKey = val
			case "ANTHROPIC_MODEL":
				ctx.Provider.Model = val
			case "ANTHROPIC_DEFAULT_HAIKU_MODEL":
				ctx.Provider.SmallModel = val
			case "DISABLE_TELEMETRY":
				if val == "1" {
					t := true
					ctx.Options.DisableTelemetry = &t
				}
			case "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS":
				if val == "1" {
					t := true
					ctx.Options.DisableBetas = &t
				}
			}
			return true
		})
	}

	// Extract selected model (may differ from env var model)
	model := gjson.GetBytes(data, `claudeCode\.selectedModel`)
	if model.Exists() && ctx.Provider.Model == "" {
		ctx.Provider.Model = model.String()
	}

	return ctx, nil
}

type envVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func buildEnvVarsArray(env map[string]string) []envVar {
	if len(env) == 0 {
		return nil
	}
	vars := make([]envVar, 0, len(env))
	for k, v := range env {
		vars = append(vars, envVar{Name: k, Value: v})
	}
	sort.Slice(vars, func(i, j int) bool {
		return vars[i].Name < vars[j].Name
	})
	return vars
}
