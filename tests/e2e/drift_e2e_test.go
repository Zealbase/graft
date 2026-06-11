package e2e

import (
	"testing"
)

// Scenario 4: out-of-band edit a provider file -> agents status shows drift ->
// sync agents reconverges.
func TestDrift_DetectedThenReconverge(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Confirm in-sync first.
	var before statusReport
	decodeJSON(t, mustGraft(t, root, "agents", "status", "-o", "json"), &before)
	if !before.Agents[0].Providers["claude-code"] {
		t.Fatalf("expected claude-code in sync before tamper: %+v", before)
	}

	// Out-of-band edit: append junk to the generated claude file.
	claudePath := ".claude/agents/code-reviewer.md"
	tampered := readFile(t, root, claudePath) + "\n# tampered out of band\n"
	writeFile(t, root, claudePath, tampered)

	// raw + db-independent: agents status now reports drift for claude-code.
	var drift statusReport
	decodeJSON(t, mustGraft(t, root, "agents", "status", "-o", "json"), &drift)
	if drift.Agents[0].Providers["claude-code"] {
		t.Fatalf("expected claude-code drift after tamper, got in-sync: %+v", drift)
	}
	if drift.Agents[0].InSync {
		t.Fatalf("agent should be out of sync after tamper: %+v", drift.Agents[0])
	}
	if drift.OutOfSyncProviders["claude-code"] != 1 {
		t.Fatalf("out_of_sync_providers[claude-code]=%d, want 1", drift.OutOfSyncProviders["claude-code"])
	}

	// sync agents reconverges.
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("reconverge sync status=%q, want done", res.Status)
	}

	var after statusReport
	decodeJSON(t, mustGraft(t, root, "agents", "status", "-o", "json"), &after)
	if !after.Agents[0].Providers["claude-code"] {
		t.Fatalf("claude-code still drifted after reconverge: %+v", after)
	}
	if !after.Agents[0].InSync {
		t.Fatalf("agent not in sync after reconverge: %+v", after.Agents[0])
	}
}
