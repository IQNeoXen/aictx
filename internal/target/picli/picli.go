package picli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
)

const ID = "pi-cli"

// extensionFileName is the generated extension that configures the provider.
const extensionFileName = "aictx-provider.ts"

type Target struct{}

func New() *Target { return &Target{} }

func (t *Target) ID() string   { return ID }
func (t *Target) Name() string { return "pi Coding Agent CLI" }

func (t *Target) piDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent")
}

func (t *Target) settingsPath() string {
	return filepath.Join(t.piDir(), "settings.json")
}

func (t *Target) extensionsDir() string {
	return filepath.Join(t.piDir(), "extensions")
}

func (t *Target) extensionPath() string {
	return filepath.Join(t.extensionsDir(), extensionFileName)
}

func (t *Target) Detect() bool {
	// Detected if the pi agent directory exists
	if _, err := os.Stat(t.piDir()); err == nil {
		return true
	}
	return false
}

func (t *Target) Apply(te config.TargetEntry) error {
	// --- 1. Write/update the extension file for provider config ---
	if err := t.applyExtension(te); err != nil {
		return err
	}

	// --- 2. Update settings.json for model/thinking preferences ---
	if err := t.applySettings(te); err != nil {
		return err
	}

	return nil
}

func (t *Target) applyExtension(te config.TargetEntry) error {
	// If no provider config, remove the extension file (native auth / OAuth)
	if te.Provider.Endpoint == "" && te.Provider.APIKey == "" && len(te.Provider.Headers) == 0 {
		path := t.extensionPath()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing pi extension: %w", err)
		}
		return nil
	}

	// Build the extension TypeScript source
	ext := t.buildExtension(te)

	// Ensure extensions directory exists
	if err := os.MkdirAll(t.extensionsDir(), 0755); err != nil {
		return fmt.Errorf("creating pi extensions dir: %w", err)
	}

	path := t.extensionPath()
	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, []byte(ext), 0644); err != nil {
		return fmt.Errorf("writing pi extension: %w", err)
	}
	return os.Rename(tmp, path)
}

// providerType returns the logical API type: "anthropic" (default) or "openai".
func (t *Target) providerType(te config.TargetEntry) string {
	if te.Provider.ProviderType != "" {
		return te.Provider.ProviderType
	}
	return "anthropic"
}

// piProviderName returns the name to pass to pi's registerProvider.
// Special cases:
//   - GitHub Copilot endpoint → "copilot" (must be checked first, as the
//     ProviderType is resolved to "openai" by the time Apply() is called).
//   - Anthropic with custom endpoint → derived from hostname so pi does NOT
//     apply its stored Anthropic OAuth credentials to proxy requests.
func (t *Target) piProviderName(te config.TargetEntry) string {
	// Special case: GitHub Copilot endpoint → "copilot".
	// This check must come first because the ProviderType is resolved to
	// "openai" (not "copilot") by the time Apply() is called.
	if te.Provider.Endpoint != "" {
		if u, err := url.Parse(te.Provider.Endpoint); err == nil {
			if strings.HasSuffix(u.Hostname(), "githubcopilot.com") {
				return "copilot"
			}
		}
	}

	ptype := t.providerType(te)
	if ptype != "anthropic" || te.Provider.Endpoint == "" {
		return ptype
	}
	u, err := url.Parse(te.Provider.Endpoint)
	if err != nil || u.Hostname() == "" {
		return "aictx"
	}
	host := u.Hostname()
	if idx := strings.Index(host, "."); idx > 0 {
		return host[:idx] // e.g. "aikeys" from "aikeys.maibornwolff.de"
	}
	return host
}

// apiType returns the pi api format string for model definitions.
func (t *Target) apiType(te config.TargetEntry) string {
	if t.providerType(te) == "openai" {
		return "openai-completions"
	}
	return "anthropic-messages"
}

// isCopilotProvider returns true when the effective provider points at the
// GitHub Copilot API. The check is endpoint-based because the ProviderType
// is resolved to "openai" by switchContext before Apply() is called.
func (t *Target) isCopilotProvider(te config.TargetEntry) bool {
	return t.piProviderName(te) == "copilot"
}

func (t *Target) buildExtension(te config.TargetEntry) string {
	if t.isCopilotProvider(te) {
		return t.buildCopilotExtension(te)
	}

	providerName := t.piProviderName(te)
	apiType := t.apiType(te)

	// Build the static fallback model list (used when the live fetch fails).
	var staticModels strings.Builder
	if te.Provider.Model != "" {
		staticModels.WriteString(t.modelEntry(te.Provider.Model, apiType))
		if te.Provider.SmallModel != "" {
			staticModels.WriteString(",\n")
			staticModels.WriteString(t.modelEntry(te.Provider.SmallModel, apiType))
		}
	}

	// Provider headers literal (also used both for the fetch and registerProvider).
	var headers strings.Builder
	headers.WriteString("{")
	if len(te.Provider.Headers) > 0 {
		headers.WriteString("\n")
		for k, v := range te.Provider.Headers {
			headers.WriteString(fmt.Sprintf("    %s: %s,\n", jsonString(k), jsonString(v)))
		}
		headers.WriteString("  }")
	} else {
		headers.WriteString("}")
	}

	var sb strings.Builder
	sb.WriteString("// Generated by aictx — do not edit manually.\n")
	sb.WriteString("import type { ExtensionAPI } from \"@mariozechner/pi-coding-agent\";\n\n")

	// Embedded helper: fetch + map + dedup the live model list, with fallback.
	sb.WriteString(dynamicModelsHelperJS)
	sb.WriteString("\n")

	sb.WriteString("export default async function (pi: ExtensionAPI) {\n")

	// Static fallback list.
	sb.WriteString("  const staticModels = [\n")
	if staticModels.Len() > 0 {
		sb.WriteString(staticModels.String())
		sb.WriteString("\n")
	}
	sb.WriteString("  ];\n")

	// Provider headers.
	sb.WriteString(fmt.Sprintf("  const providerHeaders = %s;\n", headers.String()))

	// Fetch the live model list (falls back to staticModels on any failure).
	endpointLit := jsonString(te.Provider.Endpoint)
	apiKeyLit := jsonString(te.Provider.APIKey)
	sb.WriteString(fmt.Sprintf("  const models = await aictxFetchModels(%s, %s, providerHeaders, %s, staticModels);\n\n",
		endpointLit, apiKeyLit, jsonString(apiType)))

	sb.WriteString(fmt.Sprintf("  pi.registerProvider(%s, {\n", jsonString(providerName)))
	if te.Provider.Endpoint != "" {
		sb.WriteString(fmt.Sprintf("    baseUrl: %s,\n", jsonString(te.Provider.Endpoint)))
	}
	if te.Provider.APIKey != "" {
		sb.WriteString(fmt.Sprintf("    apiKey: %s,\n", jsonString(te.Provider.APIKey)))
		sb.WriteString("    authHeader: true,\n")
	}
	if len(te.Provider.Headers) > 0 {
		sb.WriteString("    headers: providerHeaders,\n")
	}
	sb.WriteString("    models,\n")
	sb.WriteString("  });\n}\n")
	return sb.String()
}

// dynamicModelsHelperJS is the embedded TypeScript helper that fetches the live
// model catalog from the provider endpoint at pi startup, maps it to pi model
// definitions, and dedups it. The dedup rule MUST stay in sync with
// internal/models.Dedup (strip "vendor:" prefix, treat dashed versions as
// dotted, prefer the dotted unprefixed representative). On any failure it
// returns the caller-supplied static fallback list so pi always starts.
const dynamicModelsHelperJS = `function aictxNormalizeId(id: string): string {
  const colon = id.indexOf(":");
  if (colon >= 0) id = id.slice(colon + 1);
  let prev: string;
  do {
    prev = id;
    id = id.replace(/(\d)-(\d)/g, "$1.$2");
  } while (id !== prev);
  return id;
}

function aictxCandidateScore(id: string): number {
  let score = 0;
  if (!id.includes(":")) score += 2;
  if (id.includes(".")) score += 1;
  return score;
}

function aictxDedup(models: any[]): any[] {
  const groups = new Map<string, any[]>();
  for (const m of models) {
    const key = aictxNormalizeId(m.id);
    const g = groups.get(key);
    if (g) g.push(m);
    else groups.set(key, [m]);
  }
  const out: any[] = [];
  for (const g of groups.values()) {
    let best = g[0];
    for (const c of g.slice(1)) {
      const sc = aictxCandidateScore(c.id);
      const sb = aictxCandidateScore(best.id);
      if (sc > sb || (sc === sb && c.id < best.id)) best = c;
    }
    out.push(best);
  }
  out.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  return out;
}

async function aictxFetchModels(
  endpoint: string,
  authKey: string,
  providerHeaders: Record<string, string>,
  apiFormat: string,
  staticModels: any[],
): Promise<any[]> {
  const base = endpoint.replace(/\/$/, "");
  const headers: Record<string, string> = { ...providerHeaders };
  if (authKey) headers["Authorization"] = "Bearer " + authKey;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    // Preferred: /model/info carries cost + capability metadata.
    try {
      const res = await fetch(base + "/model/info", { headers, signal: controller.signal });
      if (res.ok) {
        const payload: any = await res.json();
        const mapped = (payload.data || [])
          .filter((m: any) => {
            const mode = m.model_info && m.model_info.mode;
            return mode === "chat" || mode == null;
          })
          .map((m: any) => {
            const info = m.model_info || {};
            const id = m.model_name;
            return {
              id,
              name: id,
              api: apiFormat,
              reasoning: !!info.supports_reasoning,
              input: info.supports_vision ? ["text", "image"] : ["text"],
              cost: {
                input: (info.input_cost_per_token || 0) * 1e6,
                output: (info.output_cost_per_token || 0) * 1e6,
                cacheRead: (info.cache_read_input_token_cost || 0) * 1e6,
                cacheWrite: (info.cache_creation_input_token_cost || 0) * 1e6,
              },
              contextWindow: info.max_input_tokens || 200000,
              maxTokens: info.max_output_tokens || info.max_tokens || 32000,
            };
          });
        const deduped = aictxDedup(mapped);
        if (deduped.length) return deduped;
      }
    } catch {
      // fall through to OpenAI-style endpoints
    }

    // Fallback: OpenAI-style /v1/models then /models (ids only, cost 0).
    for (const path of ["/v1/models", "/models"]) {
      try {
        const r = await fetch(base + path, { headers, signal: controller.signal });
        if (!r.ok) continue;
        const p: any = await r.json();
        const mapped = (p.data || []).map((m: any) => ({
          id: m.id,
          name: m.id,
          api: apiFormat,
          reasoning: String(m.id).includes("claude"),
          input: ["text", "image"],
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
          contextWindow: m.max_input_tokens || 200000,
          maxTokens: m.max_output_tokens || 32000,
        }));
        const deduped = aictxDedup(mapped);
        if (deduped.length) return deduped;
      } catch {
        // try next path
      }
    }
  } finally {
    clearTimeout(timer);
  }
  return staticModels;
}
`

// buildCopilotExtension generates a pi extension that uses the oauth
// refreshToken callback to auto-renew the short-lived Copilot API token.
// Instead of embedding a static token, the extension calls
// 'aictx copilot refresh' whenever pi needs a fresh credential.
func (t *Target) buildCopilotExtension(te config.TargetEntry) string {
	const apiType = "openai-completions"
	var sb strings.Builder
	sb.WriteString("// Generated by aictx — do not edit manually.\n")
	sb.WriteString("import type { ExtensionAPI } from \"@mariozechner/pi-coding-agent\";\n")
	sb.WriteString("import { execSync } from \"node:child_process\";\n\n")
	sb.WriteString("export default function (pi: ExtensionAPI) {\n")

	// Helper that calls aictx copilot refresh and returns {token, expiresAt}.
	sb.WriteString("  function refreshCopilotToken(): { token: string; expiresAt: number } {\n")
	sb.WriteString("    try {\n")
	sb.WriteString("      const out = execSync(\"aictx copilot refresh\", { encoding: \"utf8\" }).trim();\n")
	sb.WriteString("      return JSON.parse(out);\n")
	sb.WriteString("    } catch {\n")
	sb.WriteString("      throw new Error(\"Copilot token refresh failed — run 'aictx copilot login' to re-authenticate.\");\n")
	sb.WriteString("    }\n")
	sb.WriteString("  }\n\n")

	sb.WriteString("  pi.registerProvider(\"copilot\", {\n")
	sb.WriteString(fmt.Sprintf("    baseUrl: %s,\n", jsonString(te.Provider.Endpoint)))
	sb.WriteString("    authHeader: true,\n")

	if len(te.Provider.Headers) > 0 {
		sb.WriteString("    headers: {\n")
		for k, v := range te.Provider.Headers {
			sb.WriteString(fmt.Sprintf("      %s: %s,\n", jsonString(k), jsonString(v)))
		}
		sb.WriteString("    },\n")
	}

	if te.Provider.Model != "" {
		sb.WriteString("    models: [\n")
		sb.WriteString(t.modelEntry(te.Provider.Model, apiType))
		if te.Provider.SmallModel != "" {
			sb.WriteString(",\n")
			sb.WriteString(t.modelEntry(te.Provider.SmallModel, apiType))
		}
		sb.WriteString("\n    ],\n")
	}

	// oauth block: pi persists credentials in ~/.pi/agent/auth.json and calls
	// refreshToken() automatically before the expires timestamp is reached.
	sb.WriteString("    oauth: {\n")
	sb.WriteString("      name: \"GitHub Copilot\",\n")
	sb.WriteString("      async login(_callbacks: unknown) {\n")
	sb.WriteString("        const { token, expiresAt } = refreshCopilotToken();\n")
	sb.WriteString("        return { refresh: \"aictx\", access: token, expires: expiresAt };\n")
	sb.WriteString("      },\n")
	sb.WriteString("      async refreshToken(credentials: { refresh: string; access: string; expires: number }) {\n")
	sb.WriteString("        const { token, expiresAt } = refreshCopilotToken();\n")
	sb.WriteString("        return { refresh: credentials.refresh, access: token, expires: expiresAt };\n")
	sb.WriteString("      },\n")
	sb.WriteString("      getApiKey(credentials: { access: string }) {\n")
	sb.WriteString("        return credentials.access;\n")
	sb.WriteString("      },\n")
	sb.WriteString("    },\n")
	sb.WriteString("  });\n")
	sb.WriteString("}\n")
	return sb.String()
}

func (t *Target) modelEntry(modelID, apiType string) string {
	reasoning := strings.Contains(modelID, "claude") // all current claude models support thinking
	var sb strings.Builder
	sb.WriteString("      {\n")
	sb.WriteString(fmt.Sprintf("        id: %s,\n", jsonString(modelID)))
	sb.WriteString(fmt.Sprintf("        name: %s,\n", jsonString(modelID)))
	sb.WriteString(fmt.Sprintf("        api: %s,\n", jsonString(apiType)))
	sb.WriteString(fmt.Sprintf("        reasoning: %v,\n", reasoning))
	sb.WriteString("        input: [\"text\", \"image\"],\n")
	sb.WriteString("        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },\n")
	sb.WriteString("        contextWindow: 200000,\n")
	sb.WriteString("        maxTokens: 32000,\n")
	sb.WriteString("      }")
	return sb.String()
}

func (t *Target) applySettings(te config.TargetEntry) error {
	settings := map[string]interface{}{}

	path := t.settingsPath()
	if raw, err := os.ReadFile(path); err == nil {
		if jsonErr := json.Unmarshal(raw, &settings); jsonErr != nil {
			settings = map[string]interface{}{}
		}
	}

	if te.Provider.Model != "" {
		settings["defaultModel"] = te.Provider.Model
		settings["defaultProvider"] = t.piProviderName(te)
	} else {
		delete(settings, "defaultModel")
		delete(settings, "defaultProvider")
	}

	if te.Options.AlwaysThinking != nil && *te.Options.AlwaysThinking {
		settings["defaultThinkingLevel"] = "medium"
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pi settings: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".aictx-tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing pi settings: %w", err)
	}
	return os.Rename(tmp, path)
}

func (t *Target) Discover() (*config.DiscoveryResult, error) {
	dr := &config.DiscoveryResult{ID: ID}

	// Read settings.json for model (thinking is Options-level, skip during discovery)
	if raw, err := os.ReadFile(t.settingsPath()); err == nil {
		var settings map[string]interface{}
		if jsonErr := json.Unmarshal(raw, &settings); jsonErr == nil {
			if m, ok := settings["defaultModel"].(string); ok {
				dr.Provider.Model = m
			}
		}
	}

	// Read extension file for endpoint/apiKey
	extData, err := os.ReadFile(t.extensionPath())
	if err == nil {
		t.parseExtension(string(extData), dr)
	}

	return dr, nil
}

// parseExtension does a best-effort extraction of baseUrl and apiKey from
// the generated extension file.
func (t *Target) parseExtension(source string, dr *config.DiscoveryResult) {
	// Look for baseUrl: "..."
	if v := extractTSString(source, "baseUrl"); v != "" {
		dr.Provider.Endpoint = v
	}
	// Look for apiKey: "..."
	if v := extractTSString(source, "apiKey"); v != "" {
		dr.Provider.APIKey = v
	}
}

// extractTSString extracts a string value from a simple `key: "value"` pattern.
func extractTSString(source, key string) string {
	// Look for key: "value" or key: 'value'
	needle := key + ":"
	idx := strings.Index(source, needle)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(source[idx+len(needle):])
	if len(rest) == 0 {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(rest[1:], quote)
	if end < 0 {
		return ""
	}
	return rest[1 : end+1]
}

// jsonString returns a JSON-encoded string literal (with quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
