---
sidebar_position: 1
title: CLI reference
---

# CLI reference

Every graft command, its purpose, and its output.

:::warning Stub — auto-generated later
This page is a hand-written **stub**. The authoritative CLI reference will be **auto-generated from the cobra command tree after phase h** (the CLI/gateway phase), so command names, flags, and defaults come from the registration code rather than prose. Until then, the surface below tracks plan 03 and the frozen `EntryGate` contract. Flags marked **planned** are not guaranteed final.
:::

All commands route **CLI → EntryGate** only. graft is agents-first.

## Command summary

| Command | Does | Output |
|---------|------|--------|
| `graft init` | Create `.graft/`, register the workspace row, detect git mode. | Created path. |
| `graft agent list` | List canonical agents and their provider coverage. | Table. |
| `graft agent <name> status` | Drift of one agent across providers. | Per-provider in/out-of-sync. |
| `graft agents status` | Aggregated drift: providers out of sync + agent counts. | Summary table. |
| `graft sync agent <name>` | Run sync for one agent. | `run_id` + result. |
| `graft sync agents` | Run sync for all changed agents. | `run_id` + result. |
| `graft validate` | Schema + semantic validation before sync. | Findings; non-zero exit on error. |
| `graft config get/set` | Read or write global/project config. | Value. |

## `graft init`

Creates the `.graft/` store, registers a workspace row in graft's sqlite database, and detects the git mode (`tracked` or `internal`).

```bash
graft init
```

## `graft agent list`

Lists canonical agents and provider coverage as a table.

```bash
graft agent list
```

## `graft agent <name> status`

Shows whether each provider is in sync with the canonical for one agent.

```bash
graft agent <name> status
```

## `graft agents status`

Aggregated drift across the workspace: which providers are out of sync and how many agents drifted on each.

```bash
graft agents status
```

## `graft sync agent <name>` / `graft sync agents`

Run a sync for one agent or for everything that changed. Each validates first, runs the merge engine, writes enabled providers, and prints a `run_id`.

```bash
graft sync agent <name>
graft sync agents
```

Flags (**planned**):

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Show what would change without writing. |
| `--continue` | bool | `false` | Resume an open conflict run for this workspace. |

## `graft validate`

Runs schema + semantic validation. Exits non-zero on error-severity findings.

```bash
graft validate --all
graft validate --provider <provider-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | — | Validate all enabled providers. |
| `--provider <id>` | string | — | Validate against one provider. |

Validation also runs implicitly before every sync.

## `graft config get/set`

Read or write configuration. See [Config reference](./config.md) for keys.

```bash
graft config get <key>
graft config set <key> <value>
```

## Related

- [Config reference](./config.md)
- [Canonical format](./canonical-format.md)
- [How sync works](../concepts/how-sync-works.md)
