---
sidebar_position: 2
title: Canonical store
description: The on-disk format and layout of the .graft/ canonical agent store — the single source of truth for all providers.
---

# Canonical store

The canonical store is graft's source of truth. It lives at `.graft/` in your workspace and holds one directory per agent.

## What it is

Each agent is a small directory under `.graft/agents/<name>/`:

```
.graft/agents/<name>/
├─ agent.yaml        # canonical fields (provider-neutral)
├─ instructions.md   # body / system prompt
└─ .meta.json        # per-provider source hash + last commit hash
```

- `agent.yaml` holds the provider-neutral fields.
- `instructions.md` is the agent body (system prompt). In the canonical struct this is the `Body`.
- `.meta.json` tracks per-provider source hashes used for drift detection.

## Canonical fields

The wire-level field vocabulary is frozen in `internal/contract` (`CanonicalAgent`). These are the fields that cross package boundaries:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Agent identifier. Not overridable via `providerOverrides`. |
| `description` | string | Short description. Must be non-empty before a sync runs. |
| `model` | string | Default model id. |
| `tools` | string[] | Allowed tools. |
| `mcp` | string[] | MCP server references. |
| `permissions` | map&lt;string,string&gt; | Permission settings. |
| `providerOverrides` | map&lt;provider, map&gt; | Per-provider values with no canonical home. |
| `body` | string | The `instructions.md` content (not stored in `agent.yaml`). |

:::note Schema authority
The concrete on-disk shape of `agent.yaml` is owned by the canonical package (`internal/canonical`) and derived from the research team's `common-agent-definition.schema.json`. The table above lists the **frozen contract fields**. Treat the generated schema as authoritative for exact key names, types, and defaults once published.
:::

## Scaffolding a new agent

Use `graft agent init` to scaffold an agent with the right directory structure:

```bash
graft agent init my-bot "Reviews pull requests for style."
```

This creates `.graft/agents/my-bot/` and prints a next-step hint. The agent must have a non-empty description before `graft sync` will accept it.

## Lossless round-trips with providerOverrides

Providers carry settings that have no neutral home in the canonical model. To keep sync lossless, a provider's parser stashes those values under `providerOverrides[<provider>]`, and the same provider's serializer restores them when writing back. This is what lets a change at one provider survive a trip through canonical and back out to the others.

Unknown provider keys in `providerOverrides` produce an error finding (blocks sync) with a "did you mean" suggestion. Override values are validated against the provider's catalog schema (warning-only). `name` is structurally excluded from overrides — it is the agent's identity.

## Workspace identity

A workspace is **not** just a directory. It is the tuple `(root, remote, branch)`:

- One git repo can hold many `.graft/` sub-trees (different sub-dirs) — each is its own workspace row.
- The **same branch checked out in another worktree** maps to the **same** workspace, so changes there propagate. graft keys on `(remote, branch)`, not the working path.

## Related

- [Providers](./providers.md)
- [Drift and status](./drift-and-status.md)
- [Canonical format reference](../reference/canonical-format.md)
