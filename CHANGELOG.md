# Changelog

## v0.3.0 (2026-07-10)

### Added

- **`provider.name` config field** — optionally overrides the derived provider name shown in tools (e.g. pi's `/model` badge, `name: lmstudio` → `[lmstudio]`)
  - Only takes effect when an endpoint is set; Copilot endpoints always register as `copilot`

### Fixed

- **`providerType: openai` contexts no longer register under pi's built-in `openai` provider id** — doing so replaced pi's built-in OpenAI model catalog for the session, made the `/model` badge ambiguous, and let pi prefer a stored `openai` credential from `~/.pi/agent/auth.json` over the context's key
  - The provider name is now derived from the endpoint hostname for **all** provider types (e.g. `aikeys` from `aikeys.maibornwolff.de`), matching the existing anthropic behavior; IP and `localhost` endpoints derive `local`
  - Derived names are not guarded against pi built-in ids (e.g. `openai.example.com` would still derive `openai`) — use `provider.name` to override
- **Responses-API models are no longer hidden in pi** — the generated extension's `/model/info` filter now admits `mode: "responses"` entries (registered as `openai-completions`, which a LiteLLM proxy transparently transforms), so chat-capable Responses models appear in pi's `/model` picker
- Provider copies now go through a single `Provider.Clone()` helper, so newly added provider fields can no longer be silently dropped by `aictx copy` / config save

## v0.2.1 (2026-07-01)

### Fixed

- **Per-model API type in the generated pi extension** — the non-Copilot extension previously assigned a single wire protocol (`api`) to every model it registered, so on a mixed-provider proxy (e.g. LiteLLM fronting both Anthropic Claude and OpenAI/Azure/Gemini) switching to a model whose real API differed from the context default sent requests in the wrong format and failed
  - The `api` is now derived **per model**: from `/model/info`'s `litellm_params.model` when available, otherwise from the model id (`claude`/`anthropic` → `anthropic-messages`, else `openai-completions`), applied across both live fetch paths and the static fallback
  - Switching between Claude and GPT/Gemini models via `/model` now works without manual context changes; the Copilot path is unchanged

## v0.2.0 (2026-07-01)

### Added

- **`aictx model`** — select the primary model for the active context
  - Fetches the available models live from the active context's provider endpoint (OpenAI-style `/v1/models` with `/models` fallback)
  - Interactive picker (pre-selects the current model) with a non-TTY scanner fallback for piped use
  - Persists the chosen model to `config.yaml` and regenerates the pi extension immediately
  - Guards reject Copilot contexts (managed via `aictx copilot login`) and native/OAuth contexts without a provider endpoint
- **Dynamic model list in the generated pi extension** — for non-Copilot contexts, `~/.pi/agent/extensions/aictx-provider.ts` is now an async factory that fetches all models at pi startup
  - Prefers `/model/info` (deriving cost per-million, context window, max tokens, reasoning, and vision capabilities), falling back to `/v1/models` then `/models`
  - Registers every model with pi; defaults to the config model and exposes the rest via `/model`
  - Bounded by a ~5s `AbortController` timeout, falling back to the static `model` / `smallModel` entries so pi always starts
- New `internal/models` package with `FetchModelIDs` and a `Dedup` helper (strips `vendor:` prefixes, treats dashed versions as dotted, prefers the dotted unprefixed representative); the same dedup rule is mirrored in the generated extension

## v0.1.2 (2026-04-20)

### Added

- **OAuth / Anthropic account context switching** — manage multiple Claude accounts alongside API-key contexts
  - `aictx add --oauth <name>` — captures the active Claude session's OAuth credentials from the macOS Keychain (or `.credentials.json` on Linux/Windows) and the `oauthAccount` metadata from `~/.claude.json`; stores everything in the aictx keyring
  - On context switch, writes the stored OAuth credentials back to Claude's native Keychain entries (all matching entries) and `.credentials.json`, and restores `oauthAccount` in `~/.claude.json` so `/config` shows the correct account
  - Switching away from an OAuth context cleanly removes credentials; switching to an OAuth context from a non-OAuth one won't accidentally delete the user's native session
  - `aictx discover` detects active OAuth sessions and captures credentials automatically
  - `aictx copy` and `aictx rename` migrate OAuth credentials to the new context name
  - `aictx rm` cleans up the OAuth keyring entry
  - Cross-platform: macOS uses the `security` CLI for Keychain access; Linux/Windows fall back to `.credentials.json`
- `HasOAuthKey` field added to `Context` in `config.yaml`
- `IsOAuth` field added to `DiscoveryResult`
- New `internal/claudeauth` package with platform-specific `Read`, `Write`, `Remove` and shared `ReadAccountMeta` / `WriteAccountMeta` helpers
- New keyring helpers: `SetOAuth`, `GetOAuth`, `DeleteOAuth`, `SetOAuthMeta`, `GetOAuthMeta`, `DeleteOAuthMeta`

## v0.1.1 (2026-04-11)

### Added

- **GitHub Copilot provider** for the pi Coding Agent CLI
  - `aictx copilot login` — OAuth 2.0 Device Flow authentication; stores the permanent OAuth token in the OS keychain
  - `aictx copilot status` — shows login state, username, and Copilot contexts
  - `aictx copilot logout` — removes the stored OAuth token and clears login metadata
  - On every `aictx <context>` switch, the stored OAuth token is automatically exchanged for a fresh 30-minute Copilot API token and applied to the pi extension file
  - The generated pi extension registers a `"copilot"` provider with `api: "openai-completions"` and the four required Copilot API headers
  - Non-pi-cli targets in a Copilot context are skipped with a warning (Copilot API is OpenAI-compatible only; Claude Code requires Anthropic format)
- `CopilotLogin` field added to `Config` struct (stored in `config.yaml`) to persist username and login timestamp
- Keyring helpers `SetCopilotOAuth`, `GetCopilotOAuth`, `DeleteCopilotOAuth`, `IsCopilotLoggedIn` in `internal/keyring`

## v0.1.0 (2026-04-08)

### Breaking changes

**Config schema: per-context provider model**

`Provider`, `Options`, and `HasKeyringKey` have moved from per-`TargetEntry` to the `Context` level. All targets within a context now share one API key and model.

Old format (per-target):
```yaml
contexts:
  - name: work
    targets:
      - id: claude-code-cli
        provider:
          endpoint: https://api.example.com
          model: claude-opus-4-6
        options:
          alwaysThinking: true
        hasKeyringKey: true
```

New format (per-context):
```yaml
contexts:
  - name: work
    provider:
      endpoint: https://api.example.com
      model: claude-opus-4-6
    options:
      alwaysThinking: true
    hasKeyringKey: true
    targets:
      - id: claude-code-cli
      - id: claude-code-vscode
```

**Keyring account key changed**

Old: `aictx / contextName/targetID`
New: `aictx / contextName`

### Migration

Migration is **automatic and transparent**. On first run after upgrading, `aictx` detects the old schema and migrates in place:

1. Lifts the first non-empty `Provider` found across targets to the context level
2. Lifts `Options` to the context level
3. Migrates keyring entries from `contextName/targetID` to `contextName`
4. Saves the updated config

If targets had different API keys, a warning is printed to stderr and the first non-empty key is kept. Re-run `aictx copy <ctx> <ctx> --api-key <new-key>` to update.

#### Back up your API keys before upgrading

The migration moves keyring entries automatically, but if something goes wrong you want your keys handy. Run the script for your platform before upgrading and save the output somewhere safe (local file, password manager note, etc.).

**macOS** — uses the `security` CLI (built-in):

```sh
#!/usr/bin/env sh
# Reads context names from config.yaml and dumps each keyring entry.
CONFIG="$HOME/.config/aictx/config.yaml"
echo "=== aictx keyring backup (macOS) ==="
grep "^  - name:" "$CONFIG" | sed 's/.*name: //' | while read -r ctx; do
  # Old format: contextName/targetID
  for tid in claude-code-cli claude-code-vscode pi-cli; do
    key=$(security find-generic-password -s aictx -a "$ctx/$tid" -w 2>/dev/null)
    [ -n "$key" ] && echo "account=$ctx/$tid  key=$key"
  done
  # New format: contextName (already migrated)
  key=$(security find-generic-password -s aictx -a "$ctx" -w 2>/dev/null)
  [ -n "$key" ] && echo "account=$ctx  key=$key"
done
```

**Linux** — uses `secret-tool` (install via `apt install libsecret-tools` / `dnf install libsecret`):

```sh
#!/usr/bin/env sh
CONFIG="$HOME/.config/aictx/config.yaml"
echo "=== aictx keyring backup (Linux) ==="
grep "^  - name:" "$CONFIG" | sed 's/.*name: //' | while read -r ctx; do
  for tid in claude-code-cli claude-code-vscode pi-cli; do
    key=$(secret-tool lookup service aictx account "$ctx/$tid" 2>/dev/null)
    [ -n "$key" ] && echo "account=$ctx/$tid  key=$key"
  done
  key=$(secret-tool lookup service aictx account "$ctx" 2>/dev/null)
  [ -n "$key" ] && echo "account=$ctx  key=$key"
done
```

**Windows** — uses PowerShell and Windows Credential Manager:

```powershell
# Run in PowerShell. Reads config.yaml and dumps each Credential Manager entry.
$config = "$env:APPDATA\..\Local\aictx\config.yaml"
if (-not (Test-Path $config)) { $config = "$env:HOME\.config\aictx\config.yaml" }

Write-Host "=== aictx keyring backup (Windows) ==="
$contexts = (Get-Content $config | Select-String "^  - name:").Matches.Value -replace ".*name: "

Add-Type -AssemblyName System.Runtime.WindowsRuntime
[Windows.Security.Credentials.PasswordVault,Windows.Security.Credentials,ContentType=WindowsRuntime] | Out-Null
$vault = New-Object Windows.Security.Credentials.PasswordVault

foreach ($ctx in $contexts) {
    foreach ($account in @("$ctx/claude-code-cli", "$ctx/claude-code-vscode", "$ctx/pi-cli", $ctx)) {
        try {
            $cred = $vault.Retrieve("aictx", $account)
            $cred.RetrievePassword()
            Write-Host "account=$account  key=$($cred.Password)"
        } catch {}
    }
}
```

### New features

- **`aictx targets [contextname]`** — Checkbox multi-select picker to add or remove targets from a context. Pre-checks currently configured targets; detected targets are labelled. Falls back to a plain list when stdout is not a terminal.
- **`aictx version`** — Prints `aictx v0.1.0`.
- **pi CLI target** (`pi-cli`) — Support for the pi Coding Agent CLI.
- **`picker.PickMulti`** — Internal checkbox picker reused by `targets` and `add`.

### Changes

- `aictx add` interactive mode now uses the checkbox picker for target selection (detected targets pre-checked). Provider and Options are prompted once at the context level.
- `aictx copy` `--target` flag now only scopes `--env` overrides; provider/options flags always apply at the context level.
- `aictx rename` and `aictx rm` now operate on a single context-level keyring entry.

### Notes for users who script against the config YAML

The `targets[].provider` and `targets[].options` fields are removed. Scripts reading or writing `config.yaml` directly must be updated to use the top-level `provider` and `options` fields on each context.
