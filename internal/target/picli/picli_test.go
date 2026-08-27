package picli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IQNeoXen/aictx/internal/config"
)

// setupPi creates a temp HOME with the .pi/agent directory and returns a fresh Target.
func setupPi(t *testing.T) *Target {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755); err != nil {
		t.Fatalf("MkdirAll .pi/agent: %v", err)
	}
	return New()
}

func readSettingsMap(t *testing.T, tgt *Target) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(tgt.settingsPath())
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return m
}

func readExtension(t *testing.T, tgt *Target) string {
	t.Helper()
	data, err := os.ReadFile(tgt.extensionPath())
	if err != nil {
		t.Fatalf("readExtension: %v", err)
	}
	return string(data)
}

// ---------- Detect ----------

func TestDetect_NotInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tgt := New()
	if tgt.Detect() {
		t.Error("Detect() = true, want false when .pi/agent dir missing")
	}
}

func TestDetect_Installed(t *testing.T) {
	tgt := setupPi(t)
	if !tgt.Detect() {
		t.Error("Detect() = false, want true when .pi/agent dir exists")
	}
}

// ---------- Apply ----------

func TestApply_BasicProvider(t *testing.T) {
	tgt := setupPi(t)

	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-sonnet-4-6",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	// Check extension file
	ext := readExtension(t, tgt)
	if !strings.Contains(ext, `"https://aikeys.maibornwolff.de/"`) {
		t.Error("extension missing baseUrl")
	}
	if !strings.Contains(ext, `"sk-test-key"`) {
		t.Error("extension missing apiKey")
	}
	if !strings.Contains(ext, "authHeader: true") {
		t.Error("extension missing authHeader for real apiKey")
	}
	// Provider name is derived from the endpoint hostname ("aikeys" from aikeys.maibornwolff.de)
	// so pi does not apply its stored Anthropic OAuth credentials to proxy requests.
	if !strings.Contains(ext, `"aikeys"`) {
		t.Error("extension should register under derived provider name, not \"anthropic\"")
	}

	// Check settings
	m := readSettingsMap(t, tgt)
	if m["defaultModel"] != "claude-sonnet-4-6" {
		t.Errorf("defaultModel = %v", m["defaultModel"])
	}
	if m["defaultProvider"] != "aikeys" {
		t.Errorf("defaultProvider = %v", m["defaultProvider"])
	}
}

func TestApply_EmptyProvider_RemovesExtension(t *testing.T) {
	tgt := setupPi(t)

	// First apply with provider
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://example.com",
			APIKey:   "sk-xxx",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() with provider: %v", err)
	}
	if _, err := os.Stat(tgt.extensionPath()); err != nil {
		t.Fatal("extension file should exist after Apply with provider")
	}

	// Apply with empty provider (OAuth mode)
	if err := tgt.Apply(config.TargetEntry{}); err != nil {
		t.Fatalf("Apply() empty: %v", err)
	}
	if _, err := os.Stat(tgt.extensionPath()); !os.IsNotExist(err) {
		t.Error("extension file should be removed for empty provider (OAuth)")
	}
}

func TestApply_KeylessCustomEndpointUsesPlaceholderAPIKey(t *testing.T) {
	tgt := setupPi(t)

	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "http://127.0.0.1:1234/v1",
			ProviderType: "openai",
			Name:         "lmstudio",
			Model:        "meta/muse-glimmer",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	ext := readExtension(t, tgt)
	if !strings.Contains(ext, `pi.registerProvider("lmstudio"`) {
		t.Errorf("extension should register lmstudio provider; got:\n%s", ext)
	}
	if !strings.Contains(ext, `apiKey: "aictx-local"`) {
		t.Errorf("extension missing keyless placeholder apiKey; got:\n%s", ext)
	}
	if strings.Contains(ext, "authHeader: true") {
		t.Errorf("keyless placeholder should not emit authHeader; got:\n%s", ext)
	}
	if !strings.Contains(ext, `const models = await aictxFetchModels("http://127.0.0.1:1234/v1", "", providerHeaders, "openai-completions", staticModels);`) {
		t.Errorf("dynamic model fetch should keep empty fetch auth key for keyless endpoint; got:\n%s", ext)
	}

	m := readSettingsMap(t, tgt)
	if m["defaultProvider"] != "lmstudio" {
		t.Errorf("defaultProvider = %v, want lmstudio", m["defaultProvider"])
	}
}

func TestApply_Headers(t *testing.T) {
	tgt := setupPi(t)

	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://proxy.example.com",
			APIKey:   "sk-test",
			Headers:  map[string]string{"X-Team-ID": "eng"},
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	ext := readExtension(t, tgt)
	if !strings.Contains(ext, "X-Team-ID") {
		t.Error("extension missing header key")
	}
	if !strings.Contains(ext, "eng") {
		t.Error("extension missing header value")
	}
}

func TestApply_AlwaysThinking(t *testing.T) {
	tgt := setupPi(t)

	b := true
	te := config.TargetEntry{
		Options: config.Options{AlwaysThinking: &b},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	m := readSettingsMap(t, tgt)
	if m["defaultThinkingLevel"] != "medium" {
		t.Errorf("defaultThinkingLevel = %v, want medium", m["defaultThinkingLevel"])
	}
}

func TestApply_MergesExistingSettings(t *testing.T) {
	tgt := setupPi(t)

	// Write existing settings
	existing := `{"lastChangelogVersion": "0.65.2"}`
	if err := os.WriteFile(tgt.settingsPath(), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	te := config.TargetEntry{
		Provider: config.Provider{Model: "claude-sonnet-4-6"},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	m := readSettingsMap(t, tgt)
	if m["lastChangelogVersion"] != "0.65.2" {
		t.Error("existing setting was lost")
	}
	if m["defaultModel"] != "claude-sonnet-4-6" {
		t.Errorf("defaultModel = %v", m["defaultModel"])
	}
}

// ---------- Provider naming ----------

func TestPiProviderName_OpenAIEndpoint(t *testing.T) {
	tgt := New()
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "https://aikeys.maibornwolff.de/",
			ProviderType: "openai",
		},
	}
	if name := tgt.piProviderName(te); name != "aikeys" {
		t.Errorf("piProviderName() = %q, want \"aikeys\"", name)
	}
}

func TestPiProviderName_LocalEndpoints(t *testing.T) {
	tgt := New()
	endpoints := []string{
		"http://127.0.0.1:1234/v1",
		"http://localhost:1234/v1",
		"http://[::1]:8080",
	}
	for _, ptype := range []string{"anthropic", "openai"} {
		for _, ep := range endpoints {
			te := config.TargetEntry{
				Provider: config.Provider{Endpoint: ep, ProviderType: ptype},
			}
			if name := tgt.piProviderName(te); name != "local" {
				t.Errorf("piProviderName(%s, %s) = %q, want \"local\"", ptype, ep, name)
			}
		}
	}
}

func TestPiProviderName_UnparseableEndpoint(t *testing.T) {
	tgt := New()
	te := config.TargetEntry{
		Provider: config.Provider{Endpoint: ":://bad", ProviderType: "openai"},
	}
	if name := tgt.piProviderName(te); name != "aictx" {
		t.Errorf("piProviderName() = %q, want \"aictx\"", name)
	}
}

func TestPiProviderName_ExplicitName(t *testing.T) {
	tgt := New()

	// Name + custom endpoint → Name wins over hostname derivation.
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "http://127.0.0.1:1234/v1",
			ProviderType: "openai",
			Name:         "lmstudio",
		},
	}
	if name := tgt.piProviderName(te); name != "lmstudio" {
		t.Errorf("piProviderName() = %q, want \"lmstudio\"", name)
	}

	// Name + Copilot endpoint → still "copilot" (OAuth identity is keyed to it).
	te = config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "https://api.githubcopilot.com",
			ProviderType: "openai",
			Name:         "mycopilot",
		},
	}
	if name := tgt.piProviderName(te); name != "copilot" {
		t.Errorf("piProviderName() = %q, want \"copilot\"", name)
	}

	// Name + no endpoint → builtin id (Name requires an endpoint).
	te = config.TargetEntry{
		Provider: config.Provider{Name: "custom", ProviderType: "openai"},
	}
	if name := tgt.piProviderName(te); name != "openai" {
		t.Errorf("piProviderName() = %q, want \"openai\"", name)
	}
	te = config.TargetEntry{
		Provider: config.Provider{Name: "custom"},
	}
	if name := tgt.piProviderName(te); name != "anthropic" {
		t.Errorf("piProviderName() = %q, want \"anthropic\"", name)
	}
}

func TestPiApply_OpenAIProvider(t *testing.T) {
	tgt := setupPi(t)

	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "https://aikeys.maibornwolff.de/",
			APIKey:       "sk-test-key",
			Model:        "gpt-5.5",
			ProviderType: "openai",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	ext := readExtension(t, tgt)
	if !strings.Contains(ext, `pi.registerProvider("aikeys"`) {
		t.Errorf("extension should register under derived name \"aikeys\"; got:\n%s", ext)
	}
	// Exact substring — a bare "openai" check would false-match "openai-completions".
	if strings.Contains(ext, `registerProvider("openai"`) {
		t.Errorf("extension must not register under pi's builtin \"openai\" id; got:\n%s", ext)
	}

	m := readSettingsMap(t, tgt)
	if m["defaultProvider"] != "aikeys" {
		t.Errorf("defaultProvider = %v, want \"aikeys\"", m["defaultProvider"])
	}
	if m["defaultModel"] != "gpt-5.5" {
		t.Errorf("defaultModel = %v, want \"gpt-5.5\"", m["defaultModel"])
	}
}

// ---------- Copilot provider ----------

func TestPiProviderName_CopilotEndpoint(t *testing.T) {
	tgt := New()
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "https://api.githubcopilot.com",
			APIKey:       "tid=testtoken",
			ProviderType: "openai", // resolved from "copilot" by switchContext
		},
	}
	name := tgt.piProviderName(te)
	if name != "copilot" {
		t.Errorf("piProviderName() = %q, want \"copilot\"", name)
	}
}

func TestPiApply_CopilotProvider(t *testing.T) {
	tgt := setupPi(t)

	// No APIKey — Copilot extension uses oauth refresh callbacks, not a static token.
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "https://api.githubcopilot.com",
			Model:        "gpt-4o",
			SmallModel:   "gpt-4o-mini",
			ProviderType: "openai",
			Headers: map[string]string{
				"Editor-Version":         "vscode/1.85.0",
				"Editor-Plugin-Version":  "copilot-chat/0.12.0",
				"Copilot-Integration-Id": "vscode-chat",
				"OpenAI-Intent":          "conversation-panel",
			},
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	ext := readExtension(t, tgt)

	// Provider name must be "copilot".
	if !strings.Contains(ext, `"copilot"`) {
		t.Errorf("extension missing \"copilot\" provider name; got:\n%s", ext)
	}
	// Must use oauth callbacks, not a static apiKey.
	if !strings.Contains(ext, "oauth:") {
		t.Errorf("extension missing oauth block; got:\n%s", ext)
	}
	if !strings.Contains(ext, "refreshToken") {
		t.Errorf("extension missing refreshToken callback; got:\n%s", ext)
	}
	if !strings.Contains(ext, "aictx copilot refresh") {
		t.Errorf("extension should call 'aictx copilot refresh'; got:\n%s", ext)
	}
	// Must NOT contain a hardcoded api token or keyless placeholder.
	if strings.Contains(ext, "tid=") {
		t.Errorf("extension must not contain a hardcoded Copilot token; got:\n%s", ext)
	}
	if strings.Contains(ext, placeholderAPIKey) {
		t.Errorf("copilot extension must not contain placeholder api key; got:\n%s", ext)
	}
	// authHeader must be present.
	if !strings.Contains(ext, "authHeader: true") {
		t.Errorf("extension missing authHeader; got:\n%s", ext)
	}
	// API format must be openai-completions (inside the models array).
	if !strings.Contains(ext, `"openai-completions"`) {
		t.Errorf("extension missing openai-completions api; got:\n%s", ext)
	}
	// All 4 required Copilot headers must appear.
	for _, header := range []string{
		"Editor-Version",
		"Editor-Plugin-Version",
		"Copilot-Integration-Id",
		"OpenAI-Intent",
	} {
		if !strings.Contains(ext, header) {
			t.Errorf("extension missing header %q; got:\n%s", header, ext)
		}
	}
	// Models must be present with reasoning: false.
	if !strings.Contains(ext, `"gpt-4o"`) {
		t.Error("extension missing gpt-4o model")
	}
	if !strings.Contains(ext, "reasoning: false") {
		t.Errorf("extension should have reasoning: false for gpt-4o; got:\n%s", ext)
	}

	// settings.json should use "copilot" as defaultProvider.
	m := readSettingsMap(t, tgt)
	if m["defaultProvider"] != "copilot" {
		t.Errorf("defaultProvider = %v, want \"copilot\"", m["defaultProvider"])
	}
	if m["defaultModel"] != "gpt-4o" {
		t.Errorf("defaultModel = %v, want \"gpt-4o\"", m["defaultModel"])
	}
}

func TestModelEntry_NonClaudeReasoning(t *testing.T) {
	tgt := New()
	entry := tgt.modelEntry("gpt-4o", "openai-completions")
	if !strings.Contains(entry, "reasoning: false") {
		t.Errorf("gpt-4o modelEntry should have reasoning: false; got:\n%s", entry)
	}
	claudeEntry := tgt.modelEntry("claude-3.7-sonnet", "anthropic-messages")
	if !strings.Contains(claudeEntry, "reasoning: true") {
		t.Errorf("claude-3.7-sonnet modelEntry should have reasoning: true; got:\n%s", claudeEntry)
	}
}

// ---------- Per-model API classification ----------

func TestApiForModel(t *testing.T) {
	tgt := New()
	cases := map[string]string{
		"claude-opus-4.8":  "anthropic-messages",
		"Claude-Foo":       "anthropic-messages",
		"gpt-5":            "openai-completions",
		"o3":               "openai-completions",
		"gemini-3.5-flash": "openai-completions",
	}
	for id, want := range cases {
		if got := tgt.apiForModel(id); got != want {
			t.Errorf("apiForModel(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestBuildExtension_PerModelStaticFallback(t *testing.T) {
	tgt := setupPi(t)

	gptExt := tgt.buildExtension(config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "gpt-5",
		},
	})
	staticStart := strings.Index(gptExt, "const staticModels = [")
	staticEnd := strings.Index(gptExt, "const providerHeaders")
	if staticStart < 0 || staticEnd < 0 || staticEnd <= staticStart {
		t.Fatalf("could not locate static model block; got:\n%s", gptExt)
	}
	staticBlock := gptExt[staticStart:staticEnd]
	if !strings.Contains(staticBlock, `"openai-completions"`) {
		t.Errorf("gpt-5 static fallback should use openai-completions; got:\n%s", staticBlock)
	}
	if strings.Contains(staticBlock, `"anthropic-messages"`) {
		t.Errorf("gpt-5 static fallback should not use anthropic-messages; got:\n%s", staticBlock)
	}

	claudeExt := tgt.buildExtension(config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-opus-4.8",
		},
	})
	cStart := strings.Index(claudeExt, "const staticModels = [")
	cEnd := strings.Index(claudeExt, "const providerHeaders")
	if cStart < 0 || cEnd < 0 || cEnd <= cStart {
		t.Fatalf("could not locate static model block; got:\n%s", claudeExt)
	}
	claudeBlock := claudeExt[cStart:cEnd]
	if !strings.Contains(claudeBlock, `"anthropic-messages"`) {
		t.Errorf("claude static fallback should use anthropic-messages; got:\n%s", claudeBlock)
	}
}

func TestBuildExtension_PerModelHelperPresent(t *testing.T) {
	tgt := setupPi(t)
	ext := tgt.buildExtension(config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-opus-4.8",
		},
	})
	if !strings.Contains(ext, "function aictxApiForModel") {
		t.Errorf("extension should define aictxApiForModel; got:\n%s", ext)
	}
	// Neither fetch path should hardcode api: apiFormat on mapped models.
	if strings.Contains(ext, "api: apiFormat") {
		t.Errorf("mapped models should route through aictxApiForModel, not hardcode apiFormat; got:\n%s", ext)
	}
	if !strings.Contains(ext, "api: aictxApiForModel(id, (m.litellm_params && m.litellm_params.model) || \"\", apiFormat)") {
		t.Errorf("/model/info path should classify per model; got:\n%s", ext)
	}
	if !strings.Contains(ext, `api: aictxApiForModel(m.id, "", apiFormat)`) {
		t.Errorf("/v1/models fallback path should classify per model; got:\n%s", ext)
	}
}

// ---------- Async dynamic extension ----------

func TestBuildExtension_AsyncDynamic(t *testing.T) {
	tgt := setupPi(t)
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-opus-4.8",
		},
	}
	ext := tgt.buildExtension(te)

	for _, want := range []string{
		"export default async function",
		"await",
		"fetch",
		"/model/info",
		"aictxDedup",
		"staticModels",
		"AbortController",
	} {
		if !strings.Contains(ext, want) {
			t.Errorf("async extension missing %q; got:\n%s", want, ext)
		}
	}
	// registerProvider under the derived name with the fetched models.
	if !strings.Contains(ext, `pi.registerProvider("aikeys"`) {
		t.Error("extension should register under derived provider name")
	}
	if !strings.Contains(ext, "models,") {
		t.Error("extension should pass fetched models to registerProvider")
	}
}

func TestBuildExtension_ResponsesModeIncluded(t *testing.T) {
	tgt := setupPi(t)
	ext := tgt.buildExtension(config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-opus-4.8",
		},
	})
	// The /model/info filter must admit Responses-API models (mode:
	// "responses"), which are chat-capable through the LiteLLM proxy.
	if !strings.Contains(ext, `mode === "responses"`) {
		t.Errorf("extension /model/info filter should accept mode \"responses\"; got:\n%s", ext)
	}
	if !strings.Contains(ext, `mode === "chat" || mode === "responses" || mode == null`) {
		t.Errorf("extension /model/info filter should accept chat, responses, and missing mode; got:\n%s", ext)
	}
}

func TestBuildExtension_StaticFallbackPresent(t *testing.T) {
	tgt := setupPi(t)
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:   "https://aikeys.maibornwolff.de/",
			APIKey:     "sk-test-key",
			Model:      "claude-opus-4.8",
			SmallModel: "claude-haiku-4.5",
		},
	}
	ext := tgt.buildExtension(te)
	if !strings.Contains(ext, "claude-opus-4.8") {
		t.Error("static fallback should contain the config Model")
	}
	if !strings.Contains(ext, "claude-haiku-4.5") {
		t.Error("static fallback should contain the config SmallModel")
	}
}

func TestApply_KeylessPlaceholderNotDiscoverable(t *testing.T) {
	tgt := setupPi(t)
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint:     "http://127.0.0.1:1234/v1",
			ProviderType: "openai",
			Name:         "lmstudio",
			Model:        "meta/muse-glimmer",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	dr, err := tgt.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if dr.Provider.Endpoint != "http://127.0.0.1:1234/v1" {
		t.Errorf("discovered endpoint = %q", dr.Provider.Endpoint)
	}
	if dr.Provider.APIKey != "" {
		t.Errorf("placeholder apiKey should not be discoverable; got %q", dr.Provider.APIKey)
	}
	if dr.Provider.Model != "meta/muse-glimmer" {
		t.Errorf("discovered model = %q", dr.Provider.Model)
	}
}

func TestApply_AsyncExtensionDiscoverable(t *testing.T) {
	tgt := setupPi(t)
	te := config.TargetEntry{
		Provider: config.Provider{
			Endpoint: "https://aikeys.maibornwolff.de/",
			APIKey:   "sk-test-key",
			Model:    "claude-opus-4.8",
		},
	}
	if err := tgt.Apply(te); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	dr, err := tgt.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if dr.Provider.Endpoint != "https://aikeys.maibornwolff.de/" {
		t.Errorf("discovered endpoint = %q", dr.Provider.Endpoint)
	}
	if dr.Provider.APIKey != "sk-test-key" {
		t.Errorf("discovered apiKey = %q", dr.Provider.APIKey)
	}
}

// ---------- Discover ----------

func TestDiscover_NoExtension(t *testing.T) {
	tgt := setupPi(t)

	dr, err := tgt.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if dr.Provider.Endpoint != "" {
		t.Errorf("Endpoint = %q, want empty", dr.Provider.Endpoint)
	}
}

func TestDiscover_WithExtensionAndSettings(t *testing.T) {
	tgt := setupPi(t)

	// Write settings (defaultThinkingLevel is Options-level; skipped in Discover)
	settings := `{"defaultModel": "claude-sonnet-4-6", "defaultThinkingLevel": "medium"}`
	if err := os.WriteFile(tgt.settingsPath(), []byte(settings), 0644); err != nil {
		t.Fatal(err)
	}

	// Write extension
	ext := `import type { ExtensionAPI } from "@mariozechner/pi-coding-agent";
export default function (pi: ExtensionAPI) {
  pi.registerProvider("anthropic", {
    baseUrl: "https://aikeys.maibornwolff.de/",
    apiKey: "sk-test-key",
  });
}`
	if err := os.MkdirAll(tgt.extensionsDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tgt.extensionPath(), []byte(ext), 0644); err != nil {
		t.Fatal(err)
	}

	dr, err := tgt.Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if dr.Provider.Endpoint != "https://aikeys.maibornwolff.de/" {
		t.Errorf("Endpoint = %q", dr.Provider.Endpoint)
	}
	if dr.Provider.APIKey != "sk-test-key" {
		t.Errorf("APIKey = %q", dr.Provider.APIKey)
	}
	if dr.Provider.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q", dr.Provider.Model)
	}
	// AlwaysThinking is Options-level (context-level); DiscoveryResult has no Options field.
	// Verify we get a valid result without panicking.
	if dr.ID != ID {
		t.Errorf("ID = %q, want %q", dr.ID, ID)
	}
}
