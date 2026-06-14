---
sidebar_position: 5
title: Drift and status
description: What drift means in graft — when a provider's agent file diverges from canonical — and how status commands surface it.
---

# Drift and status

**Drift** is when a provider's agent file no longer matches what graft expects. Status commands report drift so you know what a sync would change before you run it.

## What it does

graft tracks, per provider, a content hash of each agent file (the `PROVIDER_LINK` record). To compute status, graft:

1. Loads the stored content hash for each provider link.
2. Recomputes the hash of the provider file on disk.
3. Compares: a provider is **in sync** when its file hash matches both the stored hash and the canonical; otherwise it is **drifted**.

```
in_sync  ⇔  provider-file-hash == stored-hash  AND  == canonical
```

## How to read status

Two views, both read-only — they never modify files:

- **Per agent** — `graft agent <name> status` shows each provider as in or out of sync for that one agent.
- **Aggregated** — `graft agents status` summarizes which providers are out of sync and how many agents drifted on each.

See the [Check status](../guides/check-status.md) guide and the [CLI reference](../reference/cli.md).

## What status reports

The aggregated report (`StatusReport` in the contract) carries:

- `agents` — each agent's per-provider in/out-of-sync map and an overall `in_sync` flag.
- `out_of_sync_providers` — a map of provider → number of agents drifted.

## Drift vs sync

Status only *reports* drift; it changes nothing. To reconcile drift, run a [sync](../guides/sync-an-agent.md). Every sync also runs validation first, so a sync can surface validation findings before it touches providers — see [Validate](../guides/validate.md).

## Related

- [How sync works](./how-sync-works.md)
- [Check status](../guides/check-status.md)
- [CLI reference](../reference/cli.md)
