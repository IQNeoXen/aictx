package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/IQNeoXen/aictx/internal/models"
	"github.com/IQNeoXen/aictx/internal/picker"
	"github.com/IQNeoXen/aictx/internal/target"
	"github.com/IQNeoXen/aictx/internal/target/picli"
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:           "model",
	Short:         "Select the primary model for the active context",
	Long:          "Fetch the available models from the active context's endpoint, pick one interactively, persist it, and regenerate the pi extension.",
	Args:          cobra.NoArgs,
	RunE:          modelRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func modelRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if cfg.State.Current == "" {
		return fmt.Errorf("no current context set. Run 'aictx <context>' first")
	}
	ctx := cfg.FindContext(cfg.State.Current)
	if ctx == nil {
		return fmt.Errorf("no current context set. Run 'aictx <context>' first")
	}

	// Guard: Copilot contexts are managed via 'aictx copilot login'.
	if ctx.Provider.ProviderType == "copilot" {
		return fmt.Errorf("'aictx model' is not supported for Copilot contexts (managed via 'aictx copilot login')")
	}

	// Guard: no endpoint means native/OAuth auth — nothing to query.
	if ctx.Provider.Endpoint == "" {
		return fmt.Errorf("context %s has no provider endpoint to query for models", ctx.Name)
	}

	fmt.Printf("Fetching models from %s ...\n", ctx.Provider.Endpoint)
	ids, err := models.FetchModelIDs(ctx.Provider.Endpoint, ctx.Provider.APIKey, ctx.Provider.Headers)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}
	if len(ids) == 0 {
		return fmt.Errorf("no models returned by %s", ctx.Provider.Endpoint)
	}

	var selected string
	if picker.IsTerminal() {
		fmt.Printf("Select model for context %s:\n", ctx.Name)
		sel, err := picker.Pick(ids, ctx.Provider.Model)
		if err != nil {
			return err
		}
		if sel == "" {
			fmt.Println("Aborted.")
			return nil
		}
		selected = sel
	} else {
		fmt.Printf("Available models: %s\n", strings.Join(ids, ", "))
		fmt.Printf("Select model [%s]: ", ctx.Provider.Model)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			selected = strings.TrimSpace(scanner.Text())
		}
		if selected == "" {
			selected = ctx.Provider.Model
		}
	}

	if selected == "" {
		fmt.Println("Aborted.")
		return nil
	}

	ctx.Provider.Model = selected
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("✓ Model for %s set to %s\n", ctx.Name, selected)

	// Regenerate the pi extension immediately so the new default takes effect.
	applied, err := applyPiTarget(cfg, ctx)
	if err != nil {
		return fmt.Errorf("regenerating pi extension: %w", err)
	}
	if applied {
		fmt.Println("✓ pi Coding Agent CLI extension regenerated")
	} else {
		fmt.Println("  (pi Coding Agent CLI not in this context or not installed — extension not regenerated)")
	}

	return nil
}

// applyPiTarget builds the effective TargetEntry for the pi-cli target (merging
// context-level Provider/Options with per-target Env) and applies it, mirroring
// the effective-entry construction in switchContext. Returns true when the
// extension was regenerated. Copilot contexts are rejected before this is called,
// so only the plain provider path runs.
func applyPiTarget(cfg *config.Config, ctx *config.Context) (bool, error) {
	te := ctx.GetTarget(picli.ID)
	if te == nil {
		return false, nil
	}

	t := target.ByID(picli.ID)
	if t == nil || !t.Detect() {
		return false, nil
	}

	effective := config.TargetEntry{
		ID:            te.ID,
		Provider:      ctx.Provider,
		Options:       ctx.Options,
		HasKeyringKey: ctx.HasKeyringKey,
		Env:           te.Env,
	}
	if err := t.Apply(effective); err != nil {
		return false, err
	}
	return true, nil
}
