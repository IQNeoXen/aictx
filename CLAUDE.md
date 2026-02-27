# aictx — AI Context Switcher

Context switcher for AI tool configurations, inspired by kubectx.

## Build & Run

```bash
go build -o aictx .
go run .
```

## Project Structure

- `cmd/` — Cobra CLI commands (root, add, rm, show, current, discover)
- `internal/config/` — Config types and load/save (`~/.config/aictx/config.yaml`)
- `internal/target/` — Target interface, registry, and implementations
  - `claudecli/` — Claude Code CLI target (`~/.claude/settings.json`)
  - `claudevscode/` — Claude Code for VSCode target (surgical edits to VSCode `settings.json`)
- `internal/fzf/` — Optional fzf fuzzy picker integration

## Architecture

- **Context**: Named configuration with a list of target entries
- **TargetEntry**: Per-target provider settings (endpoint, apiKey, model) and options (thinking, telemetry)
- **Target interface**: `ID()`, `Name()`, `Detect()`, `Apply(TargetEntry)`, `Discover()`
- Provider and Options live inside TargetEntry, not at Context level — different targets can have different settings

## Conventions

- Always commit changes after completing work
- Use atomic file writes (temp file + rename) for config files
- VSCode settings: only touch claude-specific keys, preserve everything else (use sjson/gjson)
- Empty provider = OAuth/native auth mode
