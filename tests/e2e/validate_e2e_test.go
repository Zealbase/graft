package e2e

import (
	"testing"
)

// Scenario 6: validate clean pass; an invalid agent -> validate non-zero with
// findings; and the pre-sync validate gate blocks sync.
func TestValidate_CleanPass(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// validate --all -> exit 0 (no error findings).
	r := graft(t, root, "validate", "--all", "-o", "json")
	if r.exitCode != 0 {
		t.Fatalf("validate --all exit=%d, want 0\nstdout:%s\nstderr:%s", r.exitCode, r.stdout, r.stderr)
	}
	// findings JSON is null/empty for a clean tree.
	var findings []finding
	decodeJSON(t, r, &findings)
	for _, f := range findings {
		if f.Severity == "error" {
			t.Fatalf("unexpected error finding on clean tree: %+v", f)
		}
	}
}

func TestValidate_InvalidAgent_NonZeroWithFindings(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	// Seed an invalid canonical agent directly under .graft/agents.
	provisionInvalidCanonical(t, root, "broken")

	r := graft(t, root, "validate", "--all", "-o", "json")
	if r.exitCode == 0 {
		t.Fatalf("validate over invalid agent exit=0, want non-zero\nstdout:%s", r.stdout)
	}
	var findings []finding
	decodeJSON(t, r, &findings)
	if len(findings) == 0 {
		t.Fatalf("expected validation findings, got none")
	}
	sawError := false
	for _, f := range findings {
		if f.Severity == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("expected at least one error-severity finding: %+v", findings)
	}
}

func TestValidate_PreSyncGateBlocks(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	provisionInvalidCanonical(t, root, "broken")

	// sync agents must be blocked by the pre-sync validate gate (non-zero exit,
	// no sync_run reaching done for the broken agent path).
	r := graft(t, root, "sync", "agents", "-o", "json")
	if r.exitCode == 0 {
		t.Fatalf("sync over invalid agent exit=0, want validation block\nstdout:%s", r.stdout)
	}
	if !contains(r.stderr, "validation") {
		t.Fatalf("expected validation-block message on stderr, got: %s", r.stderr)
	}
}

func TestValidate_UnknownProvider_NonZero(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	r := graft(t, root, "validate", "-p", "no-such-provider", "-o", "json")
	if r.exitCode == 0 {
		t.Fatalf("validate -p unknown exit=0, want non-zero")
	}
}

func TestValidate_ProviderAndAllMutuallyExclusive(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	r := graft(t, root, "validate", "-p", "claude-code", "--all")
	if r.exitCode == 0 {
		t.Fatalf("validate -p X --all exit=0, want mutually-exclusive error")
	}
}

// TestValidate_InitDefaultDescriptionUnblocksSync verifies the init UX fix: a
// freshly-scaffolded agent gets a non-empty default description ("<name> agent"),
// so it passes `graft validate --all` and is NOT blocked by the pre-sync
// description gate — no manual description editing required.
//
// The description-required rule itself (which blocks a genuinely empty
// description) is still covered by TestValidate_PreSyncGateBlocks and
// TestValidate_InvalidAgent_NonZeroWithFindings via provisionInvalidCanonical.
func TestValidate_InitDefaultDescriptionUnblocksSync(t *testing.T) {
	root := newGitWorkspace(t)
	writeFile(t, root, "README.md", "seed\n")
	gitCommitAll(t, root, "seed")
	mustGraft(t, root, "init")

	// Scaffold an agent (skip the auto-sync so this test focuses on the
	// description default + the explicit validate/sync calls below).
	mustGraft(t, root, "agent", "init", "my-agent", "You handle deployments.", "--no-sync")

	// The default description is "<name> agent" and non-empty.
	agentYAML := ".graft/agents/my-agent/agent.yaml"
	raw := readFile(t, root, agentYAML)
	if !contains(raw, "my-agent agent") {
		t.Fatalf("expected default description %q in agent.yaml:\n%s", "my-agent agent", raw)
	}

	// validate --all must pass (exit 0) — no empty-description error.
	rValidate := graft(t, root, "validate", "--all", "-o", "json")
	if rValidate.exitCode != 0 {
		t.Fatalf("validate of freshly-scaffolded agent exit=%d (want 0)\nstdout: %s\nstderr: %s",
			rValidate.exitCode, rValidate.stdout, rValidate.stderr)
	}

	// sync must succeed (previously blocked by the empty-description gate).
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync of freshly-scaffolded agent status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "my-agent") {
		t.Fatalf("sync changed=%v, want my-agent", res.Changed)
	}
	// Provider file now exists.
	if !exists(root, ".claude/agents/my-agent.md") {
		t.Fatal("sync must write the claude provider file")
	}
}
