---
sidebar_position: 2
title: Resolve conflicts
description: How to identify and resolve merge conflicts when graft can't automatically reconcile two providers' changes to the same agent.
---

# Resolve conflicts

When two providers changed the same agent in incompatible ways, graft's merge can't reconcile them automatically. It stops, tells you the path, and lets you resume.

## What happens on a conflict

During the merge loop, if a merge fails graft:

1. Records the conflict (path + agent) in its database.
2. Surfaces the conflicting path to you.
3. Stops the run, leaving its row at `status = conflict`.

The run is **not** lost. Its `phase` and beta branch are saved so it can pick up exactly where it stopped.

## How to use

1. graft prints something like: `merge conflict — resolve the markers in the listed file(s), then re-run graft sync`.
2. Open the path and resolve the conflict (standard merge markers).
3. Resume the run:

```bash
graft sync agents --continue
```

`graft agent sync --continue` works as well (it is an alias).

graft detects the open conflict run for this workspace, resumes from the recorded phase, and continues the merge loop rather than restarting from scratch.

## Concurrency note

While a run is paused on a conflict, a second `graft sync` on the same workspace will detect the open run and resume it rather than starting a competing run. graft holds an exclusive lock per workspace `(root, remote, branch)`.

## How it works

Conflicts are part of the merge loop in the sync engine. The beta branch holds the partial merge; resolution lets the loop advance it to the next clean state. See [How sync works](../concepts/how-sync-works.md).

## Conflicts across machines (push/pull)

A graft conflict is **local to the machine where it occurred**. The resume state never leaves that machine via `git push`.

**What stays local:**

- The global sqlite database row (`sync_run.status = conflict`) that `--continue` queries.
- Internal git refs under `refs/heads/graft/<run_id>/…` and worktrees under `.git/graft-worktrees/` (pruned when the run finishes cleanly).

**What `git push` publishes:**

Only the committed files on your working branch — the `.graft/` canonical files and provider files at HEAD. The resume state is not among them.

**What this means for teammates:**

A teammate who pulls your branch cannot run `graft sync agents --continue` on your behalf. Their database has no record of your conflict run. `--continue` will find nothing and start fresh. The correct workflow for a teammate is to pull the committed files and run their own `graft sync` against them. If their local state diverges, graft resolves it locally on their machine.

**Do not commit conflict markers.**

When a sync halts on a conflict, graft writes standard git conflict markers (`<<<<<<<` / `=======` / `>>>>>>>`) into the working `.graft/agents/<name>/agent.yaml` and `instructions.md` so you can see and resolve them. These files are **not** staged or committed by graft. If you run `git add . && git commit && git push` before resolving them, your teammates receive raw conflict markers in the canonical files.

graft guards against this: `graft validate` and every `graft sync` scan canonical files for unresolved markers before proceeding. If any are found, graft blocks with an error:

```
unresolved git conflict markers in .graft/agents/<name>/agent.yaml — resolve the markers before syncing
```

Resolve or discard the conflicting changes in the file before staging it.

## Related

- [Sync an agent](./sync-an-agent.md)
- [How sync works](../concepts/how-sync-works.md)
