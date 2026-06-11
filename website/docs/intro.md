---
slug: /
sidebar_position: 1
title: Introduction
---

# graft

graft keeps **one canonical agent definition** in sync across the AI-coding tools you use. You edit an agent once; graft propagates that change to every provider — and pulls a change made in any single provider back to the canonical source and out to the rest.

:::info Status
graft is under active development. This documentation tracks the implemented contract in `internal/contract` and the build plan. Anything not yet wired up is marked **planned**.
:::

## Why

If you use several AI-coding assistants, each keeps its own agent/instruction files in its own layout (`.claude`, `.codex`, `.cursor`, …). Editing the same agent in ten places by hand drifts immediately. graft makes the canonical store under `.graft/` the single source of truth and uses a git branch/worktree merge engine to keep providers equal.

## What it does

- Stores a provider-neutral agent under `.graft/agents/<name>/`.
- Transforms canonical ⇄ provider in **both directions** — a change at any provider can propagate to all.
- Syncs through a deterministic, resumable, concurrency-safe merge engine.
- Reports **drift**: which providers are out of sync, for which agents.
- Validates schema and semantics **before** every sync.

## What it is not (yet)

- Skills and slash-command sync are **planned** — graft is agents-first.
- No git hooks: migration and sync run only when you invoke `graft sync`.
- No TUI or web UI.

## Start here

1. [Quickstart](./getting-started/quickstart.md) — from install to your first sync.
2. [Install](./getting-started/install.md) — install, verify, upgrade, uninstall.
3. [Core concepts](./concepts/overview.md) — the canonical-first mental model.
4. [CLI reference](./reference/cli.md) — every command.
