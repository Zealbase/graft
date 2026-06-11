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
