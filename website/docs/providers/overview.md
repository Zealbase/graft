---
sidebar_position: 1
title: Providers overview
description: Index of all AI-coding providers graft supports — Claude Code, Codex, Cursor, GitHub Copilot, and more.
---

# Providers overview

graft syncs canonical agents out to **eight active** AI-coding providers, with two more planned. This page is the index; per-provider pages are added as each provider's package lands.

## Supported providers

| Provider id | Tool | Status |
|-------------|------|--------|
| `claude-code` | Claude Code | Active |
| `codex` | Codex | Active |
| `cursor` | Cursor | Active |
| `github-copilot` | GitHub Copilot | Active |
| `opencode` | OpenCode | Active |
| `roo-code` | Roo Code | Active |
| `goose` | Goose | Active |
| `grok-cli` | Grok CLI | Active |

## Skills support

Three of the eight active providers support skills (symlink-based canonical skill directories):

| Provider id | Tool | Project skills dir |
|-------------|------|--------------------|
| `claude-code` | Claude Code | `.claude/skills/` |
| `opencode` | OpenCode | `.opencode/skills/` |
| `codex` | Codex | `.codex/skills/` |

The remaining providers do not have a skills concept and are silently skipped by `graft skill` commands. Other tools in the AI-coding space are adding skills support; graft will wire up additional providers as their schemas stabilize.

## What every provider page will cover

Each provider follows the same interface (Detect, Parse, ToCanonical, Serialize, Schema), so each page documents:

- Where the provider keeps its agent files.
- Which canonical fields it maps directly.
- Which provider-specific fields it preserves via `providerOverrides`.
- Any validation rules from its JSON Schema.

## Enabling providers

Choose which providers participate with `providers.mode` and `providers.enabled[]` / `providers.disabled[]`. See [Config reference](../reference/config.md).

## Planned

Not yet wired into the sync engine — present in the embedded catalog only.

| Provider id | Tool | Status |
|-------------|------|--------|
| `antigravity` | Antigravity | Catalog only — unregistered in sync engine |
| `gemini-cli` | Gemini CLI | Catalog only — dewired per maintainer request (2026-06-15) |

:::note antigravity
antigravity has a catalog entry (schema, models, capabilities) but is currently unregistered in the sync engine pending a research spike on the agent-definition format. It is excluded from sync, agent, and skill operations until that work is done.
:::

:::note gemini-cli
gemini-cli has a catalog entry (schema, models, capabilities) but was **dewired** from the sync engine per maintainer request on 2026-06-15. The catalog code is kept as reference. Until it is re-registered it is excluded from sync, agent, and skill operations, and its skills directory (`.gemini/skills/`) is not managed by `graft skill`.
:::

## Related

- [Providers concept](../concepts/providers.md)
- [Canonical store](../concepts/canonical-store.md)
- [Skills](../concepts/skills.md)
