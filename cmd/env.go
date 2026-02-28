package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env <context> <target>",
	Short: "Manage custom env vars for a target in a context",
	Args:  cobra.ExactArgs(2),
	RunE:  envRun,
}

func envRun(cmd *cobra.Command, args []string) error {
	ctxName := args[0]
	targetID := args[1]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := cfg.FindContext(ctxName)
	if ctx == nil {
		return fmt.Errorf("context %q not found", ctxName)
	}

	te := ctx.GetTarget(targetID)
	if te == nil {
		return fmt.Errorf("target %q not found in context %q", targetID, ctxName)
	}

	scanner := bufio.NewScanner(os.Stdin)

	for {
		// List current vars
		fmt.Printf("\nCustom env vars for %s / %s:\n", ctxName, targetID)
		if len(te.Env) == 0 {
			fmt.Println("  (none)")
		} else {
			keys := make([]string, 0, len(te.Env))
			for k := range te.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %s=%s\n", k, te.Env[k])
			}
		}

		fmt.Println("\nActions: [a]dd  [r]emove  [q]uit")
		fmt.Print("Action: ")
		if !scanner.Scan() {
			break
		}
		action := strings.TrimSpace(strings.ToLower(scanner.Text()))

		switch action {
		case "a", "add":
			key := prompt(scanner, "  Name")
			if key == "" {
				fmt.Println("  Name cannot be empty.")
				continue
			}
			value := prompt(scanner, "  Value")
			if te.Env == nil {
				te.Env = map[string]string{}
			}
			te.Env[key] = value
			fmt.Printf("  Set %s.\n", key)

		case "r", "remove":
			key := prompt(scanner, "  Name to remove")
			if key == "" {
				fmt.Println("  Name cannot be empty.")
				continue
			}
			if _, ok := te.Env[key]; !ok {
				fmt.Printf("  %q not found.\n", key)
				continue
			}
			delete(te.Env, key)
			if len(te.Env) == 0 {
				te.Env = nil
			}
			fmt.Printf("  Removed %s.\n", key)

		case "q", "quit", "":
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("Saved.")
			return nil

		default:
			fmt.Println("  Unknown action.")
		}
	}

	return config.Save(cfg)
}
