---
sidebar_position: 1
title: Sync an agent
---

# Sync an agent

Reconcile an agent across providers after you change it — anywhere.

:::info Planned
Commands reflect the planned CLI surface (plan 03). The sync engine behavior is described from the frozen contract and build plan.
:::

## What it does

A sync detects changed agent files, transforms them to canonical, merges, and writes the result back out to every enabled provider. It validates first and never commits to your base branch.

## How to use

Sync one agent:

```bash
graft sync agent <name>
```

Sync everything that changed:

```bash
graft sync agents
```

Each prints a `run_id` and a result. The result lists changed agents and any conflicts.

### Preview without writing

Use a dry run to see what would change (planned `--dry-run`):

```bash
graft sync agents --dry-run
```

### Resume an interrupted run

If a previous sync stopped on a conflict, continue it:

```bash
graft sync agents --continue
```

See [Resolve conflicts](./resolve-conflicts.md).

## Which providers are written

Only providers in `providers.enabled[]` participate. See [Config reference](../reference/config.md).

## How it works

Under the hood each changed file goes onto its own temporary branch, is canonicalized, merged into a moving beta branch, and — once stable — copied into your working tree and serialized to every provider. Full walkthrough: [How sync works](../concepts/how-sync-works.md).

## Related

- [Check status](./check-status.md)
- [Validate](./validate.md)
- [CLI reference](../reference/cli.md)
