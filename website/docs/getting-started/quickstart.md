---
sidebar_position: 1
title: Quickstart
---

# Quickstart

The shortest path from nothing to your first sync. This walks through initializing graft in an existing repository, inspecting detected agents, editing one, and syncing it to every provider.

:::info Planned
The `graft` binary is built in the project's later phases. Commands below reflect the planned command surface (plan 03) and the frozen `EntryGate` contract. Until the CLI lands, treat this page as the intended flow.
:::

## 1. Install

See [Install](./install.md) for all methods. One common path:

```bash
go install <module-path>/cmd/graft@latest   # module path set at release
```

Verify:

```bash
graft --version
```

## 2. Initialize

Run inside an existing git repository:

```bash
graft init
```

This creates the `.graft/` canonical store, registers a workspace row in graft's sqlite database, and detects your git mode (`tracked` if a real repo is present, otherwise `internal`). It prints the created path.

## 3. See what was detected

```bash
graft agent list
```

You get a table of canonical agents and their coverage across providers.

## 4. Edit one agent

Open the canonical definition and change something:

```bash
$EDITOR .graft/agents/<name>/agent.yaml
$EDITOR .graft/agents/<name>/instructions.md
```

See [Canonical store format](../concepts/canonical-store.md) for the fields.

## 5. Sync

```bash
graft sync agents
```

graft validates the changed agents, runs the merge engine, writes the result to every enabled provider, and prints a `run_id` plus the outcome. To sync just one:

```bash
graft sync agent <name>
```

## 6. Confirm

```bash
graft agents status
```

All providers should report in sync.

## What just happened

graft diffed the working tree against your base branch, isolated the changed agent files onto temporary branches, merged them into the canonical store, then reapplied the canonical result out to every provider — without committing to your base branch. See [How sync works](../concepts/how-sync-works.md).

## Next

- [Core concepts](../concepts/overview.md)
- [Resolve conflicts](../guides/resolve-conflicts.md)
- [CLI reference](../reference/cli.md)
- [Config reference](../reference/config.md)
