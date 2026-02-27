package cmd

import (
	"fmt"
	"os"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/fschneidewind/aictx/internal/fzf"
	"github.com/fschneidewind/aictx/internal/target"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "aictx [context]",
	Short:         "Switch AI tool contexts",
	Long:          "aictx is a context-switcher for AI tool configurations. Switch between API keys, models and other settings.",
	Args:          cobra.MaximumNArgs(1),
	RunE:          rootRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(currentCmd)
	rootCmd.AddCommand(discoverCmd)
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

	// No args: list or fzf pick
	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts configured. Use 'aictx add <name>' or 'aictx discover' to get started.")
		return nil
	}

	if fzf.Available() && fzf.IsTerminal() {
		selected, err := fzf.Pick(cfg.ContextNames(), cfg.State.Current)
		if err != nil {
			return err
		}
		if selected == "" {
			return nil
		}
		return switchContext(cfg, selected)
	}

	// Plain list
	for _, c := range cfg.Contexts {
		if c.Name == cfg.State.Current {
			fmt.Printf("* \033[1m%s\033[0m\n", c.Name)
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
		if err := t.Apply(*ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", t.Name(), err)
			continue
		}
		applied++
		fmt.Printf("  ✓ %s\n", t.Name())
	}

	if applied == 0 {
		return fmt.Errorf("no targets could be applied")
	}

	cfg.State.Previous = cfg.State.Current
	cfg.State.Current = name
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Switched to \033[1m%s\033[0m\n", name)
	return nil
}
