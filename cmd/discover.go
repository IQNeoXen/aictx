package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fschneidewind/aictx/internal/config"
	"github.com/fschneidewind/aictx/internal/target"
	"github.com/spf13/cobra"
)

var discoverName string

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Detect current config from installed tools and save as a context",
	Args:  cobra.NoArgs,
	RunE:  discoverRun,
}

func init() {
	discoverCmd.Flags().StringVar(&discoverName, "name", "", "Context name (prompts if not provided)")
}

func discoverRun(cmd *cobra.Command, args []string) error {
	fmt.Println("Scanning targets...")

	merged := &config.Context{}
	var targets []config.TargetEntry

	found := false
	for _, t := range target.All() {
		if !t.Detect() {
			fmt.Printf("  [--] %s: not found\n", t.Name())
			continue
		}

		discovered, err := t.Discover()
		if err != nil {
			fmt.Printf("  [!!] %s: %v\n", t.Name(), err)
			continue
		}
		if discovered == nil {
			fmt.Printf("  [--] %s: no config found\n", t.Name())
			continue
		}

		fmt.Printf("  [OK] %s\n", t.Name())
		found = true
		targets = append(targets, config.TargetEntry{ID: t.ID()})

		// Merge provider (first target wins on conflicts)
		if merged.Provider.Endpoint == "" && discovered.Provider.Endpoint != "" {
			merged.Provider.Endpoint = discovered.Provider.Endpoint
		}
		if merged.Provider.APIKey == "" && discovered.Provider.APIKey != "" {
			merged.Provider.APIKey = discovered.Provider.APIKey
		}
		if merged.Provider.Model == "" && discovered.Provider.Model != "" {
			merged.Provider.Model = discovered.Provider.Model
		}
		if merged.Provider.SmallModel == "" && discovered.Provider.SmallModel != "" {
			merged.Provider.SmallModel = discovered.Provider.SmallModel
		}

		// Merge options
		if merged.Options.AlwaysThinking == nil && discovered.Options.AlwaysThinking != nil {
			merged.Options.AlwaysThinking = discovered.Options.AlwaysThinking
		}
		if merged.Options.DisableTelemetry == nil && discovered.Options.DisableTelemetry != nil {
			merged.Options.DisableTelemetry = discovered.Options.DisableTelemetry
		}
		if merged.Options.DisableBetas == nil && discovered.Options.DisableBetas != nil {
			merged.Options.DisableBetas = discovered.Options.DisableBetas
		}
	}

	if !found {
		return fmt.Errorf("no targets with existing config found")
	}

	merged.Targets = targets

	// Show what was discovered
	fmt.Println("\nDiscovered provider:")
	if merged.Provider.Endpoint != "" {
		fmt.Printf("  Endpoint    = %s\n", merged.Provider.Endpoint)
	}
	if merged.Provider.APIKey != "" {
		fmt.Printf("  API Key     = %s\n", maskValue(merged.Provider.APIKey))
	}
	if merged.Provider.Model != "" {
		fmt.Printf("  Model       = %s\n", merged.Provider.Model)
	}
	if merged.Provider.SmallModel != "" {
		fmt.Printf("  Small Model = %s\n", merged.Provider.SmallModel)
	}
	if merged.Provider.IsEmpty() {
		fmt.Println("  (native auth / OAuth)")
	}

	if merged.Options.AlwaysThinking != nil {
		fmt.Printf("\n  Always Thinking   = %v\n", *merged.Options.AlwaysThinking)
	}
	if merged.Options.DisableTelemetry != nil {
		fmt.Printf("  Disable Telemetry = %v\n", *merged.Options.DisableTelemetry)
	}
	if merged.Options.DisableBetas != nil {
		fmt.Printf("  Disable Betas     = %v\n", *merged.Options.DisableBetas)
	}

	fmt.Printf("\n  Targets: %v\n", merged.TargetIDs())

	// Get context name
	name := discoverName
	if name == "" {
		fmt.Print("\nSave as context [name]: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			name = strings.TrimSpace(scanner.Text())
		}
		if name == "" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	merged.Name = name

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if existing := cfg.FindContext(name); existing != nil {
		return fmt.Errorf("context %q already exists. Remove it first with 'aictx rm %s'", name, name)
	}

	cfg.Contexts = append(cfg.Contexts, *merged)
	cfg.State.Current = name

	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("\nContext \033[1m%s\033[0m saved and set as current.\n", name)
	return nil
}
