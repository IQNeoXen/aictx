package cmd

import (
	"fmt"
	"strings"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/spf13/cobra"
)

var showReveal bool

var showCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show context details (defaults to current)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  showRun,
}

func init() {
	showCmd.Flags().BoolVar(&showReveal, "reveal", false, "Show full secret values")
}

func showRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	name := cfg.State.Current
	if len(args) == 1 {
		name = args[0]
	}
	if name == "" {
		return fmt.Errorf("no context specified and no current context set")
	}

	ctx := cfg.FindContext(name)
	if ctx == nil {
		return fmt.Errorf("context %q not found", name)
	}

	fmt.Printf("Context: \033[1m%s\033[0m\n", ctx.Name)
	if ctx.Description != "" {
		fmt.Printf("Description: %s\n", ctx.Description)
	}

	// Provider
	prov := ctx.Provider
	if !prov.IsEmpty() {
		fmt.Println("\nProvider:")
		if prov.Endpoint != "" {
			fmt.Printf("  Endpoint    = %s\n", prov.Endpoint)
		}
		if prov.APIKey != "" {
			v := prov.APIKey
			if !showReveal {
				v = maskValue(v)
			}
			fmt.Printf("  API Key     = %s\n", v)
		}
		if prov.Model != "" {
			fmt.Printf("  Model       = %s\n", prov.Model)
		}
		if prov.SmallModel != "" {
			fmt.Printf("  Small Model = %s\n", prov.SmallModel)
		}
		if len(prov.Headers) > 0 {
			fmt.Println("  Headers:")
			for k, v := range prov.Headers {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	} else {
		fmt.Println("\nProvider: (native auth / OAuth)")
	}

	// Options
	fmt.Println("\nOptions:")
	if ctx.Options.AlwaysThinking != nil {
		fmt.Printf("  Always Thinking    = %v\n", *ctx.Options.AlwaysThinking)
	}
	if ctx.Options.DisableTelemetry != nil {
		fmt.Printf("  Disable Telemetry  = %v\n", *ctx.Options.DisableTelemetry)
	}
	if ctx.Options.DisableBetas != nil {
		fmt.Printf("  Disable Betas      = %v\n", *ctx.Options.DisableBetas)
	}

	// Targets
	fmt.Println("\nTargets:")
	for _, te := range ctx.Targets {
		overrides := []string{}
		if te.Overrides.Model != "" {
			overrides = append(overrides, fmt.Sprintf("model → %s", te.Overrides.Model))
		}
		if te.Overrides.Endpoint != "" {
			overrides = append(overrides, fmt.Sprintf("endpoint → %s", te.Overrides.Endpoint))
		}
		if te.Overrides.SmallModel != "" {
			overrides = append(overrides, fmt.Sprintf("smallModel → %s", te.Overrides.SmallModel))
		}
		if len(overrides) > 0 {
			fmt.Printf("  * %s  (%s)\n", te.ID, strings.Join(overrides, ", "))
		} else {
			fmt.Printf("  * %s\n", te.ID)
		}
	}

	return nil
}

func maskValue(v string) string {
	if len(v) <= 8 {
		return "***"
	}
	return v[:8] + "***"
}
