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

var (
	addTargets    []string
	addDesc       string
	addEndpoint   string
	addAPIKey     string
	addModel      string
	addSmallModel string
	addThinking   bool
	addNoTelemetry bool
	addNoBetas    bool
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new context",
	Args:  cobra.ExactArgs(1),
	RunE:  addRun,
}

func init() {
	addCmd.Flags().StringArrayVar(&addTargets, "target", nil, "Target to include (repeatable, e.g. claude-code-cli)")
	addCmd.Flags().StringVar(&addDesc, "description", "", "Context description")
	addCmd.Flags().StringVar(&addEndpoint, "endpoint", "", "Provider endpoint URL")
	addCmd.Flags().StringVar(&addAPIKey, "api-key", "", "Provider API key")
	addCmd.Flags().StringVar(&addModel, "model", "", "Model (e.g. claude-opus-4.6)")
	addCmd.Flags().StringVar(&addSmallModel, "small-model", "", "Small/cheap model (e.g. claude-haiku-4.5)")
	addCmd.Flags().BoolVar(&addThinking, "thinking", false, "Enable always thinking")
	addCmd.Flags().BoolVar(&addNoTelemetry, "no-telemetry", false, "Disable telemetry")
	addCmd.Flags().BoolVar(&addNoBetas, "no-betas", false, "Disable experimental betas")
}

func addRun(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if cfg.FindContext(name) != nil {
		return fmt.Errorf("context %q already exists", name)
	}

	ctx := config.Context{Name: name}

	// Check if any flags were provided for non-interactive mode
	flagsProvided := len(addTargets) > 0 || addDesc != "" || addEndpoint != "" ||
		addAPIKey != "" || addModel != "" || addSmallModel != "" ||
		addThinking || addNoTelemetry || addNoBetas

	if flagsProvided {
		ctx.Description = addDesc
		ctx.Provider.Endpoint = addEndpoint
		ctx.Provider.APIKey = addAPIKey
		ctx.Provider.Model = addModel
		ctx.Provider.SmallModel = addSmallModel

		if addThinking {
			t := true
			ctx.Options.AlwaysThinking = &t
		}
		if addNoTelemetry {
			t := true
			ctx.Options.DisableTelemetry = &t
		}
		if addNoBetas {
			t := true
			ctx.Options.DisableBetas = &t
		}

		for _, tid := range addTargets {
			if target.ByID(tid) == nil {
				return fmt.Errorf("unknown target %q. Available: %v", tid, target.IDs())
			}
			ctx.Targets = append(ctx.Targets, config.TargetEntry{ID: tid})
		}
	} else {
		// Interactive mode
		scanner := bufio.NewScanner(os.Stdin)

		ctx.Description = prompt(scanner, "Description")

		// Target selection
		fmt.Println("\nAvailable targets:")
		allTargets := target.All()
		for i, t := range allTargets {
			detected := ""
			if t.Detect() {
				detected = " (detected)"
			}
			fmt.Printf("  [%d] %s (%s)%s\n", i+1, t.Name(), t.ID(), detected)
		}
		fmt.Print("Select targets (comma-separated numbers, e.g. 1,2): ")
		if scanner.Scan() {
			parts := strings.Split(scanner.Text(), ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				idx := 0
				fmt.Sscanf(p, "%d", &idx)
				if idx >= 1 && idx <= len(allTargets) {
					ctx.Targets = append(ctx.Targets, config.TargetEntry{ID: allTargets[idx-1].ID()})
				}
			}
		}
		if len(ctx.Targets) == 0 {
			return fmt.Errorf("no targets selected")
		}

		// Provider
		fmt.Println("\nProvider settings (leave empty for native auth / OAuth):")
		ctx.Provider.Endpoint = prompt(scanner, "Endpoint URL")
		ctx.Provider.APIKey = prompt(scanner, "API Key")
		ctx.Provider.Model = prompt(scanner, "Model (e.g. claude-opus-4.6)")
		ctx.Provider.SmallModel = prompt(scanner, "Small model (e.g. claude-haiku-4.5)")

		// Headers
		fmt.Println("\nCustom headers (empty line to finish):")
		for {
			fmt.Print("  Header (key: value): ")
			if !scanner.Scan() {
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				break
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				fmt.Fprintln(os.Stderr, "  Invalid format, use key: value")
				continue
			}
			if ctx.Provider.Headers == nil {
				ctx.Provider.Headers = make(map[string]string)
			}
			ctx.Provider.Headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}

		// Options
		fmt.Println("\nOptions:")
		if yesNo(scanner, "Always thinking?", true) {
			t := true
			ctx.Options.AlwaysThinking = &t
		}
		if yesNo(scanner, "Disable telemetry?", true) {
			t := true
			ctx.Options.DisableTelemetry = &t
		}
		if yesNo(scanner, "Disable experimental betas?", false) {
			t := true
			ctx.Options.DisableBetas = &t
		}
	}

	cfg.Contexts = append(cfg.Contexts, ctx)
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Context \033[1m%s\033[0m added.\n", name)
	return nil
}

func prompt(scanner *bufio.Scanner, label string) string {
	fmt.Printf("  %s: ", label)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func yesNo(scanner *bufio.Scanner, question string, defaultYes bool) bool {
	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}
	fmt.Printf("  %s (%s): ", question, hint)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "" {
			return defaultYes
		}
		return answer == "y" || answer == "yes"
	}
	return defaultYes
}
