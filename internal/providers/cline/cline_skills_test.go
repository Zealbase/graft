package clineprov

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
}

func TestSkillDir(t *testing.T) {
	sp := SkillProvider()
	root := "/ws"
	want := filepath.Join(root, ".cline", "skills")
	if got := sp.SkillDir(root); got != want {
		t.Errorf("SkillDir=%q want %q", got, want)
	}
}

func TestHomeSkillDirs(t *testing.T) {
	sp := SkillProvider()
	dirs := sp.HomeSkillDirs("/home/user")
	wantDirs := []string{
		filepath.Join("/home/user", ".cline", "skills"),
		filepath.Join("/home/user", ".agents", "skills"),
	}
	if len(dirs) != len(wantDirs) {
		t.Fatalf("HomeSkillDirs length: got %d, want %d: %v", len(dirs), len(wantDirs), dirs)
	}
	for i, w := range wantDirs {
		if dirs[i] != w {
			t.Errorf("HomeSkillDirs[%d]: got %q, want %q", i, dirs[i], w)
		}
	}
}

func TestDetectSkillsEmpty(t *testing.T) {
	dir := t.TempDir()
	sp := SkillProvider()
	refs, err := sp.DetectSkills(dir)
	if err != nil {
		t.Fatalf("DetectSkills on empty root: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(refs))
	}
}

func TestDetectSkillsMultiDir(t *testing.T) {
	root := t.TempDir()

	// Create skill in .cline/skills/pdf-filler/SKILL.md
	clineSkillDir := filepath.Join(root, ".cline", "skills", "pdf-filler")
	if err := os.MkdirAll(clineSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clineSkillDir, "SKILL.md"), []byte("---\nname: pdf-filler\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create skill in .agents/skills/code-review/SKILL.md
	agentsSkillDir := filepath.Join(root, ".agents", "skills", "code-review")
	if err := os.MkdirAll(agentsSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsSkillDir, "SKILL.md"), []byte("# code-review"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create duplicate in .agents/skills to test dedup
	dupDir := filepath.Join(root, ".agents", "skills", "pdf-filler")
	if err := os.MkdirAll(dupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dupDir, "SKILL.md"), []byte("# pdf-filler dup"), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := SkillProvider()
	refs, err := sp.DetectSkills(root)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]int)
	for _, r := range refs {
		names[r.Name]++
		if r.Provider != name {
			t.Errorf("ref.Provider=%q want %q", r.Provider, name)
		}
	}
	if names["pdf-filler"] != 1 {
		t.Errorf("expected pdf-filler once, got %d times", names["pdf-filler"])
	}
	if names["code-review"] != 1 {
		t.Errorf("expected code-review once, got %d times", names["code-review"])
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 skill refs, got %d: %v", len(refs), refs)
	}
}

func TestDetectSkillsSingle(t *testing.T) {
	root := t.TempDir()
	sp := SkillProvider()

	skillDir := filepath.Join(sp.SkillDir(root), "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory without SKILL.md should not count.
	if err := os.MkdirAll(filepath.Join(sp.SkillDir(root), "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	refs, err := sp.DetectSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 skill, got %d: %+v", len(refs), refs)
	}
	r := refs[0]
	if r.Name != "my-skill" || r.Provider != name || r.Path != skillDir {
		t.Errorf("unexpected ref: %+v (want name=my-skill provider=%s path=%s)", r, name, skillDir)
	}
}
