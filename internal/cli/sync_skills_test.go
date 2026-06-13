package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCanonSkill seeds a canonical skill under <root>/.agents/skills/<name>.
func writeCanonSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSyncOutputReportsLinkedSkill: a canonical skill missing its provider
// symlink is healed by sync and reported ("linked N skill(s)"), NOT "already in
// sync".
func TestSyncOutputReportsLinkedSkill(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeCanonSkill(t, root, "fresh")

	out, err := execCLI(t, root, nil, "sync", "agents")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(out, "already in sync") {
		t.Fatalf("expected skill-linked report, got 'already in sync':\n%s", out)
	}
	if !strings.Contains(out, "linked") || !strings.Contains(out, "fresh") {
		t.Fatalf("expected 'linked ... fresh' in output:\n%s", out)
	}
	// Symlink actually created at claude-code.
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "fresh")); err != nil {
		t.Fatalf("link not created: %v", err)
	}
}

// TestSyncOutputReportsSkillConflict: a real dir at the link path surfaces a
// warning and the run does NOT claim fully in sync.
func TestSyncOutputReportsSkillConflict(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeCanonSkill(t, root, "blocked")
	// Real dir occupying the claude-code link path.
	conflict := filepath.Join(root, ".claude", "skills", "blocked")
	if err := os.MkdirAll(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "REAL.md"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := execCLI(t, root, nil, "sync", "agents")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "conflict") {
		t.Fatalf("expected skill conflict warning:\n%s", out)
	}
	if strings.Contains(out, "already in sync") {
		t.Fatalf("must not claim 'already in sync' with a skill conflict:\n%s", out)
	}
	// Real dir preserved (no --override).
	if _, err := os.Stat(filepath.Join(conflict, "REAL.md")); err != nil {
		t.Fatalf("conflict dir clobbered: %v", err)
	}
}

// TestSyncOutputInSyncCountsSkills: when agents and skills are all linked, the
// in-sync summary claims the skill count too.
func TestSyncOutputInSyncCountsSkills(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeCanonSkill(t, root, "ready")
	// First sync links the skill and the agent.
	if _, err := execCLI(t, root, nil, "sync", "agents"); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	// Second sync: everything already linked -> "already in sync (N providers, K skills)".
	out, err := execCLI(t, root, nil, "sync", "agents")
	if err != nil {
		t.Fatalf("sync 2: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already in sync") {
		t.Fatalf("expected 'already in sync' on clean re-sync:\n%s", out)
	}
	if !strings.Contains(out, "skill") {
		t.Fatalf("expected skill count in in-sync summary:\n%s", out)
	}
}
