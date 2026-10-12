package picli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/IQNeoXen/aictx/internal/config"
)

const ID = "pi-cli"

// extensionFileName is the generated extension that configures the provider.
const extensionFileName = "aictx-provider.ts"

// placeholderAPIKey marks keyless local/custom endpoints as configured in pi.
// pi requires auth presence for extension-registered custom provider models to
// appear in /model; local servers such as LM Studio ignore this value.
const placeholderAPIKey = "aictx-local"

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
// Resolution order:
//   - GitHub Copilot endpoint → "copilot" (must be checked first, as the
//     ProviderType is resolved to "openai" by the time Apply() is called;
//     pi's stored Copilot OAuth identity is keyed to this name, so an
//     explicit Provider.Name is ignored here).
//   - No endpoint → the builtin provider type id ("anthropic"/"openai").
//   - Explicit Provider.Name → used verbatim.
//   - Otherwise derived from the endpoint hostname: "aictx" if unparseable,
//     "local" for IP/localhost endpoints, else the first dot-label.
//
// Custom endpoints must NOT register under pi's builtin ids ("anthropic",
// "openai"): pi prefers per-name credentials from ~/.pi/agent/auth.json over
// the extension's embedded key, and registering under a builtin id replaces
// pi's builtin model catalog for that provider for the whole session.
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

	// No endpoint → builtin provider id. A Provider.Name without an endpoint
	// is ignored: applyExtension registers no provider in that case, so a
	// custom defaultProvider in settings.json would dangle.
	if te.Provider.Endpoint == "" {
		return t.providerType(te)
	}
	if te.Provider.Name != "" {
		return te.Provider.Name
	}
	u, err := url.Parse(te.Provider.Endpoint)
	if err != nil || u.Hostname() == "" {
		return "aictx"
	}
	host := u.Hostname()
	if host == "localhost" || net.ParseIP(host) != nil {
		return "local"
	}
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

// apiForModel returns the pi api format string for a single model, derived from
// the model id. This is the proxy-oriented binary rule: a LiteLLM-style proxy
// fronts both Anthropic Claude and OpenAI-compatible (GPT/Gemini/Azure/...)
// models under one baseUrl, so each model must carry its real wire protocol.
// It mirrors aictxApiForModel in the embedded TS (same lowercase +
// claude/anthropic match) so the static fallback and the live fetch agree.
func (t *Target) apiForModel(modelID string) string {
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") {
		return "anthropic-messages"
	}
	return "openai-completions"
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
		staticModels.WriteString(t.modelEntry(te.Provider.Model, t.apiForModel(te.Provider.Model)))
		if te.Provider.SmallModel != "" {
			staticModels.WriteString(",\n")
			staticModels.WriteString(t.modelEntry(te.Provider.SmallModel, t.apiForModel(te.Provider.SmallModel)))
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

	// Fetch live models and keep discovery failure separate from the static fallback.
	endpointLit := jsonString(te.Provider.Endpoint)
	apiKeyLit := jsonString(te.Provider.APIKey)
	sb.WriteString(fmt.Sprintf("  const discovery = await aictxFetchModels(%s, %s, providerHeaders, %s, staticModels);\n", endpointLit, apiKeyLit, jsonString(apiType)))
	sb.WriteString("  const models = discovery.models;\n\n")

	sb.WriteString(fmt.Sprintf("  pi.registerProvider(%s, {\n", jsonString(providerName)))
	if te.Provider.Endpoint != "" {
		sb.WriteString(fmt.Sprintf("    baseUrl: %s,\n", jsonString(te.Provider.Endpoint)))
	}
	if te.Provider.APIKey != "" {
		sb.WriteString(fmt.Sprintf("    apiKey: %s,\n", jsonString(te.Provider.APIKey)))
		sb.WriteString("    authHeader: true,\n")
	} else if te.Provider.Endpoint != "" {
		sb.WriteString(fmt.Sprintf("    apiKey: %s,\n", jsonString(placeholderAPIKey)))
	}
	if len(te.Provider.Headers) > 0 {
		sb.WriteString("    headers: providerHeaders,\n")
	}
	sb.WriteString("    models,\n")
	sb.WriteString("  });\n\n")
	sb.WriteString(`  let clearBanner: (() => void) | undefined;
  pi.on("session_start", (_event, ctx) => {
    clearBanner?.();
    clearBanner = undefined;
    if (!discovery.failure || ctx.mode !== "tui" || !ctx.ui?.setWidget || !ctx.ui?.onTerminalInput) return;

    const widgetKey = "aictx-model-discovery-error";
    let unsubscribe: (() => void) | undefined;
    const clear = () => {
      ctx.ui.setWidget(widgetKey, undefined);
      unsubscribe?.();
      unsubscribe = undefined;
    };
    clearBanner = clear;
    ctx.ui.setWidget(widgetKey, [ctx.ui.theme.fg("error", "Model discovery failed: " + discovery.failure + " (Press any key to dismiss)")], { placement: "aboveEditor" });
    unsubscribe = ctx.ui.onTerminalInput(() => {
      clear();
      clearBanner = undefined;
      return undefined;
    });
  });
  pi.on("session_before_switch", () => { clearBanner?.(); clearBanner = undefined; });
  pi.on("session_shutdown", () => { clearBanner?.(); clearBanner = undefined; });
}
`)
	return sb.String()
}

// dynamicModelsHelperJS is the embedded TypeScript helper that fetches the live
// model catalog from the provider endpoint at pi startup, maps it to pi model
// definitions, and dedups it. Discovery errors are returned separately from the static
// fallback list so the extension can display them without blocking provider registration.
// The dedup rule MUST stay in sync with
// internal/models.Dedup (strip "vendor:" prefix, treat dashed versions as
// dotted, prefer the dotted unprefixed representative). On any failure it
// returns the caller-supplied static fallback list so pi always starts.
const dynamicModelsHelperJS = `function aictxApiForModel(id: string, litellmModel: string, fallback: string): string {
  const hay = ((litellmModel || "") + " " + (id || "")).toLowerCase();
  if (hay.includes("anthropic") || hay.includes("claude")) return "anthropic-messages";
  if (hay.trim()) return "openai-completions";
  return fallback;
}

function aictxNormalizeId(id: string): string {
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
): Promise<{ models: any[]; failure?: string }> {
  const base = endpoint.replace(/\/$/, "");
  const headers: Record<string, string> = { ...providerHeaders };
  if (authKey) headers["Authorization"] = "Bearer " + authKey;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  const errors: Array<{ path: string; status?: number; detail?: string; network?: string }> = [];
  const paths = ["/model/info", "/v1/models", "/models"];
  const usefulDetail = (text: string): boolean => !!text && !/^(not found[.!]?|404\s+not found[.!]?|http\s+\d{3}(?:\s*[:\-—]\s*not found)?[.!]?)$/i.test(text.trim());
  const errorText = (payload: any): string => {
    if (typeof payload === "string") return payload.trim();
    const candidates = [payload?.error?.message, payload?.error?.detail, payload?.message, payload?.detail, payload?.error];
    for (const candidate of candidates) {
      if (typeof candidate === "string" && candidate.trim()) return candidate.trim();
      if (candidate && typeof candidate === "object") {
        const nested = candidate.message || candidate.detail;
        if (typeof nested === "string" && nested.trim()) return nested.trim();
      }
    }
    return "";
  };
  const recordHTTPError = async (path: string, res: Response) => {
    let detail = "";
    try {
      const body = await res.text();
      try { detail = errorText(JSON.parse(body)); } catch { detail = errorText(body); }
    } catch { /* ignore unreadable body */ }
    errors.push({ path, status: res.status, detail: usefulDetail(detail) ? detail : undefined });
  };
  const recordNetworkError = (path: string, error: any) => {
    const detail = error?.name === "AbortError" ? "request timed out" : (error?.message || "network request failed");
    errors.push({ path, network: detail });
  };
  const chooseFailure = (): string => {
    const specific = errors.find((e) => e.status !== undefined && e.status !== 404 && e.detail);
    if (specific?.detail) return "HTTP " + specific.status + " — " + specific.detail;
    const auth = errors.find((e) => e.status === 401 || e.status === 403);
    if (auth) return "HTTP " + auth.status;
    const status = errors.find((e) => e.status !== undefined && e.status !== 404);
    if (status) return "HTTP " + status.status;
    const network = errors.find((e) => e.network);
    if (network) return network.path + ": " + network.network;
    const notFound = errors.find((e) => e.status === 404);
    if (notFound) return "HTTP 404" + (notFound.detail ? " — " + notFound.detail : "");
    return "no usable models were returned";
  };
  const sanitizeFailure = (text: string): string => {
    let safe = authKey ? text.split(authKey).join("[REDACTED]") : text;
    safe = safe.replace(/\u001B\][^\u0007]*(?:\u0007|\u001B\\)|\u001B\[[0-?]*[ -/]*[@-~]|\u009B[0-?]*[ -/]*[@-~]/g, "");
    safe = safe.replace(/[\x00-\x1F\x7F-\x9F]/g, " ").trim();
    return safe.length > 240 ? safe.slice(0, 237) + "..." : safe;
  };
  try {
    // Preferred: /model/info carries cost + capability metadata.
    try {
      const res = await fetch(base + paths[0], { headers, signal: controller.signal });
      if (!res.ok) { await recordHTTPError(paths[0], res); }
      else {
        const payload: any = await res.json();
        const mapped = (Array.isArray(payload?.data) ? payload.data : [])
          .filter((m: any) => m && typeof m === "object")
          .filter((m: any) => {
            const mode = m.model_info && m.model_info.mode;
            return mode === "chat" || mode === "responses" || mode == null;
          })
          .map((m: any) => {
            const info = m.model_info || {};
            const id = m.model_name;
            if (typeof id !== "string" || !id.trim()) return undefined;
            return {
              id,
              name: id,
              api: aictxApiForModel(id, (m.litellm_params && m.litellm_params.model) || "", apiFormat),
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
          })
          .filter((m: any) => m !== undefined);
        const deduped = aictxDedup(mapped);
        if (deduped.length) return { models: deduped };
      }
    } catch (error) {
      if (error?.name !== "SyntaxError") recordNetworkError(paths[0], error);
    }

    // Fallback: OpenAI-style /v1/models then /models (ids only, cost 0).
    for (const path of paths.slice(1)) {
      try {
        const r = await fetch(base + path, { headers, signal: controller.signal });
        if (!r.ok) { await recordHTTPError(path, r); continue; }
        const p: any = await r.json();
        const mapped = (Array.isArray(p?.data) ? p.data : []).filter((m: any) => typeof m?.id === "string" && m.id.trim()).map((m: any) => ({
          id: m.id,
          name: m.id,
          api: aictxApiForModel(m.id, "", apiFormat),
          reasoning: String(m.id).includes("claude"),
          input: ["text", "image"],
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
          contextWindow: m.max_input_tokens || 200000,
          maxTokens: m.max_output_tokens || 32000,
        }));
        const deduped = aictxDedup(mapped);
        if (deduped.length) return { models: deduped };
      } catch (error) {
        if (error?.name !== "SyntaxError") recordNetworkError(path, error);
      }
    }
  } finally {
    clearTimeout(timer);
  }
  return { models: staticModels, failure: sanitizeFailure(chooseFailure()) };
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
	// Look for apiKey: "...". Ignore aictx's placeholder for keyless
	// endpoints so discovery does not persist it as a real secret.
	if v := extractTSString(source, "apiKey"); v != "" && v != placeholderAPIKey {
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
