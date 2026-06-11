---
sidebar_position: 3
title: Providers
---

# Providers

A **provider** is a target AI-coding tool that graft reads from and writes to. Each provider has its own file layout and schema; graft hides those differences behind one interface.

## What graft supports

graft targets ten providers, defined in the frozen `Provider` contract (`internal/contract`):

| Provider id | Tool |
|-------------|------|
| `claude-code` | Claude Code |
| `codex` | Codex |
| `gemini-cli` | Gemini CLI |
| `cursor` | Cursor |
| `github-copilot` | GitHub Copilot |
| `opencode` | OpenCode |
| `roo-code` | Roo Code |
| `goose` | Goose |
| `grok-cli` | Grok CLI |
| `antigravity` | Antigravity |

:::info Phased delivery
Each provider is its own package and is implemented one at a time. Per-provider documentation pages are added as each provider's parser/serializer and schema land. The provider id strings above are frozen in the contract; individual provider capabilities are documented as they ship. See the [Providers overview](../providers/overview.md).
:::

## What a provider does

Every provider implements the same interface:

- **Detect** — find this provider's agent files under the workspace root.
- **Parse** — read one provider file into a provider-shaped form.
- **ToCanonical** — map a parsed provider agent into the neutral canonical form, preserving non-canonical fields under `providerOverrides`.
- **Serialize** — render a canonical agent into this provider's file(s), restoring stashed overrides.
- **Schema** — return the provider's JSON Schema for validation.

Because every provider speaks this one interface, the sync engine and transform registry treat them uniformly.

## Enabling a subset

You do not have to sync all ten. The `providers.enabled[]` config selects which providers participate. See [Config reference](../reference/config.md).

## Related

- [Providers overview](../providers/overview.md)
- [Canonical store](./canonical-store.md)
- [How sync works](./how-sync-works.md)
