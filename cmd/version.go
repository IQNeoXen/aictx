package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current aictx version.
const Version = "0.3.1"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("aictx v%s\n", Version)
	},
}
