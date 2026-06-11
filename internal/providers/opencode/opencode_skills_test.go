package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsSupported(t *testing.T) {
	sp := SkillProvider()
	if !sp.SkillsSupported() {
		t.Fatal("expected SkillsSupported() == true")
	}
	if sp.Name() != name {
		t.Errorf("Name()=%q want %q", sp.Name(), name)
	}
	root := "/ws"
	want := filepath.Join(root, ".opencode/skills")
	if got := sp.SkillDir(root); got != want {
		t.Errorf("SkillDir=%q want %q", got, want)
	}
}

func TestDetectSkills(t *testing.T) {
	sp := SkillProvider()
	root := t.TempDir()

	// No skills dir yet -> nil, nil (not an error).
	refs, err := sp.DetectSkills(root)
	if err != nil {
		t.Fatalf("DetectSkills on empty root: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(refs))
	}

	// Provision one valid skill dir (with SKILL.md) and one bare dir (skipped).
	skillDir := filepath.Join(sp.SkillDir(root), "pdf-filler")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: pdf-filler\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sp.SkillDir(root), "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	refs, err = sp.DetectSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(refs), refs)
	}
	r := refs[0]
	if r.Name != "pdf-filler" || r.Provider != name || r.Path != skillDir {
		t.Errorf("unexpected ref: %+v (want name=pdf-filler provider=%s path=%s)", r, name, skillDir)
	}
}
