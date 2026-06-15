# <img src="assets/logo.svg" alt="" height="28" valign="middle" /> graft

**Collaboratively develop agents with your team.**

[![Go Report Card](https://goreportcard.com/badge/github.com/Shaik-Sirajuddin/graft)](https://goreportcard.com/report/github.com/Shaik-Sirajuddin/graft)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)](go.mod)
[![CI](https://github.com/Shaik-Sirajuddin/graft/actions/workflows/ci.yml/badge.svg)](https://github.com/Shaik-Sirajuddin/graft/actions)

<!-- color bar below heading — capsule-render (MIT, github.com/kyechan99/capsule-render) -->
<img src="https://capsule-render.vercel.app/api?type=rect&color=0:F97316,100:FB923C&height=6" width="100%" alt="" />

> [!NOTE]
> **Agent definitions, skills, instructions, and agent memory — all managed as code.**
>
> Developers use agents across Claude Code, Codex, Copilot, and more. **graft** lets you maintain agent definitions and convert them to and from provider-specific formats. An enhancement to one agent is synced to the others using git-merge-style resolution.

## Flow

```
graft sync agents
```

1. Agents are auto-detected in Claude Code | Codex | Gemini CLI | … 
2. Edit directly via `.codex/agents/designer.toml` or in `.graft/agents/`
3. Run `graft sync agents` to propagate changes to all other provider configs

Share your agent definitions with your team inside your existing codebase repo.

---

## What it does

> [!TIP]
> - **Team collaboration** — agent definitions live in `.graft/agents/` alongside your code: versioned, reviewed, and shared via git.
> - **Two-way sync** — edit at any provider; graft reads the change and writes it back to all the others.
> - **Auto resolution** — concurrent edits are merged using a branch-per-file strategy with conflict detection.
> - **Skills, auto-synced** — drop a skill in `.agents/skills/` and graft symlinks it into every supporting provider's skills dir on `graft sync`.
> - **Broad coverage** — syncs across **9 providers** (Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, OpenCode, Roo Code, Goose, Grok CLI) — and growing.

## Example config

```yaml
# .graft/agents/designer/agent.yaml
name: designer
description: UI/UX design reviewer
model: claude-sonnet-4-6          # default model for every provider
tools:
  - read_file
  - web_search

# Per-provider overrides — override any field a provider supports
# (model, tools, temperature, …). `name` stays canonical and is never overridden.
providerOverrides:
  claude-code:
    model: claude-opus-4-8
  github-copilot:
    model: gpt-5.4
  opencode:
    temperature: 0.2
```

> The agent's system prompt / instructions live alongside it in `.graft/agents/designer/instructions.md`.

Place this directory in `.graft/agents/<name>/` and run `graft sync agents` — graft writes the equivalent config for every enabled provider, applying each provider's overrides on top of the shared defaults.

### Editor validation (JSON Schema)

`.graft/agents/*/agent.yaml` is described by a [JSON Schema](internal/canonical/schema/common-agent-definition.schema.json) — point your editor at it for validation and autocomplete. In VS Code (Red Hat YAML extension):

```jsonc
// .vscode/settings.json
"yaml.schemas": {
  "internal/canonical/schema/common-agent-definition.schema.json": "**/.graft/agents/*/agent.yaml"
}
```

…or add a modeline to the top of any `agent.yaml`: `# yaml-language-server: $schema=<path-or-url>`.

## Supported providers

| Provider | Config location |
|---|---|
| **claude-code** | `.claude/agents/<name>.md` |
| **codex** | `.codex/agents/<name>.toml` |
| **gemini-cli** | `.gemini/agents/<name>.md` |
| **cursor** | `.cursor/agents/<name>.mdc` |
| **github-copilot** | `.github/copilot-instructions.md` |
| **opencode** | `.opencode/agents/<name>.toml` |
| **roo-code** | `.roo/agents/<name>.md` |
| **goose** | `.goose/agents/<name>.yaml` |
| **grok-cli** | `.grok/agents/<name>.md` |

## Getting started

See the **[documentation site](https://docs.graft.dev)** for installation instructions, a quickstart, and the full command reference.

### Quick install

```sh
go install github.com/Shaik-Sirajuddin/graft/cmd/graft@latest
```

### Basic usage

```sh
# Initialize graft in your repo
graft init

# List detected agents across all providers
graft agent list

# Sync all agents to every provider
graft sync agents

# Check drift
graft agents status
```
