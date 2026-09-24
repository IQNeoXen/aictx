---
date: 2026-09-24T08:57:20.607306+00:00
git_commit: ca5ef0c095cdda64d165dfed8bb45446962788bb
branch: main
topic: "Key-update flow for a context whose API key has been rolled"
tags: [research, codebase, contexts, api-key, keyring, cobra, targets]
status: complete
---

# Research: Context API-Key Update Flow

## Research Question

> i want to implement a key-update flow where a user can update the api key for a given context, e.g. if the key was rolled. check what needs to be done for this.

## Summary

An API key is a shared, context-level value: `Context.Provider.APIKey`. On disk, the configuration keeps only `hasKeyringKey`; the actual key is stored under the context name in the operating-system keychain. `config.Load` hydrates the in-memory API key and `config.Save` writes a non-empty in-memory API key back to the same keychain account, then scrubs it from YAML.

The existing command surface has two ways to supply a static API key:

- `aictx add <name> --api-key <key>` creates a new context.
- `aictx copy <source> <new-name> --api-key <key>` creates a new context with an override.

There is no registered command that changes `Provider.APIKey` on an existing named context. A key-update flow therefore connects to the existing command-registration pattern, context lookup, context-level provider mutation, keyring-backed `config.Save`, and—if immediate propagation is part of the command behavior—the existing target-application path.

The earlier research document `2026-04-08-target-management-and-context-api-key-model.md` described the pre-migration per-target key model at commit `4cee3f9`. The current commit has since moved the provider/key to `Context`, added target management, OAuth, Copilot, and model-selection flows; this document reflects the current model.

### Relevant file tree

```text
aictx/
├── cmd/
│   ├── root.go          command registration and switch/apply orchestration
│   ├── add.go           new context creation with --api-key
│   ├── copy.go          new context copy with --api-key override
│   ├── model.go         active-context mutation + immediate pi regeneration pattern
│   ├── show.go          masked/revealed context credential display
│   ├── rename.go        context/keyring account migration
│   └── rm.go            context/keyring account removal
├── internal/
│   ├── config/
│   │   ├── types.go     Context, Provider, and TargetEntry model
│   │   └── config.go    keyring hydration, persistence, and atomic YAML writes
│   ├── keyring/
│   │   └── keyring.go   OS-keychain API-key Set/Get/Delete wrappers
│   └── target/
│       ├── claudecli/   maps a key to ANTHROPIC_AUTH_TOKEN
│       ├── claudevscode/ maps a key to VS Code Claude environment variables
│       └── picli/       embeds a key in the generated pi provider extension
└── README.md            user-facing add/copy/keychain documentation
```

## Detailed Findings

### 1. API-key ownership and persisted representation

`Context` owns a `Provider`, `Options`, and `HasKeyringKey`; `TargetEntry` retains its target ID and target-specific environment map. `Provider.APIKey` is the shared in-memory value used for each target in a context (`internal/config/types.go:11-34`).

```text
Config
└── Context "work"
    ├── Provider.APIKey        in memory after Load
    ├── HasKeyringKey: true    in config.yaml after a successful save
    └── Targets[]              IDs + target-specific Env

OS keychain
└── service: "aictx", account: "work" → API key
```

The keyring API is context-name based: `Set(contextName, apiKey)`, `Get(contextName)`, and `Delete(contextName)` (`internal/keyring/keyring.go:9-29`). This is also why `rename` and `rm` handle a single context-level keyring account.

### 2. Existing load and save lifecycle

`config.Load` performs the following for every context (`internal/config/config.go:124-177`):

1. Reads `config.yaml` and runs the legacy per-target-to-context migration.
2. If `provider.apiKey` is present in YAML, writes it to the context's keychain account, marks `HasKeyringKey`, and keeps the value in memory for the current process.
3. If the YAML context has `HasKeyringKey` but no plaintext API key, reads the context's keychain account into `ctx.Provider.APIKey` in memory.

`config.Save` deep-copies the configuration and, for each context with a non-empty in-memory API key, calls `keyring.Set(ctx.Name, ctx.Provider.APIKey)`, marks the copied context's `HasKeyringKey`, clears `provider.apiKey` in the copy, marshals YAML, and atomically renames `config.yaml.tmp` to `config.yaml` (`internal/config/config.go:180-215`). The caller's in-memory API key remains available after `Save`.

Consequently, assigning a non-empty replacement value to an already-loaded context's `Provider.APIKey` and saving it uses the same named OS-keychain account as the prior value.

### 3. Current command mutation patterns

The root command registers all top-level commands in `cmd/root.go:48-62`; the current list does not include a dedicated key-update command.

`aictx add` accepts `--api-key`, assigns it into the new context's `Provider`, appends the context, and calls `config.Save` (`cmd/add.go:40-53`, `cmd/add.go:82-88`, `cmd/add.go:254-257`). Interactive creation prompts once for the context-level API key (`cmd/add.go:139-148`).

`aictx copy` provides the nearest existing key-override behavior. It loads the source context, deep-copies it under a distinct new name, and when `--api-key` was explicitly supplied changes `dst.Provider.APIKey`, sets `dst.HasKeyringKey` false before saving, then appends the new context (`cmd/copy.go:105-120`, `cmd/copy.go:182-185`). Its `Args` declaration requires two distinct positional arguments, so it does not update the source in place (`cmd/copy.go:27-45`).

`aictx model` is the existing in-place mutation command. It finds `cfg.State.Current`, changes `ctx.Provider.Model`, saves config, and then calls `applyPiTarget` to regenerate the pi extension (`cmd/model.go:27-106`). `applyPiTarget` builds the context provider plus target environment into the effective target entry used by pi (`cmd/model.go:109-135`).

### 4. Context application and key propagation

Normal context application is centralized in `switchContext` (`cmd/root.go:103-251`). After selecting the named context, it loops over its targets and constructs an effective `TargetEntry` with:

```go
Provider:      resolvedProvider, // normally ctx.Provider, including APIKey
Options:       ctx.Options,
HasKeyringKey: ctx.HasKeyringKey,
Env:           te.Env,
```

(`cmd/root.go:180-189`). It calls `Target.Apply` for detected targets (`cmd/root.go:160-205`), then saves switch state.

The target implementations consume `effective.Provider.APIKey` as follows:

| Target | Current key output |
| --- | --- |
| Claude Code CLI | `ANTHROPIC_AUTH_TOKEN` in `~/.claude/settings.json` (`internal/target/claudecli/claudecli.go:78-95`) |
| Claude Code VS Code | `ANTHROPIC_AUTH_TOKEN` in the Claude Code environment-variable settings (`internal/target/claudevscode/claudevscode.go:84-101`) |
| pi Coding Agent CLI | literal `apiKey` and `authHeader: true` in generated `~/.pi/agent/extensions/aictx-provider.ts` for a real non-empty key (`internal/target/picli/picli.go:232-253`) |

Thus, the existing full propagation operation is a normal context switch. The model command separately demonstrates a narrower immediate-apply path for pi only.

### 5. Provider modes relevant to an update flow

The static API-key path is distinct from native/OAuth and Copilot contexts:

- An empty `Provider` represents native authentication or OAuth (`internal/config/types.go:56-59`). Claude OAuth credentials are handled through `HasOAuthKey` and separate keyring entries during switching (`cmd/root.go:134-155`).
- Copilot contexts have `ProviderType == "copilot"`; switch resolution creates a provider without a static `APIKey`, and the generated pi extension obtains tokens through its OAuth refresh callback (`cmd/root.go:116-132`). The model command explicitly rejects this provider type (`cmd/model.go:41-44`).
- A keyless custom endpoint causes pi to receive the fixed `"aictx-local"` placeholder rather than a real credential so that pi exposes the endpoint's models (`internal/target/picli/picli.go:20-23`, `internal/target/picli/picli.go:246-253`).

### 6. Existing automated coverage and documentation anchors

Config-level tests use a mock OS keyring through `zalando/go-keyring` and an isolated `XDG_CONFIG_HOME` (`internal/config/config_test.go:197-202`). Existing coverage establishes that `Save` removes a static API key from YAML while retaining it in the caller's memory (`internal/config/config_test.go:377-406`) and that a subsequent `Load` hydrates a keyring-backed key (`internal/config/config_test.go:408-458`).

Command tests currently provide a shared `executeCmd` helper in `cmd/rename_test.go:9-15`. The model tests provide the closest current command-mutation pattern: they write a temporary config, invoke `modelRun`, reload configuration, and verify the persisted provider model and generated pi extension (`cmd/model_test.go:164-245`).

README documentation currently describes `aictx copy` as the way to make a new context with a different API key, including the `aictx copy mycontext another-context --api-key sk-xxx` example (`README.md:234-254`). The context display command masks an API key unless `--reveal` is passed (`cmd/show.go:13-75`).

## Current-Flow Map

```text
static key supplied to an existing in-memory Context
    │
    ▼
Context.Provider.APIKey = replacement
    │
    ▼
config.Save(cfg)
    │
    ├── keyring.Set("aictx", contextName, replacement)
    ├── disk Context.HasKeyringKey = true
    └── atomic config.yaml rewrite with provider.apiKey omitted

later config.Load() or current process's context application
    │
    ▼
Context.Provider.APIKey available in memory
    │
    ▼
switchContext() builds effective TargetEntry per configured target
    │
    ├── Claude CLI / VS Code: ANTHROPIC_AUTH_TOKEN
    └── pi: generated extension apiKey/authHeader
```

## Code References

| File | Lines | Current responsibility |
| --- | --- | --- |
| `cmd/root.go` | 48-62 | Registers top-level commands; no API-key update command is registered. |
| `cmd/root.go` | 103-251 | Context switch and target-application orchestration. |
| `cmd/add.go` | 40-53, 82-88, 254-257 | Accepts an API key while creating and saving a new context. |
| `cmd/copy.go` | 27-45, 105-120, 182-185 | Overrides a key only while copying to a new context name. |
| `cmd/model.go` | 27-106, 109-135 | Existing active-context mutation and immediate pi-only reapplication pattern. |
| `internal/config/types.go` | 11-59 | Context-level provider/API-key data model and empty-provider semantics. |
| `internal/config/config.go` | 124-177 | YAML/keyring load and in-memory key hydration. |
| `internal/config/config.go` | 180-215 | Keyring save, YAML secret scrubbing, and atomic config write. |
| `internal/keyring/keyring.go` | 9-29 | Context-name based API-key keyring operations. |
| `internal/target/claudecli/claudecli.go` | 78-95 | Claude CLI API-key environment mapping. |
| `internal/target/claudevscode/claudevscode.go` | 84-101 | VS Code API-key environment mapping. |
| `internal/target/picli/picli.go` | 232-253 | Generated pi-provider API-key mapping. |
| `internal/config/config_test.go` | 377-458 | API-key scrubbing and keyring-hydration tests. |
| `README.md` | 234-254 | Current public API-key-copy documentation. |

## Architecture Documentation

The codebase uses a single shared provider and key per context. Commands work on a loaded `*config.Config`, locate or construct a `Context`, mutate its context-level fields, and persist it through `config.Save`. `Save` is the boundary between the in-memory credential and its keychain-backed persisted form. The switch path later merges the shared provider with target-specific environment variables and delegates output formatting to the selected target implementation.

For a rolled static credential, the current architecture already provides the storage lifecycle and the target mappings; the absent piece is the Cobra command/handler that locates an existing context and assigns its replacement key. Current propagation behavior is available through `switchContext`; the model command supplies a separate precedent for saving a mutation and immediately regenerating only the pi target.

## Open Questions

The existing source does not establish these command-interface decisions:

1. Whether a new command addresses an explicitly named context, defaults to `State.Current`, or supports both forms.
2. Whether its replacement key is supplied only with a flag, interactively, or by both input modes.
3. Whether successful persistence should reapply every configured detected target immediately, only reapply pi as `model` does, or leave application to the next `aictx <context>` switch.
4. How the command should treat contexts using Claude OAuth, the Copilot provider type, or an intentionally keyless endpoint rather than a static API key.
5. Whether an empty key is valid command input. Current `config.Save` only writes a keyring entry for a non-empty `Provider.APIKey`; it does not itself delete an existing keyring entry for an empty value (`internal/config/config.go:193-203`).
