package cmd

import (
	"fmt"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/spf13/cobra"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the current context name",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.State.Current == "" {
			fmt.Println("(none)")
		} else {
			fmt.Println(cfg.State.Current)
		}
		return nil
	},
}
