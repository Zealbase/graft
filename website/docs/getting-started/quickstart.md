---
sidebar_position: 1
title: Quickstart
description: Install graft and sync your first agent definition across all AI providers in under 5 minutes.
---

# Quickstart

The shortest path from nothing to your first sync. This walks through initializing graft in an existing repository, creating or inspecting agents, and syncing them to every provider.

<Steps>

<Step title="Install">

See [Install](./install.md) for all methods. Build from source:

```bash
go install github.com/Shaik-Sirajuddin/graft/cmd/graft@latest
```

Verify:

```bash
graft --version
```

</Step>

<Step title="Initialize">

Run inside an existing git repository:

```bash
graft init
```

This creates the `.graft/` canonical store, registers a workspace row in graft's sqlite database, detects your git mode (`tracked` if a real repo is present, otherwise `internal`), and runs a first-time provider detection. Use `--ci` to skip the interactive prompts:

```bash
graft init --ci
```

</Step>

<Step title="Create an agent (or inspect detected ones)">

Create a new agent from scratch:

```bash
graft agent init my-bot "Reviews pull requests for correctness and style."
```

Or list agents graft detected from your existing provider files:

```bash
graft agent list
```

</Step>

<Step title="Edit the canonical agent">

Open the canonical definition and change something:

```bash
$EDITOR .graft/agents/my-bot/agent.yaml
$EDITOR .graft/agents/my-bot/instructions.md
```

See [Canonical store format](../concepts/canonical-store.md) for the fields.

</Step>

<Step title="Sync">

```bash
graft sync agents
```

graft validates the changed agents, runs the merge engine, writes the result to every enabled provider, and prints the outcome. To sync just one:

```bash
graft sync agent my-bot
```

Preview what would change without writing:

```bash
graft sync agents --dry-run
```

</Step>

<Step title="Confirm">

```bash
graft agents status
```

All providers should report in sync.

</Step>

<Step title="(Optional) Manage skills">

If you have skills in `.agents/skills/`, graft symlinks them automatically after every `init` and sync. To check the current link state:

```bash
graft skill status
```

To install a skill from a path:

```bash
graft skill install ./tools/my-skill
```

See the [skill command reference](../reference/skill-command.md).

</Step>

</Steps>

---

## What just happened

graft diffed the working tree against your base branch, isolated the changed agent files onto temporary branches, merged them into the canonical store, then reapplied the canonical result out to every provider — without committing to your base branch. See [How sync works](../concepts/how-sync-works.md).

---

## For AI agents

graft's documentation is available as machine-readable indexes:

- [`/llms.txt`](/llms.txt) — concise index with links and brief descriptions
- [`/llms-full.txt`](/llms-full.txt) — expanded variant with fuller content

These follow the [llmstxt.org](https://llmstxt.org) convention and help AI agents navigate graft's docs.

---

## Next

- [Core concepts](../concepts/overview.md)
- [Resolve conflicts](../guides/resolve-conflicts.md)
- [CLI reference](../reference/cli.md)
- [Config reference](../reference/config.md)
