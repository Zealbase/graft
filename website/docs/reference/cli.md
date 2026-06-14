---
sidebar_position: 1
title: CLI reference
description: Full reference for every graft CLI command — init, sync, status, validate, destroy, skill, and agent — with flags and output formats.
---

# CLI reference

Every graft command, its purpose, flags, and output format. All commands route **CLI → EntryGate** only.

## Command summary

| Command | Does | Output |
|---------|------|--------|
| `graft init` | Create `.graft/`, register the workspace row, detect git mode. | Created path. |
| `graft agent list` | List canonical agents and their provider coverage. | Table. |
| `graft agent <name> status` | Drift of one agent across providers. | Per-provider in/out-of-sync. |
| `graft agent init <name> [prompt]` | Scaffold a new canonical agent. | Agent record + next-step hint. |
| `graft agent model <name>` | Set or clear a per-provider model override. | Validation findings (warn-only). |
| `graft agent sync [<name>]` | Alias for `graft sync agents` / `graft sync agent <name>`. | Sync result. |
| `graft agents status` | Aggregated drift: providers out of sync + agent counts. | Summary table. |
| `graft sync agent <name>` | Run sync for one agent. | `run_id` + result. |
| `graft sync agents` | Run sync for all changed agents. | `run_id` + result. |
| `graft validate` | Schema + semantic validation (also runs before every sync). | Findings; non-zero exit on error. |
| `graft config get` | Print resolved config. | YAML (default). |
| `graft config set` | Write config keys. | YAML. |
| `graft skill list` | List canonical skills. | Table. |
| `graft skill status` | Per-provider link state for each skill. | Table. |
| `graft skill install <name\|path>` | Copy a skill into `.agents/skills` and symlink into providers. | Table. |
| `graft skill sync` | Re-apply: symlink all canonical skills into all supporting providers. | Table. |
| `graft catalog verify` | Verify embedded catalog hashes match the manifest. | OK / FAIL. |
| `graft destroy` | Remove graft state for this workspace (provider files kept). | Destroy result. |
| `graft update` | Check for / install a newer graft release. | Update result. |

---

## `graft init`

Creates the `.graft/` canonical store, registers a workspace row in graft's sqlite database, and detects the git mode. On first run a provider-detection pass runs; `--yes`/`--ci` skip prompts.

```bash
graft init
graft init --ci
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Non-interactive: skip the first-run checklist. |
| `--ci` | bool | `false` | Non-interactive (alias for `--yes`). |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft agent list`

Lists canonical agents and their coverage across providers.

```bash
graft agent list
graft agent list -o json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft agent <name> status`

Shows whether each provider is in sync with the canonical for one agent.

```bash
graft agent reviewer status
graft agent reviewer status -o json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft agent init <name> [prompt]`

Scaffolds a new canonical agent under `.graft/agents/<name>/` with a default `agent.yaml` and empty `instructions.md`. Accepts an optional `prompt` positional argument that sets the description field. The agent must have a non-empty description before a sync will accept it.

```bash
graft agent init my-bot
graft agent init my-bot "Reviews pull requests for style and correctness."
```

After scaffolding, fan the agent out to providers:

```bash
graft sync agent my-bot
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft agent model <name>`

Sets or clears a per-provider model override on an agent. The override is stored under `providerOverrides[<provider>].model` in `agent.yaml`. Warn-only: unknown model ids produce a warning but never block.

```bash
graft agent model reviewer --provider claude-code --model claude-opus-4
graft agent model reviewer --provider claude-code --clear
```

`--clear` and `--model` are mutually exclusive. `--provider` is required.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | string | — | Target provider id (required). |
| `--model` | string | — | Model id to set. |
| `--clear` | bool | `false` | Remove the provider's model override. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft agent sync [<name>]`

Alias for `graft sync agents` (no argument) or `graft sync agent <name>` (with argument). Behavior and output are identical to the `sync` subcommands. Both surfaces are kept permanently.

```bash
graft agent sync
graft agent sync reviewer
graft agent sync --dry-run
```

Flags: same as `graft sync agents` / `graft sync agent <name>` (see below).

---

## `graft agents status`

Aggregated drift across the whole workspace.

```bash
graft agents status
graft agents status -o json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft sync agent <name>` / `graft sync agents`

Run a sync for one agent or for everything that changed. Each validates first (blocking on error findings), runs the merge engine, writes enabled providers, and prints a result.

```bash
graft sync agent reviewer
graft sync agents
graft sync agents --dry-run
graft sync agents --ingest=false
graft sync agents --continue
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | `false` | Report what would change (including agents pending deletion) without writing any files or db rows. |
| `--ingest` | bool | `true` | Canonicalize provider-only agents and fan them out. Pass `--ingest=false` to suppress. |
| `--continue` | bool | `false` | Resume an interrupted conflict run for this workspace. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

### Sync semantics

- **Canonical-as-source**: editing `.graft/agents/<name>/agent.yaml` is the primary workflow; sync fans the canonical out to all providers.
- **Ingestion**: when `--ingest=true` (default), agents found only in a provider are pulled into the canonical store and fanned out — provider-only agents are not silently skipped.
- **Deletion-aware**: a deleted canonical agent is removed from all providers on the next sync. An agent removed from `.graft/agents/` is not resurrected by a provider copy of the old file.
- **Skill state**: skill symlink state is part of the in-sync check. Dead or broken skill symlinks are pruned on every sync.
- **"Already in sync"**: when nothing changed the command exits cleanly with a summary and exit code 0.

---

## `graft validate`

Runs schema + semantic validation. Exits non-zero on error-severity findings. Also runs implicitly before every sync.

```bash
graft validate --all
graft validate --provider claude-code
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | — | Validate all enabled providers. |
| `--provider <id>` | string | — | Validate against one provider. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft config get`

Print the resolved config. Default: project-over-global layered view. `-g` prints global only.

```bash
graft config get
graft config get -g
graft config get -o json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-g`, `--global` | bool | `false` | Print the global config only (no project layer). |
| `-o`, `--output` | string | `yaml` | Output format: `json`, `yaml`, or `table`. |

---

## `graft config set`

Write config keys. Default: project-overridable keys write to `.graft/config.json`; global-only keys write to the global XDG config. `-g` forces all keys to the global config.

```bash
graft config set --scope agents
graft config set --providers.mode specific --providers.enabled claude-code,gemini-cli
graft config set -g --theme dark
graft config set -g --skills.enabled false
graft config set -g --sync.gitAuto true
```

Keys that are empty are left unchanged.

| Flag | Type | Scope | Description |
|------|------|-------|-------------|
| `--scope` | string | project | Synced capability: `agents`, `skills`, or `slash`. |
| `--providers.mode` | string | project | Provider selection mode: `all` or `specific`. |
| `--providers.enabled` | string | project | Comma-separated active providers (`mode=specific`). |
| `--providers.disabled` | string | project | Comma-separated excluded providers (`mode=all`). |
| `--sync.gitAuto` | bool string | global | Auto-commit tracking branches (`true`/`false`). |
| `--theme` | string | global | Color theme: `dark`, `dark-dim`, `light`, or `colorblind`. |
| `--skills.enabled` | bool string | global | Master switch for the init/sync skill hook (`true`/`false`). |
| `--skills.autoInstall` | bool string | global | Auto-install missing referenced skills without prompting. |
| `--skills.providers` | string | global | Comma-separated subset of supporting providers for skills. |
| `-g`, `--global` | bool | — | Write all keys to the global config. |
| `-o`, `--output` | string | — | Output format: `json`, `yaml`, or `table`. |

See [Config reference](./config.md) for the full key reference.

---

## `graft catalog verify`

Recomputes the embedded catalog hashes and compares them against the manifest. Exits non-zero on mismatch. No workspace or network required.

```bash
graft catalog verify
graft catalog verify -o json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft destroy`

Removes graft-managed state for this workspace. Provider agent files (`.claude/`, `.codex/`, etc.) are **never touched**. A confirmation prompt is shown unless `--yes` or `--ci` is passed.

```bash
graft destroy
graft destroy --yes
graft destroy --ci
graft destroy --keep-store
```

What is removed:

- `.graft/config.json`, `.graft/graft.db` (if still present), the workspace lock.
- The workspace row (and all associated run/branch/agent rows) from the global sqlite database.
- `.graft/` itself (unless `--keep-store`).

What is kept:

- `.graft/agents/` and per-agent canonical files (when `--keep-store`).
- All provider files (always).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip the confirmation prompt. |
| `--ci` | bool | `false` | Non-interactive (alias for `--yes`). |
| `--keep-store` | bool | `false` | Retain `.graft/agents/` (canonical store); only drop config, db rows, and lock. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft update`

Checks the GitHub releases API for a newer graft binary and installs it if one is found. Works outside an initialized workspace.

```bash
graft update
graft update --check
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | bool | `false` | Report current vs latest version without installing. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

See [Endpoints](./endpoints.md) for the outbound URLs this command contacts.

---

## `graft skill` commands

See the dedicated [skill command reference](./skill-command.md).

---

## Output formats

All commands accept `-o json`, `-o yaml`, or `-o table`. Machine-readable outputs (`json`/`yaml`) strip table annotations and surface raw contract types.

## Related

- [Config reference](./config.md)
- [Canonical format](./canonical-format.md)
- [How sync works](../concepts/how-sync-works.md)
- [Endpoints](./endpoints.md)
