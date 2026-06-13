---
sidebar_position: 5
title: Destroy a workspace
---

# Destroy a workspace

`graft destroy` tears down graft's managed state for the current workspace. Your provider agent files (`.claude/`, `.codex/`, `.cursor/`, etc.) are **never touched**.

## What it removes

By default, `graft destroy` removes:

- The `.graft/` directory (config, db artifacts, lock).
- The workspace row and all associated run, branch, and agent rows from the global sqlite database.
- The workspace lock file.

Provider files are always preserved.

## How to use

Interactive (prompts for confirmation):

```bash
graft destroy
```

Non-interactive (CI / scripted):

```bash
graft destroy --yes
graft destroy --ci
```

Keep the canonical store (`.graft/agents/`) but drop everything else:

```bash
graft destroy --keep-store
```

With `--keep-store`, the canonical `agent.yaml` and `instructions.md` files survive. You can re-initialize the workspace later with `graft init` and the store will be picked up again.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip the confirmation prompt. |
| `--ci` | bool | `false` | Non-interactive (alias for `--yes`). |
| `--keep-store` | bool | `false` | Retain `.graft/agents/` (canonical store). |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

## After destroying

To reinitialize graft in the same directory:

```bash
graft init
```

If you used `--keep-store`, existing canonical agents will be detected and can be synced out again.

## Related

- [CLI reference — graft destroy](../reference/cli.md#graft-destroy)
- [Install — uninstall](../getting-started/install.md#uninstall)
