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

Ten: `claude-code`, `codex`, `gemini-cli`, `cursor`, `github-copilot`, `opencode`, `roo-code`, `goose`, `grok-cli`, `antigravity`. See [Providers](../concepts/providers.md). Per-provider pages are added as each lands.

## Does graft sync skills or slash commands?

Not yet — graft is **agents-first**. Skills and slash-command sync are **planned**.

## A sync stopped on a conflict. Did I lose work?

No. The run is saved with its phase and beta branch. Resolve the conflicting path and run `graft sync agents --continue`. See [Resolve conflicts](../guides/resolve-conflicts.md).

## Related

- [Core concepts](../concepts/overview.md)
- [CLI reference](./cli.md)
