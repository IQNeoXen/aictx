package claudevscode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const ID = "claude-code-vscode"

var trailingCommaRe = regexp.MustCompile(`,(\s*[\]}])`)

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
	default:
		return filepath.Join(os.Getenv("APPDATA"), "Code", "User", "settings.json")
	}
}

func (t *Target) Detect() bool {
	_, err := os.Stat(t.settingsPath())
	return err == nil
}

func (t *Target) readSettings() ([]byte, error) {
	data, err := os.ReadFile(t.settingsPath())
	if err != nil {
		return nil, err
	}
	return trailingCommaRe.ReplaceAll(data, []byte("$1")), nil
}

func (t *Target) writeSettings(data []byte) error {
	path := t.settingsPath()
	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (t *Target) Apply(te config.TargetEntry) error {
	data, err := t.readSettings()
	if err != nil {
		return fmt.Errorf("reading vscode settings: %w", err)
	}

	env := make(map[string]string)
	if te.Provider.Endpoint != "" {
		env["ANTHROPIC_BASE_URL"] = te.Provider.Endpoint
	}
	if te.Provider.APIKey != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = te.Provider.APIKey
	}
	if te.Provider.Model != "" {
		env["ANTHROPIC_MODEL"] = te.Provider.Model
	}
	if te.Provider.SmallModel != "" {
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = te.Provider.SmallModel
	}
	if te.Options.DisableTelemetry != nil && *te.Options.DisableTelemetry {
		env["DISABLE_TELEMETRY"] = "1"
	}
	if te.Options.DisableBetas != nil && *te.Options.DisableBetas {
		env["CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS"] = "1"
	}
	// Merge custom env vars from the TargetEntry
	for k, v := range te.Env {
		env[k] = v
	}

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

	if te.Provider.Model != "" {
		data, err = sjson.SetBytes(data, "claudeCode\\.selectedModel", te.Provider.Model)
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

func (t *Target) Discover() (*config.TargetEntry, error) {
	data, err := t.readSettings()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading vscode settings: %w", err)
	}

	te := &config.TargetEntry{ID: ID}

	envArr := gjson.GetBytes(data, `claudeCode\.environmentVariables`)
	if envArr.Exists() && envArr.IsArray() {
		envArr.ForEach(func(_, value gjson.Result) bool {
			name := value.Get("name").String()
			val := value.Get("value").String()
			switch name {
			case "ANTHROPIC_BASE_URL":
				te.Provider.Endpoint = val
			case "ANTHROPIC_AUTH_TOKEN":
				te.Provider.APIKey = val
			case "ANTHROPIC_MODEL":
				te.Provider.Model = val
			case "ANTHROPIC_DEFAULT_HAIKU_MODEL":
				te.Provider.SmallModel = val
			case "DISABLE_TELEMETRY":
				if val == "1" {
					b := true
					te.Options.DisableTelemetry = &b
				}
			case "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS":
				if val == "1" {
					b := true
					te.Options.DisableBetas = &b
				}
			default:
				// Collect unrecognized entries as custom env vars
				if te.Env == nil {
					te.Env = map[string]string{}
				}
				te.Env[name] = val
			}
			return true
		})
	}

	model := gjson.GetBytes(data, `claudeCode\.selectedModel`)
	if model.Exists() && te.Provider.Model == "" {
		te.Provider.Model = model.String()
	}

	return te, nil
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
