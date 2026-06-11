---
sidebar_position: 3
title: Canonical format
---

# Canonical format reference

The on-disk format of a canonical agent. This is the reference companion to the [Canonical store](../concepts/canonical-store.md) concept page.

:::info Schema authority
The exact `agent.yaml` schema is owned by `internal/canonical` and derived from the research team's `common-agent-definition.schema.json`. The fields below are the **frozen contract vocabulary** (`CanonicalAgent` in `internal/contract`). Use the generated schema for exact keys/types/defaults once published; anything beyond this set is **planned**.
:::

## Directory layout

```
.graft/agents/<name>/
├─ agent.yaml        # canonical fields (provider-neutral)
├─ instructions.md   # body / system prompt
└─ .meta.json        # per-provider source hash + last commit hash
```

## `agent.yaml` fields

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Agent identifier. |
| `description` | string | Short description. |
| `model` | string | Model id. |
| `tools` | string[] | Allowed tools. |
| `mcp` | string[] | MCP server references. |
| `permissions` | map&lt;string,string&gt; | Permission settings. |
| `providerOverrides` | map&lt;provider, map&gt; | Per-provider values with no canonical home. Restored verbatim when serializing back to that provider. |

The agent **body** (system prompt) lives in `instructions.md`, not in `agent.yaml`.

## `.meta.json`

Holds per-provider source hashes and the last commit hash, used to compute [drift](../concepts/drift-and-status.md).

## Example

```yaml
# .graft/agents/reviewer/agent.yaml
name: reviewer
description: Reviews diffs for correctness and style.
model: <model-id>
tools:
  - read
  - grep
mcp: []
permissions:
  edit: deny
providerOverrides:
  claude-code:
    # values with no canonical home, preserved for round-trips
    color: blue
```

```markdown
<!-- .graft/agents/reviewer/instructions.md -->
You are a code reviewer. Focus on correctness first, then style.
```

## Related

- [Canonical store](../concepts/canonical-store.md)
- [Providers](../concepts/providers.md)
