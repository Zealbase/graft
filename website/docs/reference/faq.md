---
sidebar_position: 4
title: FAQ & troubleshooting
description: Answers to the most common questions about graft — git commits, multi-workspace setups, conflict resolution, and provider support.
---

# FAQ & troubleshooting

## Does graft commit to my branch?

No. The merge runs on a temporary beta branch; the stabilized result is **copied** into your working tree without committing to your base branch. You commit on your own terms. See [How sync works](../concepts/how-sync-works.md).

## What if my project has no git repository?

graft uses an **internal** repo (`git_mode = internal`) and keeps working. The moment a real git repo is detected on a later sync, graft migrates to `tracked`. There are no git hooks — this happens only on `graft sync`.

## Can two syncs run at once?

Not on the same workspace. graft holds an exclusive lock per workspace `(root, remote, branch)`. A second sync waits for the first to finish, or resumes it if it stopped on a conflict.

## What counts as a "workspace"?

The tuple `(root, remote, branch)` — not just a directory. One repo can hold several `.graft/` sub-trees (separate workspaces), and the same branch in another worktree maps to the same workspace. See [Canonical store](../concepts/canonical-store.md).

## Where does graft store its database?

graft uses one **global** sqlite database with WAL plus locking for concurrency. The exact path is set by the store implementation (`internal/store`) and will be documented here once finalized (**planned**).

## Which providers are supported?

Eight active: `claude-code`, `codex`, `cursor`, `github-copilot`, `opencode`, `roo-code`, `goose`, `grok-cli`. In addition, `antigravity` is **planned** (in the catalog but not yet built into the sync engine, pending a research spike), and `gemini-cli` is **deprecated** (previously supported, removed from the active set on 2026-06-15; kept in code and catalog as reference). See [Providers → Planned](../concepts/providers.md#planned) and [Providers → Deprecated](../concepts/providers.md#deprecated). Per-provider pages are added as each lands.

Skills support is available for four active providers: `claude-code`, `opencode`, `codex`, and `grok-cli`. `gemini-cli` (`.gemini/skills/`) is excluded from skill operations because it is deprecated.

## A sync stopped on a conflict. Did I lose work?

No. The run is saved with its phase and beta branch. Resolve the conflicting path and run `graft sync agents --continue`. See [Resolve conflicts](../guides/resolve-conflicts.md).

## Can a teammate resolve a conflict I pushed?

No. Conflicts are **per-machine**. The resume state (sqlite run row, internal git refs, temporary worktrees) never leaves the machine where the conflict occurred — `git push` only publishes the committed `.graft/` files and provider files at HEAD.

A teammate who pulls your branch cannot run `--continue` on your behalf; their database has no record of your run. They simply run their own `graft sync` against the pulled files and resolve any divergence locally.

One important hazard: graft writes git conflict markers into the working canonical files (`agent.yaml`, `instructions.md`) when a sync halts — but does **not** stage or commit them. Running `git add . && git commit && git push` before resolving them publishes raw markers to teammates. graft now blocks this: `graft validate` and every `graft sync` detect unresolved markers in canonical files and produce a blocking error rather than a cryptic YAML parse failure. Resolve or discard the conflicting changes before staging. See [Conflicts across machines](../guides/resolve-conflicts.md#conflicts-across-machines-pushpull).

## Does graft sync skills or slash commands?

`graft skill` (status/install/sync) is **live**: it manages the canonical store at `.agents/skills/` and symlinks skills into the four supporting providers (`claude-code`, `opencode`, `codex`, `grok-cli`). What remains **planned**: skills as a scope of `graft sync` (i.e. `--scope skills`) and slash-command sync. graft is still **agents-first** for the sync engine.

## Related

- [Core concepts](../concepts/overview.md)
- [CLI reference](./cli.md)
