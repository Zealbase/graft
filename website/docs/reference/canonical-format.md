---
sidebar_position: 3
title: Canonical format
---

# Canonical format reference

The on-disk format of a canonical agent. This is the reference companion to the [Canonical store](../concepts/canonical-store.md) concept page.

:::note Schema authority
The exact `agent.yaml` schema is owned by `internal/canonical` and derived from the research team's `common-agent-definition.schema.json`. The fields below are the **frozen contract vocabulary** (`CanonicalAgent` in `internal/contract`). Use the generated schema for exact keys/types/defaults once published; anything beyond this set is tracked internally.
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
| `name` | string | Agent identifier. Not overridable via `providerOverrides`. |
| `description` | string | Short description. Must be non-empty before sync runs. |
| `model` | string | Default model id. |
| `tools` | string[] | Allowed tools. |
| `mcp` | string[] | MCP server references. |
| `permissions` | map&lt;string,string&gt; | Permission settings. |
| `providerOverrides` | map&lt;provider, map&gt; | Per-provider values with no canonical home. Restored verbatim when serializing back to that provider. |

The agent **body** (system prompt) lives in `instructions.md`, not in `agent.yaml`.

## `providerOverrides` rules

`providerOverrides` lets you set provider-specific fields that the canonical model does not have a home for. The key must be a recognized provider id.

- **`name` is excluded**: the serialization layer enforces this — a `providerOverrides[p]["name"]` entry produces a warning and is silently dropped, never written.
- **Unknown provider key → error**: an unrecognized key under `providerOverrides` blocks sync. graft uses Levenshtein distance to suggest the nearest valid provider id ("did you mean ...?").
- **Field validation → warning**: override values are checked against the provider's catalog schema. Unrecognized fields produce a warning (never blocking), since catalog schemas may be incomplete.

## Per-provider model override

Use `graft agent model` to set or clear a per-provider model override without editing `agent.yaml` by hand:

```bash
graft agent model reviewer --provider claude-code --model claude-opus-4
graft agent model reviewer --provider claude-code --clear
```

This writes `providerOverrides.claude-code.model` in `agent.yaml`.

## `.meta.json`

Holds per-provider source hashes and the last commit hash, used to compute [drift](../concepts/drift-and-status.md).

## Example

```yaml
# .graft/agents/reviewer/agent.yaml
name: reviewer
description: Reviews diffs for correctness and style.
model: claude-sonnet-4
tools:
  - read
  - grep
mcp: []
permissions:
  edit: deny
providerOverrides:
  claude-code:
    color: blue   # provider-specific, no canonical home; preserved on round-trip
  gemini-cli:
    model: gemini-2.0-flash   # per-provider model override
```

```markdown
<!-- .graft/agents/reviewer/instructions.md -->
You are a code reviewer. Focus on correctness first, then style.
```

## Related

- [Canonical store](../concepts/canonical-store.md)
- [Providers](../concepts/providers.md)
- [CLI reference — agent model](./cli.md#graft-agent-model-name)
