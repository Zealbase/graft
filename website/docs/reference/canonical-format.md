---
sidebar_position: 3
title: Canonical format
description: Complete reference for the on-disk YAML format of a graft canonical agent — all fields, types, defaults, and provider-override rules.
---

# Canonical format reference

The on-disk format of a canonical agent. This is the reference companion to the [Canonical store](../concepts/canonical-store.md) concept page.

:::note Schema authority
The exact `agent.yaml` schema is `internal/canonical/schema/common-agent-definition.schema.json`. Its public `$id` is:

```
https://raw.githubusercontent.com/Shaik-Sirajuddin/graft/main/internal/canonical/schema/common-agent-definition.schema.json
```

Point your editor's JSON Schema association at this URL for live validation and completion. The fields below are the **frozen contract vocabulary** (`CanonicalAgent` in `internal/contract`). The JSON Schema is authoritative for exact keys/types/defaults and enums.
:::

## Directory layout

```
.graft/agents/<name>/
├─ agent.yaml        # canonical fields (provider-neutral)
├─ instructions.md   # body / system prompt
└─ .meta.json        # per-provider source hash + last commit hash
```

## `agent.yaml` fields

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Agent identifier. Not overridable via `providerOverrides`. |
| `description` | string | Short description. Must be non-empty before sync runs. |
| `model` | string | Default model id. Default `inherit` (parent/session model). |
| `tools` | string[] or map | Allowed tools. Array form: canonical tool names or wildcard patterns. Map form: `{tool: bool}` (OpenCode-style). See [Tool names](#tool-names). |
| `mcpServers` | object | MCP servers scoped to this agent. Each key is a server name; value is the server config. |
| `permissionMode` | string | Permission/autonomy posture (`default`, `acceptEdits`, `ask`, `bypassPermissions`, …). |
| `background` | bool | Run as non-blocking background task. |
| `readonly` | bool | Restrict to read-only. |
| `maxTurns` | int | Cap on agentic turns. |
| `providerOverrides` | object | Per-provider values with no canonical home. Restored verbatim on serialize. Keys are registered provider ids; values are validated against that provider's schema. |
| `skills` | string[] | Skills to preload with this agent. graft writes these into the per-agent file for claude-code (YAML frontmatter) and codex (`[[skills.config]]` TOML). Other providers do not have a per-agent skills field. |
| `temperature` | number | Sampling temperature (0+). |
| `timeoutMins` | number | Max execution time in minutes. |

The agent **body** (system prompt) lives in `instructions.md`, not in `agent.yaml`.

## Tool names

The `tools` field in `agent.yaml` uses **canonical tool names** — a `lowercase_snake_case` taxonomy shared across all providers:

```yaml
tools:
  - read_file
  - grep
  - web_search
  - bash
```

The JSON Schema enumerates all valid canonical names. An unrecognized name is a validation error. Wildcards and MCP patterns are always accepted:

| Pattern | Meaning |
|---------|---------|
| `*` | All tools |
| `mcp_*` | All MCP tools |
| `mcp__server__tool` | Specific MCP tool |
| `Agent(...)` | Agent-spawn syntax |

The full canonical → native mapping is in `internal/catalog/data/canonical-tools.md`. On apply, graft translates each canonical name to the native spelling for each provider (e.g. `web_search` → Claude `WebSearch`, Gemini `google_web_search`, OpenCode `websearch`).

## `providerOverrides` rules

`providerOverrides` lets you set provider-specific fields that the canonical model does not have a home for. The key must be a recognized provider id.

The schema is **schema-bound**: `providerOverrides` uses `additionalProperties: false` so only the eight active registered provider ids are valid keys, and each value is validated against that provider's own schema (`$defs/po-<provider>`). Editors with JSON Schema support will validate keys and offer field completion.

- **`name` is excluded**: the serialization layer enforces this — a `providerOverrides[p]["name"]` entry produces a warning and is silently dropped, never written. The schema omits `name` from every `po-<provider>` definition.
- **Unknown provider key → error**: an unrecognized key blocks sync. graft uses Levenshtein distance to suggest the nearest valid provider id ("did you mean ...?").
- **Field validation → warning**: override values are checked against the provider's catalog schema. Unrecognized fields produce a warning (never blocking), since catalog schemas may be incomplete.

## Per-provider model override

Use `graft agent model` to set or clear a per-provider model override without editing `agent.yaml` by hand:

```bash
graft agent model reviewer --provider claude-code --model claude-opus-4
graft agent model reviewer --provider claude-code --clear
```

This writes `providerOverrides.claude-code.model` in `agent.yaml`.

## Per-provider tool override

You can override the tool list for one provider via `providerOverrides[<provider>].tools`, independently of the canonical `tools` field. Use **native** tool names in override entries (the schema for each provider enumerates the accepted values):

```yaml
name: reviewer
description: Reviews diffs for correctness and style.
tools:
  - read_file
  - grep
providerOverrides:
  claude-code:
    tools:
      - Read
      - Grep
      - WebSearch      # native claude-code name
  opencode:
    tools:
      - read
      - grep
      - websearch      # native opencode name
```

Provider tool overrides are validated against the per-provider schema, which enumerates native tool names for that provider. Wildcards (`*`, `mcp_*`) and `Agent(...)` are always accepted.

## `.meta.json`

Tracks per-provider content hashes used for change detection and drift classification. See [Change detection](../concepts/change-detection.md) for the full hash architecture.

## Example

```yaml
# .graft/agents/reviewer/agent.yaml
name: reviewer
description: Reviews diffs for correctness and style.
model: claude-sonnet-4
tools:
  - read_file     # canonical name — graft translates to native spelling on apply
  - grep
providerOverrides:
  claude-code:
    color: blue        # provider-specific field; no canonical home — preserved on round-trip
    tools:
      - Read           # per-provider tool override; uses native claude-code name
      - Grep
  opencode:
    model: openai/gpt-4o   # per-provider model override; uses provider/model form
```

```markdown
<!-- .graft/agents/reviewer/instructions.md -->
You are a code reviewer. Focus on correctness first, then style.
```

## Related

- [Canonical store](../concepts/canonical-store.md)
- [Providers](../concepts/providers.md)
- [Change detection](../concepts/change-detection.md)
- [CLI reference — agent model](./cli.md#graft-agent-model-name)
