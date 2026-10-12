package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/IQNeoXen/aictx/internal/config"
	"github.com/IQNeoXen/aictx/internal/copilot"
	"github.com/IQNeoXen/aictx/internal/keyring"
	"github.com/IQNeoXen/aictx/internal/picker"
	"github.com/IQNeoXen/aictx/internal/target"
	"github.com/IQNeoXen/aictx/internal/target/picli"
	"github.com/spf13/cobra"
)

var copilotCmd = &cobra.Command{
	Use:           "copilot",
	Short:         "Switch to the 'copilot' context, or manage GitHub Copilot authentication",
	RunE:          copilotRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func copilotRun(cmd *cobra.Command, args []string) error {
	// When called with no subcommand, behave like `aictx copilot` —
	// i.e. switch to a context named "copilot", just like any other context name.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.FindContext("copilot") == nil {
		// No context named "copilot" — show the subcommand help instead.
		return cmd.Help()
	}
	return switchContext(cfg, "copilot")
}

var copilotLoginCmd = &cobra.Command{
	Use:           "login",
	Short:         "Authenticate with GitHub Copilot via Device Flow",
	RunE:          copilotLoginRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var copilotStatusCmd = &cobra.Command{
	Use:           "status",
	Short:         "Show GitHub Copilot login status",
	RunE:          copilotStatusRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var copilotLogoutCmd = &cobra.Command{
	Use:           "logout",
	Short:         "Remove stored GitHub Copilot credentials",
	RunE:          copilotLogoutRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	copilotCmd.AddCommand(copilotLoginCmd)
	copilotCmd.AddCommand(copilotStatusCmd)
	copilotCmd.AddCommand(copilotLogoutCmd)
	copilotCmd.AddCommand(copilotRefreshCmd)
}

// copilotRefreshCmd is a hidden subcommand used internally by the pi extension.
// It exchanges the stored OAuth token for a fresh Copilot API token and prints
// JSON to stdout: {"token": "...", "expiresAt": <unix_ms>}
var copilotRefreshCmd = &cobra.Command{
	Use:           "refresh",
	Short:         "Print a fresh Copilot API token as JSON (used by the pi extension)",
	Hidden:        true,
	RunE:          copilotRefreshRun,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func copilotRefreshRun(cmd *cobra.Command, args []string) error {
	oauthToken, err := keyring.GetCopilotOAuth()
	if err != nil {
		return fmt.Errorf("not logged in — run 'aictx copilot login' first")
	}
	ct, err := copilot.ExchangeToken(oauthToken)
	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expiresAt"` // Unix milliseconds (compatible with JS Date.now())
	}{
		Token:     ct.Token,
		ExpiresAt: ct.ExpiresAt.UnixMilli(),
	})
}

func copilotLoginRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Check if already logged in.
	if keyring.IsCopilotLoggedIn() {
		username := cfg.CopilotLogin.Username
		if username == "" {
			username = "unknown"
		}
		fmt.Printf("Already logged in as @%s. Re-login? (y/N): ", username)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}
	}

	fmt.Println("Logging in to GitHub Copilot...")

	// Device flow authentication.
	oauthToken, err := copilot.RunDeviceFlow(cmd.Context())
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	fmt.Printf("✓ Authorized as @%s\n", oauthToken.Username)

	// Verify Copilot subscription by exchanging for an API token.
	fmt.Println("Verifying Copilot subscription...")
	ct, err := copilot.ExchangeToken(oauthToken.Token)
	if err != nil {
		if err == copilot.ErrNoCopilotSubscription {
			return fmt.Errorf("your GitHub account does not have an active Copilot subscription")
		}
		return fmt.Errorf("verifying subscription: %w", err)
	}

	// Fetch available models.
	fmt.Println("Fetching available models...")
	models, err := copilot.ListModels(ct.Token)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}

	modelIDs := make([]string, len(models))
	for i, m := range models {
		modelIDs[i] = m.ID
	}

	// Pick a sensible default: prefer Claude Sonnet, fall back to gpt-4o, then first.
	defaultModel := modelIDs[0]
	for _, prefer := range []string{"claude-sonnet-4-6", "claude-sonnet-4-5", "claude-3.7-sonnet", "gpt-4o"} {
		for _, id := range modelIDs {
			if id == prefer {
				defaultModel = id
				goto foundDefault
			}
		}
	}
foundDefault:

	// Model selection.
	var selectedModel string
	if picker.IsTerminal() {
		selected, err := picker.Pick(modelIDs, defaultModel, picker.WithTitle("Select GitHub Copilot Model"))
		if err != nil {
			return err
		}
		if selected == "" {
			fmt.Println("Aborted.")
			return nil
		}
		selectedModel = selected
	} else {
		fmt.Printf("Select default model (available: %s) [%s]: ", strings.Join(modelIDs, ", "), defaultModel)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			selectedModel = strings.TrimSpace(scanner.Text())
		}
		if selectedModel == "" {
			selectedModel = defaultModel
		}
	}

	// Small model selection (optional).
	var selectedSmallModel string
	smallOptions := append([]string{"(none)"}, modelIDs...)
	if picker.IsTerminal() {
		selected, err := picker.Pick(smallOptions, "(none)", picker.WithTitle("Select Small Model (Optional)"))
		if err != nil {
			return err
		}
		if selected != "" && selected != "(none)" {
			selectedSmallModel = selected
		}
	} else {
		fmt.Printf("Select small model (leave empty to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			selectedSmallModel = strings.TrimSpace(scanner.Text())
		}
	}

	// Context name.
	defaultCtxName := "github-copilot"
	var ctxName string
	fmt.Printf("Context name [%s]: ", defaultCtxName)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ctxName = strings.TrimSpace(scanner.Text())
	}
	if ctxName == "" {
		ctxName = defaultCtxName
	}

	// Copilot only supports pi-cli (OpenAI-compatible API).
	// Auto-select it; no picker needed.
	piTarget := target.ByID(picli.ID)
	if piTarget == nil || !piTarget.Detect() {
		return fmt.Errorf("pi Coding Agent CLI not found — install pi first (https://github.com/mariozechner/pi)")
	}
	selectedTargets := []string{picli.ID}
	fmt.Printf("  ✓ %s (auto-selected, only supported target)\n", piTarget.Name())

	// Build context.
	ctx := config.Context{
		Name: ctxName,
		Provider: config.Provider{
			Endpoint:     copilot.CopilotAPIEndpoint,
			Model:        selectedModel,
			SmallModel:   selectedSmallModel,
			ProviderType: "copilot",
		},
	}
	for _, tid := range selectedTargets {
		ctx.Targets = append(ctx.Targets, config.TargetEntry{ID: tid})
	}

	// Remove existing context with the same name if present.
	cfg.RemoveContext(ctxName)
	cfg.Contexts = append(cfg.Contexts, ctx)

	// Store OAuth token in keychain.
	if err := keyring.SetCopilotOAuth(oauthToken.Token); err != nil {
		return fmt.Errorf("storing OAuth token: %w", err)
	}

	// Persist login metadata.
	cfg.CopilotLogin = config.CopilotLogin{
		Username:   oauthToken.Username,
		LoggedInAt: time.Now(),
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Switch to the new context (this also exchanges the token and applies to targets).
	fmt.Printf("✓ Context %s saved and switching...\n", ctxName)
	return switchContext(cfg, ctxName)
}

func copilotStatusRun(cmd *cobra.Command, args []string) error {
	if !keyring.IsCopilotLoggedIn() {
		fmt.Println("Not logged in. Run 'aictx copilot login' to authenticate.")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("GitHub Copilot")
	if cfg.CopilotLogin.Username != "" {
		fmt.Printf("  Logged in as:  @%s\n", cfg.CopilotLogin.Username)
	}
	if !cfg.CopilotLogin.LoggedInAt.IsZero() {
		fmt.Printf("  Logged in at:  %s\n", cfg.CopilotLogin.LoggedInAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Println("  OAuth token:   present (stored in keychain)")
	fmt.Println("  Token TTL:     ~30 min (refreshed on every 'aictx <context>' invocation)")

	// List Copilot contexts.
	var copilotContexts []string
	for _, ctx := range cfg.Contexts {
		if ctx.Provider.ProviderType == "copilot" {
			copilotContexts = append(copilotContexts, ctx.Name)
		}
	}
	if len(copilotContexts) > 0 {
		fmt.Printf("  Contexts:      %s\n", strings.Join(copilotContexts, ", "))
	}
	if cfg.State.Current != "" {
		for _, name := range copilotContexts {
			if name == cfg.State.Current {
				fmt.Printf("  Active:        %s\n", cfg.State.Current)
				break
			}
		}
	}

	return nil
}

func copilotLogoutRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !keyring.IsCopilotLoggedIn() {
		fmt.Println("Not logged in.")
		return nil
	}

	// Warn if current context uses Copilot.
	if cfg.State.Current != "" {
		ctx := cfg.FindContext(cfg.State.Current)
		if ctx != nil && ctx.Provider.ProviderType == "copilot" {
			fmt.Fprintf(os.Stderr, "Warning: active context %q uses Copilot — switch to another context before using AI tools.\n", cfg.State.Current)
		}
	}

	if err := keyring.DeleteCopilotOAuth(); err != nil {
		return fmt.Errorf("removing OAuth token: %w", err)
	}

	cfg.CopilotLogin = config.CopilotLogin{}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Logged out of GitHub Copilot.")
	return nil
}
