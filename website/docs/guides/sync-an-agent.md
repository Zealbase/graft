---
sidebar_position: 1
title: Sync an agent
---

# Sync an agent

Reconcile an agent across providers after you change it — anywhere.

## What it does

A sync detects changed agent files, transforms them to canonical, merges, and writes the result back out to every enabled provider. It validates first and never commits to your base branch.

## How to use

Sync one agent:

```bash
graft sync agent <name>
```

Sync everything that changed:

```bash
graft sync agents
```

Both are also available as `graft agent sync [<name>]` — same behavior, kept as a convenient alias.

Each command prints a result with agent names, provider outcomes, and (when skills are enabled) a count of canonical skills.

### Preview without writing

```bash
graft sync agents --dry-run
```

Dry-run reports what would change — including agents pending deletion — without mutating any files or database rows.

### Ingest provider-only agents

By default (`--ingest=true`), agents found only in a provider's directory are pulled into the canonical store and fanned out to all providers. To skip this:

```bash
graft sync agents --ingest=false
```

### Resume an interrupted run

If a previous sync stopped on a conflict, continue it:

```bash
graft sync agents --continue
```

See [Resolve conflicts](./resolve-conflicts.md).

## Deletion behavior

Deleting `.graft/agents/<name>/` and running sync removes the agent from all providers. graft does not resurrect an agent from a stale provider copy.

## Which providers are written

The effective provider set comes from your config (`providers.mode`, `providers.enabled[]`, `providers.disabled[]`), with the project config taking priority over global. See [Config reference](../reference/config.md).

## How it works

Under the hood each changed file goes onto its own temporary branch, is canonicalized, merged into a moving beta branch, and — once stable — copied into your working tree and serialized to every provider. Skill symlink state is pruned and rechecked as part of every sync pass. Full walkthrough: [How sync works](../concepts/how-sync-works.md).

## Related

- [Check status](./check-status.md)
- [Validate](./validate.md)
- [CLI reference](../reference/cli.md)
