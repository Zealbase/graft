---
sidebar_position: 7
title: Change detection
description: How graft uses content hashes in .meta.json to classify each agent as ingest, fan-out, merge, or no-op on every sync.
---

# Change detection

graft uses content hashes stored in a `.meta.json` sidecar to detect what changed — and on which side — before touching any provider file. This is what makes sync fast: most agents are a no-op in under a millisecond.

## The sidecar: `.graft/agents/<name>/.meta.json`

Each canonical agent directory contains a `.meta.json` file alongside `agent.yaml` and `instructions.md`. It holds two families of hashes:

| Field | Scope | Value |
|-------|-------|-------|
| `canonicalHash` | top-level | `sha256` of the canonical agent content (field-sorted, normalized — cosmetic whitespace changes never shift it) |
| `providers.<id>.sourceHash` | per provider | `sha256` of that provider's file on disk at the last sync |
| `providers.<id>.canonicalHash` | per provider | the `canonicalHash` value at the time this provider file was last written |
| `providers.<id>.lastCommitHash` | per provider | git commit SHA when `sourceHash` was recorded (provenance only — not used for sync decisions) |

All hash values are plain hex SHA-256 of file content. `lastCommitHash` is the only git-derived value.

## How hashes classify each agent

On every sync graft computes two comparisons per agent:

1. **Has the provider file changed?** — compare `sha256(provider file on disk)` against the recorded `sourceHash`.
2. **Has the canonical changed?** — compare `sha256(canonical content)` against the recorded `canonicalHash`.

Those two bits determine the action:

| Provider changed | Canonical changed | Action |
|-----------------|-------------------|--------|
| Yes | No | **Ingest** — pull provider edit into canonical, fan out to all providers |
| No | Yes | **Fan-out** — write canonical to all providers |
| Yes | Yes | **3-way merge** (git beta worktree) |
| No | No | **No-op** — already in sync |

## Subset-sync staleness healing

When you sync only a subset of providers (e.g. `--providers=claude-code`), opencode's file is not rewritten. Its `sourceHash` still matches its on-disk file, so it would look "in sync" on the next run — but its `providers.opencode.canonicalHash` differs from the current `canonicalHash`, revealing it was last rendered from an older canonical.

On the next full sync, graft detects this staleness and force-rewrites opencode's file to match the current canonical. This is how subset syncs stay self-healing.

## `.meta.json` is a derived cache

`.meta.json` is a cache that can be reconstructed from the committed files:

- `sourceHash` and `canonicalHash` are recomputable from files already in the repo.
- `lastCommitHash` is not recomputable from files alone; it re-stamps to the current HEAD on first sync.
- The **merge ancestor** for 3-way merges is the git-committed canonical state, not `.meta.json`. A fresh clone without `.meta.json` stays merge-safe.

**Fresh-clone behavior:**

| Scenario | Result |
|----------|--------|
| `.meta.json` present | True no-op when nothing changed |
| `.meta.json` absent | graft rewrites provider files to byte-identical content and regenerates `.meta.json`. No data loss. |
| Absent + a provider edit disagrees with canonical | The edit is preserved and promoted (not silently dropped) |
| Absent + canonical and provider both edited to different values | A surfaced, resumable git conflict (no silent data loss) |

graft commits `.meta.json` by default (it is not gitignored). This keeps the first sync after a `git pull` a true no-op rather than a full identical-content rewrite, and avoids spurious conflicts.

## Related

- [How sync works](./how-sync-works.md)
- [Canonical store](./canonical-store.md)
- [Drift and status](./drift-and-status.md)
- [Canonical format reference](../reference/canonical-format.md)
