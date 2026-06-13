---
sidebar_position: 3
title: Check status and drift
---

# Check status and drift

See which providers are out of sync before you run anything. Status commands are read-only.

## What it does

Status compares each provider's on-disk agent file against graft's stored content hash and the canonical. It reports in-sync vs drifted without changing any files.

## How to use

List agents and their provider coverage:

```bash
graft agent list
```

Check one agent across all providers:

```bash
graft agent <name> status
```

Aggregated drift across the whole workspace:

```bash
graft agents status
```

All commands accept `-o json` or `-o yaml` for machine-readable output.

## Reading the output

- Per-agent status shows each provider as in or out of sync for that agent.
- Aggregated status summarizes which providers are out of sync and how many agents drifted on each (`out_of_sync_providers`).
- The sync summary includes a skill count when skills are enabled and canonical skills are present.

## How it works

graft loads the stored `content_hash` per provider link, recomputes the file hash, and marks the provider in sync only when both the stored hash and the canonical match. See [Drift and status](../concepts/drift-and-status.md).

## Related

- [Sync an agent](./sync-an-agent.md)
- [Drift and status](../concepts/drift-and-status.md)
- [CLI reference](../reference/cli.md)
