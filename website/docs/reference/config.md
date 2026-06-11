---
sidebar_position: 2
title: Config reference
---

# Config reference

graft's configuration. Read and write it with `graft config get/set`.

:::info Planned
Config keys reflect plan 03 (global config). Exact key paths, types, and defaults are confirmed against the config implementation (`internal/config`, koanf) once it lands; treat keys below as the planned surface.
:::

## Global config

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `sync.gitAuto` | bool | — | Auto-commit graft's tracking branches vs. using the builtin-git path only. |
| `scope` | string | `agents` | Which capability is synced. `agents` today; `skills` and `slash` are **planned**. |
| `providers.enabled[]` | string[] | — | Subset of the ten providers that participate in sync. |

## Provider ids

`providers.enabled[]` accepts these ids:

`claude-code`, `codex`, `gemini-cli`, `cursor`, `github-copilot`, `opencode`, `roo-code`, `goose`, `grok-cli`, `antigravity`

See [Providers](../concepts/providers.md).

## Usage

```bash
graft config get scope
graft config set scope agents
graft config set sync.gitAuto true
```

## Global vs project config

graft keeps global configuration plus per-project config under each `.graft/`. Where the implementation separates the two, this page will document them separately (root config vs per-project overrides). For now, the keys above are the global surface.

## Related

- [CLI reference](./cli.md)
- [Providers](../concepts/providers.md)
