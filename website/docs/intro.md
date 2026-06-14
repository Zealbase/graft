---
slug: /
sidebar_position: 1
title: Introduction
description: graft keeps one canonical agent definition in sync across all your AI-coding providers — Claude Code, Codex, Cursor, and more.
---

# graft

graft keeps **one canonical agent definition** in sync across the AI-coding tools you use. You edit an agent once; graft propagates that change to every provider — and pulls a change made in any single provider back to the canonical source and out to the rest.

## Why

If you use several AI-coding assistants, each keeps its own agent/instruction files in its own layout (`.claude`, `.codex`, `.cursor`, …). Editing the same agent in ten places by hand drifts immediately. graft makes the canonical store under `.graft/` the single source of truth and uses a git branch/worktree merge engine to keep providers equal.

## What it does

- Stores a provider-neutral agent under `.graft/agents/<name>/`.
- Transforms canonical ⇄ provider in **both directions** — a change at any provider can propagate to all.
- Syncs through a deterministic, resumable, concurrency-safe merge engine.
- Reports **drift**: which providers are out of sync, for which agents.
- Validates schema and semantics **before** every sync, with per-provider `providerOverrides` validation.
- Manages **skills**: a canonical store under `.agents/skills/` symlinked into supporting providers.
- Checks for and installs binary updates via `graft update`.

## Supported platforms

macOS and Windows are both supported. On Windows, symlink-based skill linking requires Developer Mode or Administrator privileges; without those, skill commands report a capability error rather than silently falling back to a file copy.

## What is not active yet

- No TUI or web UI.
- No git hooks: migration and sync run only when you invoke `graft sync`.

A planned provider (`antigravity`) is in the catalog but not yet wired into the sync engine — see [Providers → Planned](./concepts/providers.md#planned).

## Start here

1. [Quickstart](./getting-started/quickstart.md) — from install to your first sync.
2. [Install](./getting-started/install.md) — install, verify, upgrade, uninstall.
3. [Core concepts](./concepts/overview.md) — the canonical-first mental model.
4. [CLI reference](./reference/cli.md) — every command.

## Command for agents

AI agents and LLM-based tools can use these machine-readable indexes to navigate the graft documentation:

- [`/llms.txt`](/llms.txt) — a concise index with links and brief descriptions (following the [llmstxt.org](https://llmstxt.org) convention)
- [`/llms-full.txt`](/llms-full.txt) — the expanded variant with fuller descriptions and inlined content for comprehensive documentation consumption
