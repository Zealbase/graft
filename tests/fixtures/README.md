# graft e2e fixtures

Predefined inputs consumed by the end-to-end suite (`tests/e2e`). Each fixture is
copied into a throwaway `t.TempDir()` workspace; nothing here is mutated.

## Layout

- `agents/claude/` — Claude Code provider agent files (`.claude/agents/<name>.md`),
  YAML frontmatter + Markdown body.
  - `code-reviewer.md` — minimal valid agent (canonical fields only).
  - `planner.md` — valid agent carrying non-canonical frontmatter keys
    (`disallowedTools`, `permissionMode`) that must survive a sync via
    `ProviderOverrides["claude-code"]` (lossless round-trip case).
- `agents/invalid/agent.yaml` — a canonical agent missing required fields
  (no `name`/body); used to trip `validate` and the pre-sync validate gate.

## Merge-conflict note

The plan calls for a merge-conflict fixture (`tests/fixtures/`) since merges
operate on canonical form. With the current branch-per-agent sync engine an
*organic* merge conflict is structurally unreachable through the binary (each
agent owns its own `.graft/agents/<name>/` files; one agent branch merging into a
beta cut from the same base is always clean). The suite therefore documents this
and asserts the reachable conflict plumbing (resume-blocked guard, conflict
table) rather than a fabricated fixture. See `conflict_e2e_test.go`.
