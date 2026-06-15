---
sidebar_position: 3
title: Providers
description: The AI-coding providers graft syncs to — Claude Code, Codex, Cursor, and more.
---

# Providers

A **provider** is a target AI-coding tool that graft reads from and writes to. Each provider has its own file layout and schema; graft hides those differences behind one interface.

## What graft supports

graft targets **eight active** providers, defined in the frozen `Provider` contract (`internal/contract`), with two more planned:

| Provider id | Tool | Active |
|-------------|------|--------|
| `claude-code` | Claude Code | Yes |
| `codex` | Codex | Yes |
| `cursor` | Cursor | Yes |
| `github-copilot` | GitHub Copilot | Yes |
| `opencode` | OpenCode | Yes |
| `roo-code` | Roo Code | Yes |
| `goose` | Goose | Yes |
| `grok-cli` | Grok CLI | Yes |

## What a provider does

Every provider implements the same interface:

- **Detect** — find this provider's agent files under the workspace root.
- **Parse** — read one provider file into a provider-shaped form.
- **ToCanonical** — map a parsed provider agent into the neutral canonical form, preserving non-canonical fields under `providerOverrides`.
- **Serialize** — render a canonical agent into this provider's file(s), restoring stashed overrides.
- **Schema** — return the provider's JSON Schema for validation.

Because every provider speaks this one interface, the sync engine and transform registry treat them uniformly.

## providerOverrides

Providers carry settings that have no neutral home in the canonical model. These are stored under `providerOverrides[<provider>]` in `agent.yaml` and restored verbatim when serializing back to that provider.

### Schema binding

`providerOverrides` is **schema-bound per provider** in the canonical JSON Schema (`internal/canonical/schema/common-agent-definition.schema.json`):

- The top-level `providerOverrides` object uses `additionalProperties: false`, so only the registered provider ids are accepted as keys. An unknown key (typo or unregistered provider) causes a validation error — editors with JSON Schema support will underline it immediately.
- Each `providerOverrides[<provider>]` value is validated against the definition for that provider (`$defs/po-<provider>`). This means editors will offer completion and type-check the fields you set per provider.
- The `$id` of the schema is the public raw GitHub URL:
  ```
  https://raw.githubusercontent.com/Shaik-Sirajuddin/graft/main/internal/canonical/schema/common-agent-definition.schema.json
  ```
  Point your editor's JSON Schema association at this URL for live validation.

### Rules

- `name` is **never** overridable via `providerOverrides` — it is the agent's identity and is enforced at the serialization layer. The schema omits `name` from every `po-<provider>` definition.
- An unknown provider key under `providerOverrides` is an **error** (blocks sync). graft uses Levenshtein distance to suggest the nearest valid provider id.
- Override values are validated against the provider's schema. Unrecognized fields produce a **warning** (never blocking) because catalog schemas may be incomplete.

### Per-provider tool override

Each `po-<provider>` schema includes a `tools` field. You can override the tool list for a single provider without changing the canonical `tools`:

```yaml
# .graft/agents/reviewer/agent.yaml
name: reviewer
description: Reviews diffs for correctness and style.
tools:
  - read_file
  - grep
providerOverrides:
  claude-code:
    tools:
      - Read           # native claude-code name
      - Grep
      - WebSearch
  opencode:
    tools:
      - read
      - grep
      - websearch
```

The canonical `tools` field uses canonical names (see [Tool names and canonicalization](#tool-names-and-canonicalization) below). Provider tool overrides use the provider's **native** tool names — the schema for each provider enumerates the accepted values plus a wildcard pattern (`*`, `mcp_*`, `mcp__server__tool`, `Agent(...)`).

## Tool names and canonicalization

graft stores **canonical tool names** in `agent.yaml` using a `lowercase_snake_case` taxonomy. On sync, graft translates each canonical name into the native spelling for each provider:

| Canonical | Claude Code | Gemini CLI | OpenCode | Codex |
|-----------|-------------|------------|----------|-------|
| `web_search` | `WebSearch` | `google_web_search` | `websearch` | `web_search` |
| `read_file` | `Read` | `read_file` | `read` | — |
| `bash` | `Bash` | `run_shell_command` | `bash` | `shell` |
| `glob` | `Glob` | `glob` | `glob` | — |
| `grep` | `Grep` | `search_file_content` | `grep` | — |
| `web_fetch` | `WebFetch` | `web_fetch` | `webfetch` | — |
| `file_edit` | `Edit` | `edit` + `replace` | `edit` | — |
| `file_write` | `Write` | `write_file` | — | — |

The full taxonomy is in `internal/catalog/data/canonical-tools.md`. Use canonical names in the top-level `tools` field of `agent.yaml`. Use native names only inside `providerOverrides[<provider>].tools`.

The canonical `tools` array is validated against an enumerated set in the JSON Schema, so editors flag unrecognized canonical names. Wildcards (`*`), MCP patterns (`mcp_*`, `mcp__server__tool`), and `Agent(...)` pass through and are never rejected by the enum.

## Enabling a subset

You do not have to sync all providers. `providers.mode` and `providers.enabled[]` / `providers.disabled[]` control which providers participate. See [Config reference](../reference/config.md).

## Related

- [Providers overview](../providers/overview.md)
- [Canonical store](./canonical-store.md)
- [How sync works](./how-sync-works.md)
- [Config reference](../reference/config.md)

## Planned

Not yet wired into the sync engine — present in the embedded catalog only.

| Provider id | Tool | Status |
|-------------|------|--------|
| `antigravity` | Antigravity | Catalog only — unregistered, pending research spike |
| `gemini-cli` | Gemini CLI | Catalog only — dewired per maintainer request (2026-06-15) |

:::note antigravity
antigravity has a catalog entry (schema, models, capabilities) but is currently **not registered** in the sync engine. The agent-definition format and home-scope paths need a research spike before it can be wired up. Until then it is excluded from `graft sync`, `graft agent`, and provider-count summaries. It will be re-registered once the format is confirmed.
:::

:::note gemini-cli
gemini-cli has a catalog entry (schema, models, capabilities) but was **dewired** from the sync engine per maintainer request on 2026-06-15. The catalog code is kept as reference. Until it is re-registered it is excluded from `graft sync`, `graft agent`, and provider-count summaries, and its skills directory (`.gemini/skills/`) is not managed by `graft skill`.
:::
