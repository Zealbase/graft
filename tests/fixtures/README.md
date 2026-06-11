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

## Skills suite (no static fixtures)

The skills e2e suite (`skill_*_test.go`) provisions all states PROGRAMMATICALLY
in the tmp workspace rather than from static fixtures, because the provider-dir
state matrix needs live symlinks / dangling links / real-dir conflicts that
cannot be checked-in portably. `skill_helpers_test.go` builds each state
(absent / correct / wrong / dangling / real) under the supporting provider dirs
(`.claude/skills`, `.gemini/skills`, `.opencode/skills`) and verifies via
lstat/readlink (no db — skills keep no link table).

## Merge cases (`merge/`)

Since core's per-provider-file canonical merge landed, real conflicts ARE
reachable through the binary. Each merge fixture defines the SAME agent (`dev`)
in two providers; the suite drops `claude.md` -> `.claude/agents/dev.md` and
`opencode.md` -> `.opencode/agents/dev.md` so both are detected as changed.

- `merge/conflict-model/` — both providers set `model` differently
  (opus vs sonnet) on an otherwise identical agent -> SAME canonical field
  diverges -> real git conflict (markers in `.graft/agents/dev/agent.yaml`).
  Drives the conflict + resolution (select-source / select-target / manual /
  leftover-markers) cases.
- `merge/automerge-fields/` — providers differ on DISJOINT canonical lines
  (claude `tools`, opencode `temperature` override) -> git auto-merges, no
  conflict; both edits survive.
- `merge/automerge-capability/` — capability variance: claude expresses `tools`
  (a field opencode's canonical does not), all shared fields agree -> auto-merge.

See `conflict_e2e_test.go` + `conflict_helpers_test.go` (marker resolvers and the
bare-re-run / `--continue` resume helper).
