---
sidebar_position: 4
title: Validate before sync
description: Run graft validate to check schema and semantic correctness of your canonical agent definitions before syncing.
---

# Validate before sync

graft validates schema and semantics before every sync. You can also run validation on its own.

## What it does

Validation runs schema checks (against each provider's JSON Schema and the canonical schema) plus semantic checks. It returns **findings**, each with an agent, optional provider/path, a message, and a severity (`error` or `warning`). Errors block a sync.

Checks include:

- Canonical field types and required fields (non-empty description, valid model).
- `providerOverrides` key validity — an unknown provider id is an error with a "did you mean" suggestion.
- Override field conformance against the provider's catalog schema (warning-only; catalog schemas may be incomplete).
- The `name` field is structurally excluded from overrides; attempting to override it produces a separate warning.

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

The EntryGate calls the validator with the changed scope before handing off to the sync engine. Clean → proceed; findings → block. The catalog provides the offline per-provider schema used for override field validation.

## Related

- [Sync an agent](./sync-an-agent.md)
- [Providers](../concepts/providers.md)
- [Catalog](../reference/cli.md#graft-catalog-verify)
