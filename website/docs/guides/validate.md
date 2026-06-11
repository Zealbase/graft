---
sidebar_position: 4
title: Validate before sync
---

# Validate before sync

graft validates schema and semantics before every sync. You can also run validation on its own.

:::info Planned
Commands reflect the planned CLI surface (plan 03). Auto-validation before sync is part of the frozen gate behavior.
:::

## What it does

Validation runs schema checks (against each provider's JSON Schema and the canonical schema) plus semantic checks. It returns **findings**, each with an agent, optional provider/path, a message, and a severity (`error` or `warning`). Errors block a sync.

## How to use

Validate everything:

```bash
graft validate --all
```

Validate against one provider:

```bash
graft validate --provider <provider-id>
```

Valid provider ids are listed in [Providers](../concepts/providers.md) (e.g. `claude-code`, `codex`, `cursor`).

## Auto-validation on sync

You rarely need to call `validate` directly: every `graft sync` runs validation first. If there are blocking findings, the sync is stopped and the failures are reported per agent before any provider is written. `graft validate` just exposes that same gate standalone, so you can check before committing to a sync.

## Exit behavior

`graft validate` exits non-zero when there are error-severity findings, so it works in scripts and CI.

## How it works

The EntryGate calls the validator with the changed scope before handing off to the sync engine. Clean → proceed; findings → block. See the validate-before-sync gate in [the CLI reference](../reference/cli.md).

## Related

- [Sync an agent](./sync-an-agent.md)
- [Providers](../concepts/providers.md)
