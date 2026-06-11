package e2e

import (
	"testing"
)

// PROVIDER-DIR STATE MATRIX (plan-skills 05 / 02). For each supporting provider
// and a single skill, provision <provDir>/<skill> in a state and assert the
// resulting link state + action of `skill sync` (with and without --override).
// All verification is FS (lstat/readlink) + raw (-o json + exit code); no db.
//
// The init/sync skill hook auto-applies, so each test DISABLES the hook before
// init (config skills.enabled=false) and then provisions the state, so the
// transition under test is performed by the explicit `skill sync` we invoke —
// not silently by a hook.

// initWithSkillHookDisabled git-inits, disables the skill hook, runs init, then
// provisions a canonical skill (so init's hook — even though disabled — never
// pre-links). Returns the workspace root.
func initSkillWorkspace(t *testing.T, skill string) string {
	t.Helper()
	root := newGitWorkspace(t)
	// Disable the hook so init/sync never auto-link; the test drives skill sync.
	mustGraft(t, root, "config", "set", "--skills.enabled", "false")
	mustGraft(t, root, "init")
	writeCanonicalSkill(t, root, skill, "Body of "+skill)
	return root
}

func TestSkillMatrix_Absent_Links(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "absent")

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "linked" {
		t.Fatalf("absent -> sync: state=%q, want linked", s)
	}
	assertLinkedTo(t, provLinkPath(root, "claude-code", "hello"), canonicalSkillDir(root, "hello"))
}

func TestSkillMatrix_CorrectSymlink_Idempotent(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "correct")
	link := provLinkPath(root, "claude-code", "hello")
	before := linkTargetMtime(t, link)

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "linked" {
		t.Fatalf("correct symlink -> sync: state=%q, want linked", s)
	}
	// Idempotent: the symlink itself (target + mtime) is unchanged.
	assertLinkedTo(t, link, canonicalSkillDir(root, "hello"))
	if after := linkTargetMtime(t, link); after != before {
		t.Fatalf("idempotent sync changed the symlink mtime: %d -> %d", before, after)
	}
}

func TestSkillMatrix_WrongTarget_ReLinked(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "wrong")

	// status must classify it as wrong-link before sync.
	var st []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "status", "-p", "claude-code", "-o", "json"), &st)
	if s, _ := stateOf(st, "claude-code", "hello"); s != "wrong-link" {
		t.Fatalf("wrong target status=%q, want wrong-link", s)
	}

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "linked" {
		t.Fatalf("wrong target -> sync: state=%q, want linked", s)
	}
	assertLinkedTo(t, provLinkPath(root, "claude-code", "hello"), canonicalSkillDir(root, "hello"))
}

func TestSkillMatrix_Dangling_ReLinked(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "dangling")

	var st []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "status", "-p", "claude-code", "-o", "json"), &st)
	if s, _ := stateOf(st, "claude-code", "hello"); s != "wrong-link" {
		t.Fatalf("dangling status=%q, want wrong-link", s)
	}

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "linked" {
		t.Fatalf("dangling -> sync: state=%q, want linked", s)
	}
	assertLinkedTo(t, provLinkPath(root, "claude-code", "hello"), canonicalSkillDir(root, "hello"))
}

func TestSkillMatrix_RealDir_ConflictUntouched_NoOverride(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "real")
	link := provLinkPath(root, "claude-code", "hello")

	// status: conflict.
	var st []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "status", "-p", "claude-code", "-o", "json"), &st)
	if s, _ := stateOf(st, "claude-code", "hello"); s != "conflict" {
		t.Fatalf("real dir status=%q, want conflict", s)
	}

	// sync without --override: still conflict, user content left UNTOUCHED.
	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "conflict" {
		t.Fatalf("real dir -> sync (no override): state=%q, want conflict", s)
	}
	assertRealDir(t, link)
	if got := readFile(t, root, ".claude/skills/hello/SKILL.md"); got != "USER CONTENT\n" {
		t.Fatalf("user content modified without --override: %q", got)
	}
}

func TestSkillMatrix_RealDir_OverrideReplaces(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "real")
	link := provLinkPath(root, "claude-code", "hello")

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "--override", "-o", "json"), &states)
	if s, _ := stateOf(states, "claude-code", "hello"); s != "linked" {
		t.Fatalf("real dir -> sync --override: state=%q, want linked", s)
	}
	// The real user entry is gone; replaced by a symlink to canonical.
	assertLinkedTo(t, link, canonicalSkillDir(root, "hello"))
	// And the user content is no longer reachable as a real file at that path
	// (the symlink resolves to canonical, which carries the canonical SKILL.md).
	if got := readFile(t, root, ".claude/skills/hello/SKILL.md"); got == "USER CONTENT\n" {
		t.Fatalf("user content survived --override (expected replaced): %q", got)
	}
}
