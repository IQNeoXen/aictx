package cmd

import (
	"fmt"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:     "rm <name>",
	Aliases: []string{"remove", "delete"},
	Short:   "Remove a context",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if !cfg.RemoveContext(name) {
			return fmt.Errorf("context %q not found", name)
		}

		if cfg.State.Current == name {
			cfg.State.Current = ""
		}
		if cfg.State.Previous == name {
			cfg.State.Previous = ""
		}

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Context \033[1m%s\033[0m removed.\n", name)
		return nil
	},
}
