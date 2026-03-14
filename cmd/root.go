package cmd

import (
	"fmt"
	"os"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/IQNeoXen/aictx/internal/picker"
	"github.com/IQNeoXen/aictx/internal/target"
	"github.com/IQNeoXen/aictx/internal/target/claudecli"
	"github.com/IQNeoXen/aictx/internal/target/claudevscode"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "aictx [context]",
	Short:             "Switch AI tool contexts",
	Long:              "aictx is a context-switcher for AI tool configurations. Switch between API keys, models and other settings.",
	Args:              cobra.MaximumNArgs(1),
	RunE:              rootRun,
	SilenceErrors:     true,
	SilenceUsage:      true,
	ValidArgsFunction: contextCompletion,
}

func contextCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return cfg.ContextNames(), cobra.ShellCompDirectiveNoFileComp
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(copyCmd)
	rootCmd.AddCommand(renameCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(currentCmd)
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(completionCmd)
}

func rootRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// aictx <name> or aictx -
	if len(args) == 1 {
		return switchContext(cfg, args[0])
	}

	// No args: interactive picker or plain list
	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts configured. Use 'aictx add <name>' or 'aictx discover' to get started.")
		return nil
	}

	if picker.IsTerminal() {
		selected, err := picker.Pick(cfg.ContextNames(), cfg.State.Current)
		if err != nil {
			return err
		}
		if selected == "" {
			return nil
		}
		return switchContext(cfg, selected)
	}

	// Plain list (piped output)
	for _, c := range cfg.Contexts {
		if c.Name == cfg.State.Current {
			fmt.Printf("* %s\n", c.Name)
		} else {
			fmt.Printf("  %s\n", c.Name)
		}
	}
	return nil
}

func switchContext(cfg *config.Config, name string) error {
	if name == "-" {
		if cfg.State.Previous == "" {
			return fmt.Errorf("no previous context")
		}
		name = cfg.State.Previous
	}

	ctx := cfg.FindContext(name)
	if ctx == nil {
		return fmt.Errorf("context %q not found. Available: %v", name, cfg.ContextNames())
	}

	// Only apply to targets listed in this context
	applied := 0
	newAppliedEnvKeys := map[string][]string{}
	for _, te := range ctx.Targets {
		t := target.ByID(te.ID)
		if t == nil {
			fmt.Fprintf(os.Stderr, "  ? %s: unknown target\n", te.ID)
			continue
		}
		if !t.Detect() {
			fmt.Fprintf(os.Stderr, "  - %s: not installed\n", t.Name())
			continue
		}

		// Inject previously-applied env keys so Apply() can remove stale
		// entries before writing new ones.
		if cfg.State.AppliedEnvKeys != nil {
			if ct, ok := t.(*claudecli.Target); ok {
				ct.PrevEnvKeys = cfg.State.AppliedEnvKeys[te.ID]
			}
			if vt, ok := t.(*claudevscode.Target); ok {
				vt.PrevEnvKeys = cfg.State.AppliedEnvKeys[te.ID]
			}
		}

		if err := t.Apply(te); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", t.Name(), err)
			continue
		}
		applied++
		fmt.Printf("  ✓ %s\n", t.Name())

		// Track which env keys were applied for this target so the next
		// switch can clean them up.
		if te.ID == claudecli.ID || te.ID == claudevscode.ID {
			keys := targetAppliedEnvKeys(te)
			if len(keys) > 0 {
				newAppliedEnvKeys[te.ID] = keys
			}
		}
	}

	if applied == 0 {
		return fmt.Errorf("no targets could be applied")
	}

	cfg.State.Previous = cfg.State.Current
	cfg.State.Current = name
	if cfg.State.AppliedEnvKeys == nil {
		cfg.State.AppliedEnvKeys = map[string][]string{}
	}
	for k, v := range newAppliedEnvKeys {
		cfg.State.AppliedEnvKeys[k] = v
	}
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Switched to \033[1m%s\033[0m\n", name)
	return nil
}

// targetAppliedEnvKeys returns the set of env keys that Apply() will write
// for a TargetEntry. Used by both claudecli and claudevscode to track stale keys.
func targetAppliedEnvKeys(te config.TargetEntry) []string {
	var keys []string
	if te.Provider.Endpoint != "" {
		keys = append(keys, "ANTHROPIC_BASE_URL")
	}
	if te.Provider.APIKey != "" {
		keys = append(keys, "ANTHROPIC_AUTH_TOKEN")
	}
	if te.Provider.Model != "" {
		keys = append(keys, "ANTHROPIC_MODEL")
	}
	if te.Provider.SmallModel != "" {
		keys = append(keys, "ANTHROPIC_DEFAULT_HAIKU_MODEL")
	}
	if te.Options.DisableTelemetry != nil && *te.Options.DisableTelemetry {
		keys = append(keys, "DISABLE_TELEMETRY")
	}
	if te.Options.DisableBetas != nil && *te.Options.DisableBetas {
		keys = append(keys, "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS")
	}
	if len(te.Provider.Headers) > 0 {
		keys = append(keys, "ANTHROPIC_CUSTOM_HEADERS")
	}
	for k := range te.Env {
		keys = append(keys, k)
	}
	return keys
}
