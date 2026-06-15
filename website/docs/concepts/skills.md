---
sidebar_position: 6
title: Skills
description: Skills are self-contained capability directories stored canonically in .agents/skills/ and symlinked into each supporting provider.
---

# Skills

A **skill** is a self-contained capability directory that a supporting AI coding tool reads at startup. Skills are stored once in a canonical location and **symlinked** — not copied or merged — into each supporting provider's skills folder.

## What a skill is

A skill is a directory containing a `SKILL.md` marker file plus any supporting assets. The marker file is what makes a directory a skill; directories without `SKILL.md` are ignored.

```
.agents/skills/<name>/
├─ SKILL.md        # required marker; body of the skill
└─ <assets>/       # optional supporting files
```

## Canonical store

The canonical store lives at `.agents/skills/<name>/` inside your workspace root. The plural `.agents` is the [agentskills.io](https://agentskills.io) vendor-neutral convention; codex and opencode read this location natively.

```
<workspace-root>/
└─ .agents/
   └─ skills/
      ├─ my-skill/
      │  └─ SKILL.md
      └─ another-skill/
         └─ SKILL.md
```

There is no database entry and no git involvement. Every link state is computed live from the filesystem (lstat/readlink) each time you run a skill command.

## Reconciliation by symlink

Unlike agents — which are transformed, merged, and tracked in sqlite — skills are reconciled purely by the filesystem. graft creates one symlink per (provider, skill) pair pointing at the canonical directory. It never copies or merges skill content between providers.

```
.claude/skills/my-skill  →  ../../.agents/skills/my-skill
.opencode/skills/my-skill → ../../.agents/skills/my-skill
```

Link state for each (provider, skill) pair is one of:

| State | Meaning |
|-------|---------|
| `linked` | Symlink exists and points at the canonical skill dir. |
| `missing` | No entry at the provider path. |
| `wrong-link` | A symlink exists but points somewhere else. |
| `conflict` | A real (non-symlink) directory or file is present; use `--override` to replace it. |

## Supporting providers

Only three graft providers support skills; the others are silently skipped. Two of them are symlink-based (graft creates a symlink into their project skills dir); `codex` reads the canonical `.agents/skills/` directory natively, so no symlink is created for it.

| Provider id | Tool | Project skills dir |
|-------------|------|--------------------|
| `claude-code` | Claude Code | `.claude/skills/` |
| `opencode` | OpenCode | `.opencode/skills/` |
| `codex` | Codex | native (`.agents/skills/`, no symlink) |

:::note Claude Code and the vendor-neutral store
Claude Code does not read `.agents/skills` directly, so it always gets a symlink under `.claude/skills/`. The symlink is what makes skills available to Claude Code in a project.
:::

:::note gemini-cli dewired
`gemini-cli` previously supported skills (`.gemini/skills/`) but was **dewired** from the sync engine per maintainer request on 2026-06-15. Its code is kept as reference; until it is re-registered, `graft skill` does not manage `.gemini/skills/`.
:::

## Home-scope detection

graft also scans each supporting provider's personal (home) skill directories when looking for install candidates. Home-scope directories are **read-only sources** — graft surfaces their contents as skills you can install into the canonical store, but it never symlinks anything into them.

| Provider | Personal skill directories scanned |
|----------|-----------------------------------|
| `claude-code` | `~/.claude/skills` |
| `codex` | `~/.codex/skills`, `~/.agents/skills` |
| `opencode` | `~/.config/opencode/skills`, `~/.claude/skills`, `~/.agents/skills` |

A skill found in any of these locations appears as an install candidate in `graft skill status` and can be installed by bare name: `graft skill install <name>`.

## Legacy migration

The legacy location `.agent/skills` (singular) from earlier graft versions is automatically migrated to `.agents/skills` (plural) the first time any skill command runs. The migration is idempotent: skills already present canonically are not overwritten, and the legacy directory is removed once it has been drained.

## Init and sync hook

After a successful `graft init` or `graft sync agents`, graft automatically runs a skill apply pass (the init/sync hook). This hook symlinks every canonical skill into all supporting providers without requiring a separate `graft skill sync`. Hook behavior is controlled by config keys under `skills.*`:

- `skills.enabled` — master switch; `true` by default.
- `skills.autoInstall` — when `true`, install missing referenced skills without prompting (equivalent to `--yes`).
- `skills.providers[]` — restrict the hook to specific providers (empty = all supporting providers).

A skill problem in the hook never fails the agent operation. Errors are logged to stderr and the hook result is swallowed.

## Related

- [skill command reference](../reference/skill-command.md)
- [Providers](./providers.md)
- [Canonical store](./canonical-store.md)
