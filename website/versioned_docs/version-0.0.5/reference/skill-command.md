---
sidebar_position: 5
title: skill command
---

# skill command

`graft skill` manages the canonical skills store and the per-provider symlinks that make skills available to supporting tools (Claude Code, OpenCode, and Codex).

## Command summary

| Command | Does | Output |
|---------|------|--------|
| `graft skill list` | List canonical skills in `.agents/skills`. | Table. |
| `graft skill status` | Show per-provider link state for each canonical skill. | Table. |
| `graft skill install <name\|path>` | Copy a skill into `.agents/skills` (if absent) and symlink it into supporting providers. | Table. |
| `graft skill sync` | Re-apply: symlink all canonical skills into all supporting providers. | Table. |

All commands route through the EntryGate only.

---

## `graft skill list`

Lists the canonical skills present in `.agents/skills`. A directory is a skill only when it contains a `SKILL.md` marker file; directories without the marker are ignored.

```bash
graft skill list
```

Output columns: `name`, `dir` (absolute path to the canonical skill directory).

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft skill status`

Reports the live filesystem link state of every canonical skill for every supporting provider. State is always computed live (lstat/readlink) — not from a database.

```bash
graft skill status
graft skill status --provider claude-code
```

Output columns: `skill`, `provider`, `state`, `link_path`.

Link states:

| State | Meaning |
|-------|---------|
| `linked` | Symlink points at the canonical skill dir. All good. |
| `missing` | No entry at the provider path. Run `graft skill sync` to fix. |
| `wrong-link` | A symlink exists but points elsewhere. Run `graft skill sync` to fix. |
| `conflict` | A real (non-symlink) dir or file is present. Use `--override` to replace it. |

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--override` | bool | `false` | Replace a non-symlink entry with a symlink. |
| `-p`, `--provider` | string | — | Limit output to a single supporting provider. |
| `--yes` / `--install` | bool | `false` | Non-interactive: auto-install missing referenced skills. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft skill install <name|path>`

Copies a skill into the canonical store at `.agents/skills/<name>/` (if it is not already present), then symlinks it into every supporting provider's skills directory. The argument can be:

- A **filesystem path** to a skill directory that contains `SKILL.md` (absolute or relative, starting with `/`, `.`, or `~`, or containing a path separator).
- A **bare skill name** already present in the canonical store — no copy-in is performed; Apply runs directly.
- A **bare skill name** found in a supporting provider's project skills directory — copied from there into the canonical store, then symlinked everywhere.
- A **bare skill name** found in any home-scope directory (e.g. `~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, `~/.config/opencode/skills`) — copied into the canonical store.

Install is idempotent: if the named skill already exists canonically, it is not overwritten.

```bash
graft skill install my-skill
graft skill install ./tools/my-skill
graft skill install /path/to/my-skill
graft skill install my-skill --provider claude-code
graft skill install my-skill --override
```

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--override` | bool | `false` | Replace a non-symlink entry at the provider path with a symlink. |
| `-p`, `--provider` | string | — | Limit symlinking to a single supporting provider. |
| `--yes` / `--install` | bool | `false` | Non-interactive: auto-install without prompting. `--install` is an alias for `--yes`. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## `graft skill sync`

Re-applies all canonical skills: iterates every skill in `.agents/skills` and every supporting provider, creating or correcting the symlink at each provider's skills directory. This is the idempotent reconciliation pass.

```bash
graft skill sync
graft skill sync --provider claude-code
graft skill sync --override
```

Use `graft skill sync` after manually adding a skill directory to `.agents/skills`, or to repair `missing` / `wrong-link` states without running a full agent sync.

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--override` | bool | `false` | Replace a non-symlink entry with a symlink. |
| `-p`, `--provider` | string | — | Limit to a single supporting provider. |
| `--yes` / `--install` | bool | `false` | Non-interactive: auto-install missing referenced skills. |
| `-o`, `--output` | string | `table` | Output format: `json`, `yaml`, or `table`. |

---

## Config keys

The `skills.*` keys control the automatic skill apply hook that runs after `graft init` and `graft sync agents`.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `skills.enabled` | bool | `true` | Master switch for the init/sync hook. When `false`, the hook is disabled and skills are never applied automatically. |
| `skills.autoInstall` | bool | `false` | When `true`, the hook installs missing referenced skills without prompting (equivalent to `--yes`). |
| `skills.providers[]` | string[] | — | Restrict the hook to these supporting provider ids. Empty = all supporting providers. |

Read or write config with `graft config get` / `graft config set`. See [Config reference](./config.md).

---

## Init/sync hook

`graft init` and `graft sync agents` automatically run a skill apply pass after success when `skills.enabled` is `true`. The hook:

- Runs `Apply` across all supporting providers (or those listed in `skills.providers[]`).
- Respects `skills.autoInstall` as the `--yes` equivalent.
- Swallows all errors and logs them to stderr — a skill problem never fails an agent operation.
- Returns the resulting link states as part of the command summary.

To opt out entirely, set `skills.enabled = false`.

---

## Legacy migration

The first time any `graft skill` command runs, graft checks for a legacy `.agent/skills` directory (singular) and migrates its contents into `.agents/skills` (plural). The migration is idempotent; skills already present canonically are not overwritten. The legacy directory is removed once it is empty.

---

## Related

- [Skills concept](../concepts/skills.md)
- [Config reference](./config.md)
- [CLI reference](./cli.md)
