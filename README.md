# graft

**Collaboratively develop agents with your team.**

[![Go Report Card](https://goreportcard.com/badge/github.com/Shaik-Sirajuddin/graft)](https://goreportcard.com/report/github.com/Shaik-Sirajuddin/graft)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)](go.mod)
[![CI](https://github.com/Shaik-Sirajuddin/graft/actions/workflows/ci.yml/badge.svg)](https://github.com/Shaik-Sirajuddin/graft/actions)

---

Agent definitions, skills, instructions, and agent memory — all managed as code.

Developers use agents across Claude Code, Codex, Copilot, and more. **graft** lets you maintain agent definitions and convert them to and from provider-specific formats. An enhancement to one agent is synced to the others using git-merge-style resolution.

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

| Capability | Description |
|---|---|
| **Team collaboration** | Agent definitions live in `.graft/agents/` alongside your code — versioned, reviewed, and shared via git |
| **Two-way sync** | Edit at any provider; graft reads the change and writes it back to all others |
| **Auto resolution** | Concurrent edits are merged using a branch-per-file strategy with conflict detection |

## Example config

```yaml
# .graft/agents/designer/agent.yaml
name: designer
description: UI/UX design reviewer
model: claude-sonnet-4-5
instructions: |
  You are a design-focused reviewer. Focus on usability,
  accessibility, and visual consistency.
tools:
  - read_file
  - web_search
```

Place this file in `.graft/agents/<name>/agent.yaml` and run `graft sync agents` — graft writes the equivalent config for every enabled provider.

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
