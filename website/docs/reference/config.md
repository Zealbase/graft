---
sidebar_position: 2
title: Config reference
description: Reference for graft's global and per-project config files — all keys, types, defaults, and how to read or write them with graft config.
---

# Config reference

graft has two configuration layers: a **global** config at the XDG data path and a **per-project** config at `.graft/config.json`. Read and write them with `graft config get` / `graft config set`.

## Global config

The global config lives at `~/.local/share/graft/config.json` (XDG data home). It is the base layer; all fields fall back to it when the project config has no override.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `scope` | string | `agents` | Which capability is synced: `agents`, `skills`, or `slash`. |
| `sync.gitAuto` | bool | `false` | Auto-commit graft's tracking branches vs. using the builtin-git path only. |
| `providers.mode` | string | `all` | Provider selection: `all` (all supported providers except disabled) or `specific` (only `providers.enabled`). |
| `providers.enabled[]` | string[] | — | Active providers when `mode=specific`. |
| `providers.disabled[]` | string[] | — | Excluded providers when `mode=all`. |
| `theme` | string | `dark` | Color theme: `dark`, `dark-dim`, `light`, or `colorblind`. |
| `skills.enabled` | bool | `true` | Master switch for the init/sync skill-apply hook. |
| `skills.autoInstall` | bool | `false` | Install missing referenced skills without prompting (equivalent to `--yes`). |
| `skills.providers[]` | string[] | — | Restrict the skill hook to these supporting provider ids. Empty = all supporting providers. |

## Per-project config

The per-project config lives at `.graft/config.json` inside the workspace. It travels with the repo and overrides global provider selection and scope for that project. Global-only keys (theme, skills, sync.gitAuto) have no project meaning.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `scope` | string | (global) | Override synced capability for this project. |
| `providers.mode` | string | (global) | Override provider mode for this project. |
| `providers.enabled[]` | string[] | (global) | Override active providers for this project. |
| `providers.disabled[]` | string[] | (global) | Override excluded providers for this project. |

When a project sets `providers`, the project value wins entirely; the global effective set is not merged.

## Provider ids

The supported provider id strings (use in `providers.enabled[]` / `providers.disabled[]`):

| Provider id | Tool | Notes |
|-------------|------|-------|
| `claude-code` | Claude Code | |
| `codex` | Codex | |
| `gemini-cli` | Gemini CLI | |
| `cursor` | Cursor | |
| `github-copilot` | GitHub Copilot | |
| `opencode` | OpenCode | |
| `roo-code` | Roo Code | |
| `goose` | Goose | |
| `grok-cli` | Grok CLI | |
| `antigravity` | Antigravity | Catalog entry present; unregistered in sync engine (pending research spike). |

See [Providers](../concepts/providers.md).

## Usage examples

```bash
# View the resolved config for this project
graft config get

# View global config only
graft config get -g

# Set scope globally
graft config set -g --scope agents

# Restrict sync to specific providers (project)
graft config set --providers.mode specific --providers.enabled claude-code,gemini-cli

# Exclude a provider (project, mode=all)
graft config set --providers.mode all --providers.disabled goose

# Toggle skills hook off globally
graft config set -g --skills.enabled false

# Change color theme globally
graft config set -g --theme dark-dim
```

## Config resolution order

1. **Project config** (`.graft/config.json`) — provider selection and scope, when set.
2. **Global config** (`~/.local/share/graft/config.json`) — everything else.

`graft config get` (no `-g`) shows the resolved view: what a sync would actually use.

## Related

- [CLI reference](./cli.md)
- [Providers](../concepts/providers.md)
- [skill command reference](./skill-command.md)
