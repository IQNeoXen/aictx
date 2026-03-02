package cmd

import (
	"fmt"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/IQNeoXen/aictx/internal/target"
	"github.com/spf13/cobra"
)

var (
	copyTargets     []string
	copyDesc        string
	copyEndpoint    string
	copyAPIKey      string
	copyModel       string
	copySmallModel  string
	copyThinking    bool
	copyNoTelemetry bool
	copyNoBetas     bool
	copyEnv         []string
	copyHeaders     []string
)

var copyCmd = &cobra.Command{
	Use:   "copy <source> <new-name>",
	Short: "Copy a context, optionally overriding settings",
	Long: `Copy an existing context to a new name. All settings are inherited from
the source context. Use flags to override individual fields in the copy.

Only explicitly provided flags override the source — everything else is
preserved as-is. --env and --header flags merge into the inherited values.

Examples:
  aictx copy falcon another-context --api-key sk-xxx
  aictx copy falcon staging --endpoint https://staging.api.example.com --model claude-haiku-4-5
  aictx copy prod dev --no-telemetry --env DEBUG=1`,
	Args:              cobra.ExactArgs(2),
	RunE:              copyRun,
	ValidArgsFunction: copyCompletion,
}

func init() {
	copyCmd.Flags().StringArrayVar(&copyTargets, "target", nil, "Restrict overrides to these target IDs (repeatable); default: all targets")
	copyCmd.Flags().StringVar(&copyDesc, "description", "", "Override description")
	copyCmd.Flags().StringVar(&copyEndpoint, "endpoint", "", "Override provider endpoint URL")
	copyCmd.Flags().StringVar(&copyAPIKey, "api-key", "", "Override provider API key")
	copyCmd.Flags().StringVar(&copyModel, "model", "", "Override model (e.g. claude-opus-4.6)")
	copyCmd.Flags().StringVar(&copySmallModel, "small-model", "", "Override small/cheap model (e.g. claude-haiku-4.5)")
	copyCmd.Flags().BoolVar(&copyThinking, "thinking", false, "Enable always thinking")
	copyCmd.Flags().BoolVar(&copyNoTelemetry, "no-telemetry", false, "Disable telemetry")
	copyCmd.Flags().BoolVar(&copyNoBetas, "no-betas", false, "Disable experimental betas")
	copyCmd.Flags().StringArrayVar(&copyEnv, "env", nil, "Merge env variable as KEY=VALUE (repeatable)")
	copyCmd.Flags().StringArrayVar(&copyHeaders, "header", nil, "Merge custom HTTP header as Key:Value (repeatable)")
}

func copyRun(cmd *cobra.Command, args []string) error {
	srcName := args[0]
	dstName := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	src := cfg.FindContext(srcName)
	if src == nil {
		return fmt.Errorf("source context %q not found", srcName)
	}
	if cfg.FindContext(dstName) != nil {
		return fmt.Errorf("context %q already exists", dstName)
	}

	// Validate --target overrides
	for _, tid := range copyTargets {
		if target.ByID(tid) == nil {
			return fmt.Errorf("unknown target %q. Available: %v", tid, target.IDs())
		}
	}

	// Parse --env overrides
	envOverrides := map[string]string{}
	for _, e := range copyEnv {
		idx := strings.Index(e, "=")
		if idx <= 0 {
			return fmt.Errorf("invalid --env value %q: expected KEY=VALUE", e)
		}
		envOverrides[e[:idx]] = e[idx+1:]
	}

	// Parse --header overrides
	headerOverrides := map[string]string{}
	for _, h := range copyHeaders {
		idx := strings.Index(h, ":")
		if idx <= 0 {
			return fmt.Errorf("invalid --header value %q: expected Key:Value", h)
		}
		headerOverrides[h[:idx]] = h[idx+1:]
	}

	// Deep copy the source context
	dst := deepCopyContext(*src)
	dst.Name = dstName

	if cmd.Flags().Changed("description") {
		dst.Description = copyDesc
	}

	// Apply flag overrides to each target entry
	for i := range dst.Targets {
		te := &dst.Targets[i]

		// Skip targets not in the --target filter (if any filter specified)
		if len(copyTargets) > 0 && !containsString(copyTargets, te.ID) {
			continue
		}

		if cmd.Flags().Changed("endpoint") {
			te.Provider.Endpoint = copyEndpoint
		}
		if cmd.Flags().Changed("api-key") {
			te.Provider.APIKey = copyAPIKey
			te.HasKeyringKey = false // will be set by Save
		}
		if cmd.Flags().Changed("model") {
			te.Provider.Model = copyModel
		}
		if cmd.Flags().Changed("small-model") {
			te.Provider.SmallModel = copySmallModel
		}
		if cmd.Flags().Changed("thinking") {
			b := copyThinking
			te.Options.AlwaysThinking = &b
		}
		if cmd.Flags().Changed("no-telemetry") {
			b := copyNoTelemetry
			te.Options.DisableTelemetry = &b
		}
		if cmd.Flags().Changed("no-betas") {
			b := copyNoBetas
			te.Options.DisableBetas = &b
		}

		// Merge env overrides
		if len(envOverrides) > 0 {
			if te.Env == nil {
				te.Env = map[string]string{}
			}
			for k, v := range envOverrides {
				te.Env[k] = v
			}
		}

		// Merge header overrides
		if len(headerOverrides) > 0 {
			if te.Provider.Headers == nil {
				te.Provider.Headers = map[string]string{}
			}
			for k, v := range headerOverrides {
				te.Provider.Headers[k] = v
			}
		}
	}

	cfg.Contexts = append(cfg.Contexts, dst)
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Context \033[1m%s\033[0m copied from \033[1m%s\033[0m.\n", dstName, srcName)
	return nil
}

func deepCopyContext(src config.Context) config.Context {
	dst := config.Context{
		Name:        src.Name,
		Description: src.Description,
		Targets:     make([]config.TargetEntry, len(src.Targets)),
	}
	for i, te := range src.Targets {
		dst.Targets[i] = deepCopyTargetEntry(te)
	}
	return dst
}

func deepCopyTargetEntry(src config.TargetEntry) config.TargetEntry {
	dst := config.TargetEntry{
		ID:            src.ID,
		HasKeyringKey: src.HasKeyringKey,
		Provider: config.Provider{
			Endpoint:   src.Provider.Endpoint,
			APIKey:     src.Provider.APIKey,
			Model:      src.Provider.Model,
			SmallModel: src.Provider.SmallModel,
		},
		Options: config.Options{},
	}

	// Deep copy pointer-to-bool options
	if src.Options.AlwaysThinking != nil {
		b := *src.Options.AlwaysThinking
		dst.Options.AlwaysThinking = &b
	}
	if src.Options.DisableTelemetry != nil {
		b := *src.Options.DisableTelemetry
		dst.Options.DisableTelemetry = &b
	}
	if src.Options.DisableBetas != nil {
		b := *src.Options.DisableBetas
		dst.Options.DisableBetas = &b
	}

	// Deep copy maps
	if len(src.Provider.Headers) > 0 {
		dst.Provider.Headers = make(map[string]string, len(src.Provider.Headers))
		for k, v := range src.Provider.Headers {
			dst.Provider.Headers[k] = v
		}
	}
	if len(src.Env) > 0 {
		dst.Env = make(map[string]string, len(src.Env))
		for k, v := range src.Env {
			dst.Env[k] = v
		}
	}

	return dst
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func copyCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, c := range cfg.Contexts {
		names = append(names, c.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
