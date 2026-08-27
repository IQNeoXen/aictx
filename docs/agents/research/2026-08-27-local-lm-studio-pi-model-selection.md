---
date: 2026-08-27T22:00:05.335778+00:00
git_commit: be00bac5be75491fd8281c0e2fd5c62e56596f38
branch: main
topic: "LM Studio local endpoint model discovery and selection in pi"
tags: [research, codebase, picli, lm-studio, local-endpoint, model-selection]
status: complete
---

# Research: LM Studio local endpoint model discovery and selection in pi

## Research Question

Why do the `local` and `gemma4` aictx contexts, both using the LM Studio endpoint `http://127.0.0.1:1234/v1`, cause pi to fall back to the Codex subscription and prevent selection of models returned by LM Studio?

## Summary

The current `local` activation did generate and load an `lmstudio` provider; it is not being registered as the built-in `openai` or `openai-codex` provider. Its generated extension requests exactly the three paths logged by LM Studio:

```text
http://127.0.0.1:1234/v1/model/info
http://127.0.0.1:1234/v1/v1/models
http://127.0.0.1:1234/v1/models
```

The final request is the successful LM Studio model-list endpoint. The extension maps that response into registered `lmstudio` models. However, the active `local` extension has no API key, OAuth configuration, or stored pi credential. Current pi only exposes models from *configured* providers in its `/model` selector; an extension provider without any configured credential is absent from that available-model snapshot.

Separately, the configured defaults do not match the models in the observed LM Studio response. `local` sets `qwen/qwen3.8-27b`, and `gemma4` sets `google/gemma4`, while the server response contains `meta/muse-glimmer` and `text-embedding-nomic-embed-text-v1.5`. Pi cannot resolve the configured default `(lmstudio, qwen/qwen3.8-27b)` to a registered model and consequently proceeds through its fallback model-resolution path, which can select an available Codex-subscription model.

## Key Files

```text
aictx/
├── cmd/
│   ├── root.go                       context switch → target application
│   └── model.go                      interactive model selection command
├── internal/
│   ├── models/models.go              OpenAI-style endpoint model-list helper
│   └── target/picli/picli.go         generated pi provider extension/settings
└── docs/agents/research/
    └── 2026-08-27-local-lm-studio-pi-model-selection.md

~/.config/aictx/config.yaml           local and gemma4 context definitions
~/.pi/agent/
├── extensions/aictx-provider.ts      generated active local provider
└── settings.json                     pi default provider/model
```

## Detailed Findings

### Context switch and provider identity

`switchContext` transfers the selected context's provider, options, and target environment into an effective `TargetEntry`, then calls the detected target's `Apply` method (`cmd/root.go:157-207`). `picli.Target.Apply` writes the extension and pi settings (`internal/target/picli/picli.go:52-64`).

Both local contexts specify `providerType: openai`, `endpoint: http://127.0.0.1:1234/v1`, `name: lmstudio`, and `pi-cli` as a target. Because an endpoint is present, `piProviderName` uses the explicit `name` value (`internal/target/picli/picli.go:127-135`). The active generated extension therefore calls:

```ts
pi.registerProvider("lmstudio", { ... })
```

The active settings file likewise contains:

```json
{
  "defaultProvider": "lmstudio",
  "defaultModel": "qwen/qwen3.8-27b"
}
```

This is separate from pi's `openai-codex` subscription provider.

### Request paths behind the LM Studio log

The extension generator preserves the configured endpoint as `baseUrl` and passes it to the embedded dynamic model helper (`internal/target/picli/picli.go:231-249`). That helper removes only a trailing slash (`:313`) and then tries:

1. `base + "/model/info"` (`:321`),
2. `base + "/v1/models"` (`:355-358`),
3. `base + "/models"` (`:355-358`).

With the configured `base` ending in `/v1`, these become `/v1/model/info`, `/v1/v1/models`, and `/v1/models`, respectively. This reproduces the recorded order exactly. The helper ignores unsuccessful or unparsable intermediate results and continues to later paths; when the final response has a nonempty `data[].id` list, it maps and registers those models (`internal/target/picli/picli.go:355-372`).

The standalone `aictx model` command follows a related path: `models.FetchModelIDs` queries `endpoint + "/v1/models"` and then `endpoint + "/models"` (`internal/models/models.go:113-133`). It therefore also reaches `/v1/v1/models` first and `/v1/models` as its fallback for this endpoint. On success it presents the returned IDs, saves the selection as `ctx.Provider.Model`, and regenerates the pi extension (`cmd/model.go:51-104`).

### Why fetched LM Studio models are absent from pi's selectable list

The active generated `aictx-provider.ts` invokes the helper with an empty API-key argument and registers `lmstudio` without an `apiKey`, `authHeader`, or OAuth section. Pi's `ModelSelector` builds its list from `modelRuntime.getAvailableSnapshot()` rather than from every registered model (`pi .../model-selector.js:104-108`).

Pi adds an extension provider to its configured/available-provider set only when it has stored credentials or an extension request-auth configuration (`pi .../model-runtime.js:555-590`). Its `configuredRequestAuthStatus` returns no status when neither the normal provider configuration nor the extension provides an API key (`pi .../provider-composer.js:382-393`). Thus the no-key active `local` extension can register models but does not make `lmstudio` a configured provider, so its models do not enter pi's available-model snapshot used by `/model`.

### Why pi does not start with the aictx local default

At startup, pi accepts `settings.defaultProvider` and `settings.defaultModel` only if it finds the exact model under that provider and finds that provider configured (`pi .../model-resolver.js:502-514`). The observed LM Studio response contains these IDs:

```text
meta/muse-glimmer
text-embedding-nomic-embed-text-v1.5
```

Neither appears in the `local` default `qwen/qwen3.8-27b` nor the `gemma4` default `google/gemma4` from `~/.config/aictx/config.yaml`. The exact local default lookup therefore has no registered match. Pi next attempts available models and, when any are available, uses its provider-default selection or the first available model (`pi .../model-resolver.js:516-533`). This explains the observed fallthrough to a Codex-subscription model while `lmstudio` is not in the available-model snapshot.

### Existing research currency

Two older reports cover related mechanics:

- `docs/agents/research/2026-07-01-model-selection-and-dynamic-pi-models.md`
- `docs/agents/research/2026-07-10-pi-provider-naming-openai-collision.md`

A history check found both superseded for this topic. Since their recorded commits, the current code added generic model fetching, the `aictx model` command, asynchronous dynamic extension fetching, per-model API type selection, and custom endpoint/provider naming.

## Code References

- `~/.config/aictx/config.yaml` — `local` and `gemma4` LM Studio endpoint/model values.
- `~/.pi/agent/extensions/aictx-provider.ts` — active provider registration and empty key configuration.
- `~/.pi/agent/settings.json` — active default `(lmstudio, qwen/qwen3.8-27b)`.
- `cmd/root.go:157-207` — effective target construction and `Apply` call.
- `cmd/model.go:51-104` — fetch IDs, picker, persist selection, regenerate extension.
- `internal/models/models.go:113-133` — `/v1/models` then `/models` construction.
- `internal/target/picli/picli.go:127-147` — explicit/derived provider naming.
- `internal/target/picli/picli.go:231-249` — extension dynamic-helper and provider registration generation.
- `internal/target/picli/picli.go:313-380` — dynamic endpoint paths, mapping, fallback.
- `pi .../core/provider-composer.js:382-393` — configured request-auth determination.
- `pi .../core/model-runtime.js:555-590` — configured-provider/available snapshot update.
- `pi .../modes/interactive/components/model-selector.js:104-108` — `/model` source is available snapshot.
- `pi .../core/model-resolver.js:502-533` — default lookup and fallback sequence.

## Architecture Documentation

```text
aictx local
  → switchContext
  → picli.Apply
  ├─ ~/.pi/agent/extensions/aictx-provider.ts
  │    → GET /v1/model/info
  │    → GET /v1/v1/models
  │    → GET /v1/models  → registerProvider("lmstudio", models)
  └─ ~/.pi/agent/settings.json
       → defaultProvider: lmstudio
       → defaultModel: qwen/qwen3.8-27b

pi startup
  → extension registers the endpoint models
  → availability snapshot retains configured providers only
  → default model lookup requires an exact registered, available model
  → unavailable/missing default falls through to another available provider
```

## Open Questions

- Whether the `gemma4` keychain entry supplies a nonempty API key when `config.Load` constructs its effective provider; the observed active `local` extension has no key.
- Whether LM Studio's two returned IDs are both intended for chat use. The aictx fallback mapper treats OpenAI-style `/models` entries generically and does not filter the embedding model.
