# aictx

> **Note:** This project was largely created using LLMs (Claude). Use at your own discretion.

A context switcher for AI tools. Inspired by [kubectx](https://github.com/ahmetb/kubectx), but for AI tool configurations.

Switch between API keys, endpoints, models and other settings across multiple AI tools with a single command.

## Supported Targets

- [x] Claude Code CLI
- [x] Claude Code for VSCode
- [x] pi Coding Agent CLI
- [ ] Roo Code for VSCode
- [ ] Cline for VSCode
- [ ] GitHub Copilot CLI
- [ ] GitHub Copilot for VSCode

Each target translates abstract provider settings into its own config format. PRs welcome!

## Supported Providers

- [x] **Anthropic** (direct or via proxy) — all targets
- [x] **OpenAI-compatible** (custom endpoint) — pi CLI
- [x] **GitHub Copilot** (OAuth Device Flow) — pi CLI only (see [GitHub Copilot](#github-copilot))

## Install

### Homebrew (macOS / Linux)

```bash
brew tap IQNeoXen/aictx
brew install aictx
```

> **macOS Gatekeeper warning:** On macOS 15 (Sequoia) and later, Apple may block the binary with a "cannot be opened because the developer cannot be verified" dialog. To unblock it:
>
> **Option A — Terminal:** Run `xattr -d com.apple.quarantine $(which aictx)` in your terminal.
>
> **Option B — System Settings:**
> Open **System Settings → Privacy & Security**, scroll down to the Security section, and click **"Open Anyway"** next to the `aictx` message.

### From source (go install)

Requires [Go](https://go.dev/doc/install) 1.21+.

```bash
go install github.com/IQNeoXen/aictx@latest
```

`go install` places the binary in `~/go/bin`. If that's not in your PATH, add it:

```bash
# Bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc

# Zsh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc

# Fish
fish_add_path ~/go/bin
```

### Build from source

```bash
git clone https://github.com/IQNeoXen/aictx.git
cd aictx
go build -o aictx .
```

### Linux prerequisite

API keys are stored in the system keychain via libsecret. Install it before using `aictx`:

```bash
# Debian / Ubuntu
sudo apt-get install libsecret-1-0

# Fedora / RHEL
sudo dnf install libsecret
```

## Quick Start

```bash
# Discover existing config from installed tools
aictx discover

# Or create a context manually
aictx add work \
  --target claude-code-cli \
  --endpoint https://proxy.example.com \
  --api-key sk-xxx \
  --model claude-opus-4-6 \
  --thinking

# Switch contexts
aictx work

# Interactive picker (arrow keys + enter)
aictx
```

## Commands

| Command                    | Description                                              |
| -------------------------- | -------------------------------------------------------- |
| `aictx`                    | Pick context interactively or list when piped            |
| `aictx <name>`             | Switch to a context                                      |
| `aictx -`                  | Switch back to the previous context                      |
| `aictx list`               | List all contexts                                        |
| `aictx add <name>`         | Add a new context (interactive or with flags)            |
| `aictx copy <src> <name>`  | Copy a context, optionally overriding settings           |
| `aictx rename <old> <new>` | Rename a context (alias: `mv`)                           |
| `aictx rm <name>`          | Remove a context                                         |
| `aictx show [name]`        | Show context details (defaults to current)               |
| `aictx current`            | Print the current context name                           |
| `aictx discover`           | Detect config from installed tools and save as a context |
| `aictx model`              | Pick the primary model for the active context (live list) |
| `aictx completion <shell>` | Print a shell completion script                          |
| `aictx copilot login`      | Authenticate with GitHub Copilot via Device Flow         |
| `aictx copilot status`     | Show GitHub Copilot login status                         |
| `aictx copilot logout`     | Remove stored Copilot credentials                        |

## Switching Contexts

```bash
aictx work        # switch by name
aictx -           # switch back to previous
aictx             # interactive picker (requires a terminal)
```

## Listing Contexts

```bash
aictx list              # human-readable, active context marked with *
aictx list --names-only # one name per line, useful in scripts
```

## Inspecting the Current Context

```bash
aictx current              # print context name
aictx current --json       # full context as JSON (API key masked)
aictx current --json --reveal  # include full API key
aictx current --env        # print export KEY=value lines (for eval)
```

`--env` output can be used to apply settings to a shell session:

```bash
eval "$(aictx current --env --reveal)"
```

## Adding a Context

**With flags:**

```bash
aictx add work-project \
  --target claude-code-cli \
  --target claude-code-vscode \
  --endpoint https://proxy.example.com \
  --api-key sk-xxx \
  --model claude-opus-4-6 \
  --small-model claude-haiku-4-5 \
  --thinking \
  --no-telemetry \
  --command "sandbox start" \
  --env OPENAI_API_VERSION=2024-02-01 \
  --env MY_CUSTOM_FLAG=enabled
```

The `--env` flag is repeatable and stores arbitrary environment variables alongside the context. They are applied to the target's config on switch and cleaned up on the next switch.

Use `--command` to run a shell command automatically every time this context is selected:

```bash
aictx add sandbox \
  --target claude-code-cli \
  --command "sandbox start"

aictx add work \
  --target claude-code-cli \
  --endpoint https://proxy.example.com \
  --api-key sk-xxx \
  --command "burn stop"
```

The command runs via `$SHELL -c` after all targets are applied, and its output is streamed to the terminal.

Use `--header` to pass custom HTTP headers required by LiteLLM proxies or other endpoints (e.g. `X-Proxy-Auth`, `X-Team-ID`). Headers are encoded as a JSON object in the `ANTHROPIC_CUSTOM_HEADERS` environment variable that Claude Code reads natively:

```bash
aictx add work-project \
  --target claude-code-cli \
  --endpoint https://proxy.example.com \
  --api-key sk-xxx \
  --header X-Proxy-Auth:token123 \
  --header X-Team-ID:eng
```

**Interactively** (run `aictx add <name>` without flags):

```txt
Description: Work proxy config
Command to run on switch (leave empty to skip): burn stop
Available targets:
  [1] Claude Code CLI (claude-code-cli) (detected)
  [2] Claude Code for VSCode (claude-code-vscode) (detected)
Select targets (comma-separated numbers): 1,2

--- Claude Code CLI (claude-code-cli) ---
Provider (leave empty for native auth / OAuth):
  Endpoint URL: https://proxy.example.com
  API Key: sk-xxx
  Model: claude-opus-4-6
  ...
Options:
  ...
Custom headers (leave name empty to finish):
  Header name: X-Proxy-Auth
  Value: token123
  Header name:
Custom env vars (leave name empty to finish):
  Name: OPENAI_API_VERSION
  Value: 2024-02-01
  Name:
```

To change headers or env vars after creation, edit `~/.config/aictx/config.yaml` directly.

## Copying a Context

`aictx copy` clones an existing context to a new name. Only the flags you explicitly provide override the copy — everything else is inherited from the source.

```bash
# Same settings, different API key
aictx copy mycontext another-context --api-key sk-xxx

# Different endpoint and model
aictx copy mycontext staging --endpoint https://staging.api.example.com --model claude-haiku-4-5

# Add env vars on top of what the source already has
aictx copy prod dev --no-telemetry --env DEBUG=1

# Override only a specific target, leave others untouched
aictx copy mycontext another --api-key sk-xxx --target claude-code-cli
```

The `--env` and `--header` flags **merge** into the inherited values rather than replacing them.
API keys are copied to the OS keychain under the new context name automatically.

## Renaming a Context

`aictx rename` (alias: `mv`) renames a context in-place. State references (current/previous) and keyring entries are updated automatically.

```bash
aictx rename old-name new-name
aictx mv old-name new-name        # same thing
```

Errors if the old name does not exist, the new name is already taken, or both names are identical.

## Showing a Context

```bash
aictx show            # current context (API key, header values, and env var values masked)
aictx show work       # specific context
aictx show --reveal   # show full API key, header values, and env var values
```

## Selecting a Model

`aictx model` fetches the models available from the **active context's**
provider endpoint, shows an interactive picker (pre-selecting the current
model), persists your choice to `~/.config/aictx/config.yaml`, and immediately
regenerates the pi extension so the new default takes effect.

```bash
aictx trustedzone-opus   # switch to a context first
aictx model              # pick a new primary model from the live list
```

When stdout is not a terminal (piped), it prints the available models and reads
your selection from stdin, defaulting to the current model on empty input.

> `aictx model` is not supported for Copilot contexts (managed via
> `aictx copilot login`) or for native/OAuth contexts with no provider endpoint.

### Dynamic model list in the pi extension

The aictx-generated pi extension (`~/.pi/agent/extensions/aictx-provider.ts`)
for non-Copilot contexts is an **async factory** that queries the provider
endpoint for **all** available models at pi startup and registers each one with
pi (deriving cost, context window, and capabilities from `/model/info` where
available). pi defaults to the model in your aictx config, and you can switch to
any other model on the fly via `/model`. If the endpoint is slow or unreachable,
the extension falls back (after a ~5s timeout) to the static `model` /
`smallModel` entries from the config, so pi always starts.

## Shell Completion

```bash
# Fish
aictx completion fish > ~/.config/fish/completions/aictx.fish

# Bash
aictx completion bash >> ~/.bashrc

# Zsh
aictx completion zsh > "${fpath[1]}/_aictx"

# PowerShell
aictx completion powershell >> $PROFILE
```

Tab completion works for context names on `aictx`, `aictx show`, and `aictx rm`.

## Security

API keys are stored in the OS keychain — never in plain text on disk:

| Platform | Storage                    |
| -------- | -------------------------- |
| macOS    | Keychain                   |
| Linux    | libsecret / GNOME Keyring  |
| Windows  | Windows Credential Manager |

The config file (`~/.config/aictx/config.yaml`) stores only metadata: context names, endpoints, models, and options.

## GitHub Copilot

aictx supports **GitHub Copilot as a provider** for the pi Coding Agent CLI via a standard OAuth 2.0 Device Flow. A one-time `aictx copilot login` wizard handles authentication; every subsequent context switch automatically exchanges the stored OAuth credential for a fresh short-lived Copilot API token.

> **Note:** Claude Code targets are not supported with the Copilot provider — the Copilot API uses the OpenAI-compatible format, while Claude Code requires the Anthropic API format. Only the pi CLI target is supported.

### Login

```bash
aictx copilot login
```

This will:
1. Start the GitHub Device Flow and print a URL + code to authorize
2. Wait for you to open the URL in a browser and enter the code
3. Verify your Copilot subscription
4. Let you pick a default model and an optional small model
5. Create a context named `github-copilot` (or a name you choose)
6. Switch to the new context immediately

The OAuth token is stored in your OS keychain. Only login metadata (username, login time) is written to `~/.config/aictx/config.yaml`.

### Switching

```bash
aictx github-copilot
```

On each switch, aictx exchanges the stored OAuth token for a fresh 30-minute Copilot API token and writes it to the pi extension file. The output shows when the token expires:

```
  ↺ Copilot token refreshed (expires in 30m)
  ✓ pi Coding Agent CLI
Switched to github-copilot
```

> **Token TTL:** Copilot API tokens expire after ~30 minutes. If pi reports an auth error during a session, re-run `aictx github-copilot` to refresh the token.

### Status

```bash
aictx copilot status
```

Shows the current login state, username, login time, and which contexts use the Copilot provider.

### Logout

```bash
aictx copilot logout
```

Removes the OAuth token from the keychain and clears login metadata from the config. Copilot contexts remain in the config but will fail to switch until you log in again.

### Config example

```yaml
copilotLogin:
  username: philip.brembeck
  loggedInAt: "2026-04-11T14:30:00Z"

contexts:
  - name: github-copilot
    provider:
      endpoint: https://api.githubcopilot.com
      model: gpt-4o
      smallModel: gpt-4o-mini
      providerType: copilot   # triggers OAuth token exchange on switch
    targets:
      - id: pi-cli
```

The generated pi extension (`~/.pi/agent/extensions/aictx-provider.ts`) registers a `"copilot"` provider with `api: "openai-completions"` and the required Copilot API headers.

## Config

Stored at `~/.config/aictx/config.yaml` (API keys are in the keychain, not here):

```yaml
state:
  current: work

contexts:
  - name: work
    description: "LiteLLM proxy for work"
    targets:
      - id: claude-code-cli
        provider:
          endpoint: https://proxy.example.com
          model: claude-opus-4-6
          smallModel: claude-haiku-4-5
          headers:
            X-Proxy-Auth: token123
            X-Team-ID: eng
        options:
          alwaysThinking: true
          disableTelemetry: true
        env:
          OPENAI_API_VERSION: "2024-02-01"
        hasKeyringKey: true

      - id: claude-code-vscode
        provider:
          endpoint: https://proxy.example.com
          model: claude-sonnet-4-6
        options:
          alwaysThinking: true
        hasKeyringKey: true

  - name: personal
    description: "Personal Claude subscription (OAuth)"
    targets:
      - id: claude-code-cli
        options:
          alwaysThinking: true
      - id: claude-code-vscode
        options:
          alwaysThinking: true
```

An empty provider (no endpoint/apiKey) means native auth / OAuth.
