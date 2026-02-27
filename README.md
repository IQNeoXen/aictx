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

Requires [Go](https://go.dev/doc/install) (1.21+).

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

Or build from source:

```bash
git clone https://github.com/IQNeoXen/aictx.git
cd aictx
go build -o aictx .
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
  --model claude-opus-4.6 \
  --thinking

# Switch contexts
aictx work

# Interactive picker (arrow keys + enter)
aictx
```

## Commands

| Command             | Description                                              |
| ------------------- | -------------------------------------------------------- |
| `aictx`             | List contexts or pick interactively                      |
| `aictx <name>`      | Switch to a context                                      |
| `aictx -`           | Switch to the previous context                           |
| `aictx add <name>`  | Add a new context (interactive or with flags)            |
| `aictx rm <name>`   | Remove a context                                         |
| `aictx show [name]` | Show context details (defaults to current)               |
| `aictx current`     | Print the current context name                           |
| `aictx discover`    | Detect config from installed tools and save as a context |

## Config

Stored at `~/.config/aictx/config.yaml`:

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
          apiKey: sk-xxx
          model: claude-opus-4.6
          smallModel: claude-haiku-4.5
        options:
          alwaysThinking: true
          disableTelemetry: true

      - id: claude-code-vscode
        provider:
          endpoint: https://proxy.example.com
          apiKey: sk-xxx
          model: claude-sonnet-4.6
        options:
          alwaysThinking: true

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

## Adding a Context

**With flags:**

```bash
aictx add work-project \
  --target claude-code-cli \
  --target claude-code-vscode \
  --endpoint https://proxy.example.com \
  --api-key sk-xxx \
  --model claude-opus-4.6 \
  --small-model claude-haiku-4.5 \
  --thinking \
  --no-telemetry
```

**Interactively** (just run `aictx add mycontext` without flags):

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
  Model: claude-opus-4.6
  ...
```

## Showing a Context

```bash
# Show current context
aictx show

# Show a specific context (secrets masked by default)
aictx show work

# Reveal secrets
aictx show work --reveal
```

## Shell Completion

```bash
# Fish
aictx completion fish > ~/.config/fish/completions/aictx.fish

# Bash
aictx completion bash > /etc/bash_completion.d/aictx

# Zsh
aictx completion zsh > "${fpath[1]}/_aictx"
```

Tab completion works for context names on `aictx`, `aictx show`, and `aictx rm`.
