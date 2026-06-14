---
sidebar_position: 3
title: Providers
description: The AI-coding providers graft syncs to — Claude Code, Codex, Cursor, Gemini CLI, and more.
---

# Providers

A **provider** is a target AI-coding tool that graft reads from and writes to. Each provider has its own file layout and schema; graft hides those differences behind one interface.

## What graft supports

graft targets **nine active** providers, defined in the frozen `Provider` contract (`internal/contract`), with one more planned:

| Provider id | Tool | Active |
|-------------|------|--------|
| `claude-code` | Claude Code | Yes |
| `codex` | Codex | Yes |
| `gemini-cli` | Gemini CLI | Yes |
| `cursor` | Cursor | Yes |
| `github-copilot` | GitHub Copilot | Yes |
| `opencode` | OpenCode | Yes |
| `roo-code` | Roo Code | Yes |
| `goose` | Goose | Yes |
| `grok-cli` | Grok CLI | Yes |

## Planned

Not yet wired into the sync engine — present in the embedded catalog only.

| Provider id | Tool | Status |
|-------------|------|--------|
| `antigravity` | Antigravity | Catalog only — unregistered, pending research spike |

:::note antigravity
antigravity has a catalog entry (schema, models, capabilities) but is currently **not registered** in the sync engine. The agent-definition format and home-scope paths need a research spike before it can be wired up. Until then it is excluded from `graft sync`, `graft agent`, and provider-count summaries. It will be re-registered once the format is confirmed.
:::

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

Rules:

- `name` is **never** overridable via `providerOverrides` — it is the agent's identity and is enforced at the serialization layer.
- An unknown provider key under `providerOverrides` is an **error** (blocks sync). graft uses Levenshtein distance to suggest the nearest valid provider id.
- Override values are validated against the provider's catalog schema. Unrecognized fields produce a **warning** (never blocking) because catalog schemas may be incomplete.

## Enabling a subset

You do not have to sync all providers. `providers.mode` and `providers.enabled[]` / `providers.disabled[]` control which providers participate. See [Config reference](../reference/config.md).

## Related

- [Providers overview](../providers/overview.md)
- [Canonical store](./canonical-store.md)
- [How sync works](./how-sync-works.md)
- [Config reference](../reference/config.md)
