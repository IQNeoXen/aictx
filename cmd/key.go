package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage context API keys",
}

var keyUpdateCmd = &cobra.Command{
	Use:               "update [context]",
	Short:             "Replace a context's static API key",
	Args:              cobra.MaximumNArgs(1),
	RunE:              keyUpdateRun,
	ValidArgsFunction: contextCompletion,
}

// Narrow adapter makes the interactive, no-echo path testable without a PTY.
var saveKeyConfig = config.Save

var keyInput = struct {
	isTerminal   func(int) bool
	readPassword func(int) ([]byte, error)
}{term.IsTerminal, term.ReadPassword}

func init() {
	keyUpdateCmd.Flags().String("api-key", "", "Replacement API key (visible in shell history and process listings)")
	keyCmd.AddCommand(keyUpdateCmd)
}

func keyUpdateRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name := cfg.State.Current
	if len(args) == 1 {
		name = args[0]
	} else if name == "" {
		return fmt.Errorf("no current context set. Run 'aictx <context>' first or specify a context")
	}
	ctx := cfg.FindContext(name)
	if ctx == nil {
		return fmt.Errorf("context %q not found", name)
	}
	if ctx.HasOAuthKey {
		return fmt.Errorf("context %q uses OAuth credentials; API-key updates are not supported", name)
	}
	if ctx.Provider.ProviderType == "copilot" {
		return fmt.Errorf("context %q uses Copilot credentials; API-key updates are not supported", name)
	}
	// An existing static key is required: a custom endpoint alone can be
	// intentionally keyless (for example, a local model server).
	if !ctx.HasKeyringKey && ctx.Provider.APIKey == "" {
		return fmt.Errorf("context %q uses native/keyless authentication; API-key updates are not supported", name)
	}

	var replacement string
	if cmd.Flags().Changed("api-key") {
		replacement, err = cmd.Flags().GetString("api-key")
		if err != nil {
			return err
		}
	} else {
		fd := int(os.Stdin.Fd())
		if !keyInput.isTerminal(fd) {
			return fmt.Errorf("interactive API-key entry requires a terminal; use --api-key for scripting")
		}
		fmt.Printf("New API key for %s: ", ctx.Name)
		password, readErr := keyInput.readPassword(fd)
		fmt.Println()
		if readErr != nil {
			return fmt.Errorf("reading API key: %w", readErr)
		}
		replacement = string(password)
	}
	replacement = strings.TrimSpace(replacement)
	if replacement == "" {
		if cmd.Flags().Changed("api-key") {
			return fmt.Errorf("--api-key must not be empty")
		}
		return fmt.Errorf("API key must not be empty")
	}

	ctx.Provider.APIKey = replacement
	if err := saveKeyConfig(cfg); err != nil {
		return fmt.Errorf("saving API key: %w", err)
	}
	fmt.Printf("✓ API key for %s updated in the OS keychain\n", ctx.Name)
	if ctx.Name != cfg.State.Current {
		fmt.Printf("  Context is not active; run `aictx %s` to apply it.\n", ctx.Name)
		return nil
	}

	result := applyContextTargets(cfg, ctx, ctx.Provider, false)
	// Save again when successful target application changes stale-env tracking.
	// The replacement is already persisted, so a subsequent failure must say so.
	if result.Applied > 0 {
		if err := saveKeyConfig(cfg); err != nil {
			return fmt.Errorf("API key saved, but recording applied target state failed: %w; retry `aictx %s`", err, ctx.Name)
		}
	}
	if len(result.Failed) > 0 {
		return fmt.Errorf("API key saved, but %d target(s) failed to apply (other targets may already be updated); retry `aictx %s`", len(result.Failed), ctx.Name)
	}
	if result.Applied == 0 {
		fmt.Printf("  No configured targets were detected; run `aictx %s` when installed.\n", ctx.Name)
	}
	return nil
}
