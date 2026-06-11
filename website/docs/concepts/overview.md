---
sidebar_position: 1
title: Overview
---

# Core concepts

graft has one job: keep a single canonical agent definition equal to its copies across every provider. This page gives you the mental model and the vocabulary the rest of the docs use.

## The canonical-first model

There is exactly one source of truth per agent: the **canonical** definition under `.graft/agents/<name>/`. Every provider holds a *projection* of that canonical agent in its own on-disk format. graft's job is to keep all projections equal to the canonical, in both directions:

- Edit the canonical → graft writes every provider.
- Edit a provider → graft pulls it back to canonical, then writes the rest.

## Glossary

| Term | Meaning |
|------|---------|
| **Canonical** | The provider-neutral agent doc under `.graft/agents/<name>/`. The source of truth. |
| **Provider** | A target tool (claude-code, codex, …) with its own file layout and schema. |
| **EntryGate** | The single object the CLI talks to; holds the store, engine, and locks. The CLI never calls the engine, store, or providers directly. |
| **Workspace** | The identity tuple `(root, remote, branch)`. One `.graft/` tree per workspace row. |
| **Sync run** | One tracked execution, recorded with a `run_id` in sqlite. |
| **Drift** | A provider's file no longer matches the stored/canonical hash. |
| **git mode** | `tracked` (uses your real git repo) or `internal` (graft keeps its own repo when none exists). |

## The pieces

- **[Canonical store](./canonical-store.md)** — the on-disk format of an agent.
- **[Providers](./providers.md)** — the target tools graft writes to.
- **[How sync works](./how-sync-works.md)** — the branch/worktree/merge engine.
- **[Drift and status](./drift-and-status.md)** — how graft decides what is out of sync.

## How the system is wired

The CLI calls only the **EntryGate**. The gate owns the store (sqlite), the sync engine, and concurrency locks. The engine uses a transform registry (canonical ⇄ provider), a git abstraction (`GitX`, with a go-git default and a shell fallback), and validation. Each provider is its own package behind one `Provider` interface.

This boundary is a hard rule: every command routes **CLI → EntryGate → everything else**.
