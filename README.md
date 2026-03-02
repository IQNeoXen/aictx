# aictx

> **Note:** This project was largely created using LLMs (Claude). Use at your own discretion.

A context switcher for AI tools. Inspired by [kubectx](https://github.com/ahmetb/kubectx), but for AI tool configurations.

Switch between API keys, endpoints, models and other settings across multiple AI tools with a single command.

## Supported Targets

- [x] Claude Code CLI
- [x] Claude Code for VSCode
- [ ] Roo Code for VSCode
- [ ] Cline for VSCode
- [ ] GitHub Copilot CLI
- [ ] GitHub Copilot for VSCode

Each target translates abstract provider settings into its own config format. PRs welcome!

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
| `aictx rm <name>`          | Remove a context                                         |
| `aictx show [name]`        | Show context details (defaults to current)               |
| `aictx current`            | Print the current context name                           |
| `aictx discover`           | Detect config from installed tools and save as a context |
| `aictx completion <shell>` | Print a shell completion script                          |

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
  --env OPENAI_API_VERSION=2024-02-01 \
  --env MY_CUSTOM_FLAG=enabled
```

The `--env` flag is repeatable and stores arbitrary environment variables alongside the context. They are applied to the target's config on switch and cleaned up on the next switch.

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

## Showing a Context

```bash
aictx show            # current context (API key, header values, and env var values masked)
aictx show work       # specific context
aictx show --reveal   # show full API key, header values, and env var values
```

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
