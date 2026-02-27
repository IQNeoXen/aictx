package cmd

import (
	"fmt"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/spf13/cobra"
)

var listNamesOnly bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all contexts",
	Args:  cobra.NoArgs,
	RunE:  listRun,
}

func init() {
	listCmd.Flags().BoolVar(&listNamesOnly, "names-only", false, "Print context names one per line")
}

func listRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts configured. Use 'aictx add <name>' to get started.")
		return nil
	}
	for _, c := range cfg.Contexts {
		if listNamesOnly {
			fmt.Println(c.Name)
		} else {
			marker := "  "
			if c.Name == cfg.State.Current {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, c.Name)
		}
	}
	return nil
}
