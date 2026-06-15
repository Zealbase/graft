---
sidebar_position: 4
title: How sync works
description: How graft's sync engine works — branch isolation, canonical merging, and conflict-safe propagation to every provider.
---

# How sync works

This page explains graft's sync engine conceptually: how a change becomes a merge and lands back in every provider. You do not need this to use graft, but it explains the guarantees — no commits to your base branch, resumable conflicts, and safe concurrency.

## The shape of a sync

A sync is one tracked **run** (`run_id`) that moves through these stages:

1. **Detect & diff** — the base is your current branch. graft diffs the working tree against it to find changed agent files.
2. **Branch per changed file** — each changed file is moved onto its own temporary branch (`graft/<run_id>/agent/<name>`).
3. **Canonicalize** — each changed provider file is transformed into the canonical form and written under `.graft/agents/<name>/`.
4. **Merge loop** — the per-file branches are merged sequentially into a moving result branch (`graft/<run_id>/beta/<n>`).
5. **Conflict → manual → resume** — if a merge fails, graft records the conflict, surfaces the path, and stops. You resolve it and rerun; the run picks up from where it left off.
6. **Reapply onto a moving base** — when all branches are merged, graft checks whether the base moved during the run. If it did, the merge is redone onto a fresh beta (`beta_y`). If stable, it proceeds.
7. **Copy to base, no commit** — the stabilized beta tree is copied into the working directory as the result. **The base branch gets no commit.** The beta branch acts only as a tracked reference.
8. **Write providers & prune** — the canonical result is serialized out to every enabled provider, and the temporary branches are pruned.

---

## Canonical-as-source

Editing `.graft/agents/<name>/agent.yaml` is the primary workflow. Sync fans the canonical out to all enabled providers. You can also edit a provider file directly; graft will pull the change back to canonical on the next sync and reapply to all providers.

## Ingestion

When `--ingest=true` (the default), agents that exist only in a provider (no canonical entry yet) are pulled into `.graft/agents/` and fanned out to every other provider. Pass `--ingest=false` to suppress this behavior and only process agents that already have a canonical entry.

## Deletion semantics

A deleted canonical agent is removed from all providers on the next sync. Deleting `.graft/agents/<name>/` is enough — graft will not resurrect the agent from a stale provider copy. `--dry-run` shows deletion candidates before they are applied.

## Tool canonicalization through sync

graft stores tool names in canonical form (`lowercase_snake_case`) in `.graft/agents/<name>/agent.yaml`. On apply, the serialization layer translates each canonical name to the provider's native spelling:

| Canonical | Claude Code | Gemini CLI (deprecated) | OpenCode | Codex |
|-----------|-------------|------------|----------|-------|
| `web_search` | `WebSearch` | `google_web_search` | `websearch` | `web_search` |
| `read_file` | `Read` | `read_file` | `read` | — |
| `bash` | `Bash` | `run_shell_command` | `bash` | `shell` |
| `grep` | `Grep` | `search_file_content` | `grep` | — |
| `web_fetch` | `WebFetch` | `web_fetch` | `webfetch` | — |
| `file_edit` | `Edit` | `edit` / `replace` | `edit` | — |

This means you write tool names once, in canonical form, and every provider gets its native spelling. The full mapping is in `internal/catalog/data/canonical-tools.md`.

When canonicalizing an existing provider file (ingest or provider-side edit), native tool names are reverse-mapped back to canonical form before being written to `agent.yaml`. Aliases are case-insensitive (e.g. `WebSearch`, `websearch`, and `web_search` all resolve to canonical `web_search`).

Per-provider tool overrides (`providerOverrides[<provider>].tools`) bypass translation — they are written and read verbatim in the provider's native naming.

## Skill state in sync

Skill symlink state is included in the in-sync check. Dead or broken skill symlinks are pruned during every agent sync pass. The sync summary includes a count of canonical skills when skills are enabled.

## "Already in sync"

When no files have changed and all providers match the canonical, graft exits cleanly with a summary. Exit code is 0.

---

## Why a beta branch instead of a commit

The merge loop runs on a fresh branch cut from the base (`graft/<run_id>/beta/<n>`). That beta *is* the moving "new base": each clean merge advances it. When it stabilizes, its tree is copied back into the working directory. Your base branch is never committed to — graft leaves your history clean and lets you commit on your own terms.

## Resumable runs

A run's `phase` and `beta_branch` are recorded in sqlite. If a run halts mid-flight — for example, while you edit a conflicted file — its row stays `status = conflict`. The next `graft sync` (or `graft sync --continue`) detects it and resumes from the recorded phase instead of restarting. See [Resolve conflicts](../guides/resolve-conflicts.md).

## Concurrency

graft takes an **exclusive lock per workspace** `(root, remote, branch)`. A second `graft sync` on the same workspace waits for the first to finish its full read → merge → apply cycle. The global sqlite database uses WAL plus locks so multiple CLI invocations stay safe.

## Git mode

- **Existing git repo → `tracked`**: graft uses the repo's own git pointer and current branch as the base; temp/beta branches and worktrees are created in that same repo. No separate repo is made.
- **No git → `internal`**: graft falls back to an internal repo. The moment a real git repo is detected on a later sync, graft migrates to `tracked`.

There are **no git hooks** — migration and sync run only when you invoke `graft sync`.

---

## Related

- [Drift and status](./drift-and-status.md)
- [Resolve conflicts](../guides/resolve-conflicts.md)
- [Sync an agent](../guides/sync-an-agent.md)
- [Skills](./skills.md)
