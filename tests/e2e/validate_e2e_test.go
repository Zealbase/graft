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

// TestValidate_EmptyDescriptionBlocksSync verifies the v0.0.4 description rule:
// a canonical agent with an empty description must fail `graft validate --all`
// with an error-severity finding AND block `graft sync agents`. Once a
// non-empty description is written, both passes succeed.
func TestValidate_EmptyDescriptionBlocksSync(t *testing.T) {
	root := newGitWorkspace(t)
	writeFile(t, root, "README.md", "seed\n")
	gitCommitAll(t, root, "seed")
	mustGraft(t, root, "init")

	// Scaffold an agent — description is empty by design (BuildDefault leaves it
	// empty so the user is required to fill it in before syncing).
	mustGraft(t, root, "agent", "init", "my-agent", "You handle deployments.")

	// validate --all must fail with an error finding for the empty description.
	rValidate := graft(t, root, "validate", "--all", "-o", "json")
	if rValidate.exitCode == 0 {
		t.Fatalf("validate of agent with empty description exited 0 (want non-zero)\nstdout: %s", rValidate.stdout)
	}
	var findings []finding
	decodeJSON(t, rValidate, &findings)
	sawDescError := false
	for _, f := range findings {
		if f.Severity == "error" {
			sawDescError = true
			break
		}
	}
	if !sawDescError {
		t.Fatalf("expected error-severity finding for empty description, got: %+v", findings)
	}

	// sync must also be blocked (pre-sync gate runs validate).
	rSync := graft(t, root, "sync", "agents", "-o", "json")
	if rSync.exitCode == 0 {
		t.Fatalf("sync of agent with empty description exited 0 (want validation block)\nstdout: %s\nstderr: %s",
			rSync.stdout, rSync.stderr)
	}
	// No provider file should have been written.
	if exists(root, ".claude/agents/my-agent.md") {
		t.Fatal("blocked sync must not write any provider file")
	}

	// Now add a description directly to the canonical YAML.
	agentYAML := ".graft/agents/my-agent/agent.yaml"
	raw := readFile(t, root, agentYAML)
	// Insert description after the name line.
	updated := ""
	for _, line := range splitLines(raw) {
		updated += line + "\n"
		if len(line) >= 5 && line[:5] == "name:" {
			updated += "description: Handles deployment automation and rollout pipelines.\n"
		}
	}
	writeFile(t, root, agentYAML, updated)

	// validate --all must now pass (exit 0).
	rValidate2 := graft(t, root, "validate", "--all", "-o", "json")
	if rValidate2.exitCode != 0 {
		t.Fatalf("validate after adding description exit=%d (want 0)\nstdout: %s\nstderr: %s",
			rValidate2.exitCode, rValidate2.stdout, rValidate2.stderr)
	}

	// sync must now succeed.
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync after description set status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "my-agent") {
		t.Fatalf("sync after description set changed=%v, want my-agent", res.Changed)
	}
	// Provider file now exists.
	if !exists(root, ".claude/agents/my-agent.md") {
		t.Fatal("sync after description set must write the claude provider file")
	}
}
