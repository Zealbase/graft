package kilocode

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSkillsSupported verifies the SkillProvider returns SkillsSupported==true and Name=="kilo-code".
func TestSkillsSupported(t *testing.T) {
	sp := SkillProvider()
	if !sp.SkillsSupported() {
		t.Error("expected SkillsSupported() == true")
	}
	if sp.Name() != "kilo-code" {
		t.Errorf("expected Name()=%q, got %q", "kilo-code", sp.Name())
	}
}

// TestSkillDir verifies SkillDir("/ws") == "/ws/.kilo/skills".
func TestSkillDir(t *testing.T) {
	sp := SkillProvider()
	got := sp.SkillDir("/ws")
	want := filepath.Join("/ws", ".kilo", "skills")
	if got != want {
		t.Errorf("SkillDir: got %q, want %q", got, want)
	}
}

// TestHomeSkillDirs verifies HomeSkillDirs includes the expected directories,
// including the XDG path ~/.config/kilo/skills.
func TestHomeSkillDirs(t *testing.T) {
	sp := SkillProvider()
	dirs := sp.HomeSkillDirs("/home/user")
	wantDirs := []string{
		filepath.Join("/home/user", ".kilo", "skills"),
		filepath.Join("/home/user", ".config", "kilo", "skills"),
		filepath.Join("/home/user", ".kilocode", "skills"),
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

// TestDetectSkillsEmpty verifies that a temp dir with no skill dirs returns nil,nil.
func TestDetectSkillsEmpty(t *testing.T) {
	dir := t.TempDir()
	sp := SkillProvider()
	refs, err := sp.DetectSkills(dir)
	if err != nil {
		t.Fatalf("DetectSkills: unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected no skill refs, got %d: %v", len(refs), refs)
	}
}

// TestDetectSkillsMultiDir creates skills in .kilo/skills and .kilocode/skills
// and verifies both appear deduped by name.
func TestDetectSkillsMultiDir(t *testing.T) {
	root := t.TempDir()

	// Create skill in .kilo/skills/alpha/SKILL.md
	kiloSkillDir := filepath.Join(root, ".kilo", "skills", "alpha")
	if err := os.MkdirAll(kiloSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kiloSkillDir, "SKILL.md"), []byte("# alpha"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create skill in .kilocode/skills/beta/SKILL.md
	kilocodeSkillDir := filepath.Join(root, ".kilocode", "skills", "beta")
	if err := os.MkdirAll(kilocodeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kilocodeSkillDir, "SKILL.md"), []byte("# beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a duplicate skill in .agents/skills/alpha/SKILL.md to test dedup.
	agentsSkillDir := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(agentsSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsSkillDir, "SKILL.md"), []byte("# alpha dup"), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := SkillProvider()
	refs, err := sp.DetectSkills(root)
	if err != nil {
		t.Fatalf("DetectSkills: %v", err)
	}

	// Expect alpha and beta (alpha deduped — only one entry)
	names := make(map[string]int)
	for _, r := range refs {
		names[r.Name]++
	}
	if names["alpha"] != 1 {
		t.Errorf("expected alpha once, got %d times", names["alpha"])
	}
	if names["beta"] != 1 {
		t.Errorf("expected beta once, got %d times", names["beta"])
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 skill refs (alpha+beta), got %d: %v", len(refs), refs)
	}
}
