---
sidebar_position: 1
title: Providers overview
---

# Providers overview

graft syncs canonical agents out to ten AI-coding providers. This page is the index; per-provider pages are added as each provider's package lands.

:::info Phased delivery
Provider packages are implemented one at a time. The provider **ids** are frozen in the `Provider` contract; a dedicated page per provider (file layout, supported fields, `providerOverrides` it preserves) is published as each ships.
:::

## Supported providers

| Provider id | Tool | Page |
|-------------|------|------|
| `claude-code` | Claude Code | _planned_ |
| `codex` | Codex | _planned_ |
| `gemini-cli` | Gemini CLI | _planned_ |
| `cursor` | Cursor | _planned_ |
| `github-copilot` | GitHub Copilot | _planned_ |
| `opencode` | OpenCode | _planned_ |
| `roo-code` | Roo Code | _planned_ |
| `goose` | Goose | _planned_ |
| `grok-cli` | Grok CLI | _planned_ |
| `antigravity` | Antigravity | _planned_ |

## What every provider page will cover

Each provider follows the same interface (Detect, Parse, ToCanonical, Serialize, Schema), so each page documents:

- Where the provider keeps its agent files.
- Which canonical fields it maps directly.
- Which provider-specific fields it preserves via `providerOverrides`.
- Any validation rules from its JSON Schema.

## Enabling providers

Choose which providers participate with `providers.enabled[]`. See [Config reference](../reference/config.md).

## Related

- [Providers concept](../concepts/providers.md)
- [Canonical store](../concepts/canonical-store.md)
