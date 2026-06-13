package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkillDir creates a standalone skill dir (SKILL.md + an asset) at path.
func writeSkillDir(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "helper.sh"), []byte("echo hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestStore_ListEmptyAndMissing(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	got, err := s.List()
	if err != nil {
		t.Fatalf("List on missing store: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List = %v, want empty", got)
	}
	if s.Has("anything") {
		t.Fatalf("Has on missing store returned true")
	}
}

func TestStore_ListSkipsNonSkillDirs(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "good")
	// A dir without SKILL.md must be ignored.
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "notaskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewStore(root)
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("List = %+v, want only [good]", got)
	}
}

func TestStore_InstallCopyIn(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "mover")
	writeSkillDir(t, src, "# mover skill\n")

	s := NewStore(root)
	sk, err := s.Install(src, "")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if sk.Name != "mover" {
		t.Fatalf("installed name = %q, want mover", sk.Name)
	}
	// Canonical dir + content + asset copied in.
	if !s.Has("mover") {
		t.Fatalf("store does not have installed skill")
	}
	if b, _ := os.ReadFile(filepath.Join(sk.Dir, "SKILL.md")); string(b) != "# mover skill\n" {
		t.Fatalf("SKILL.md content mismatch: %q", b)
	}
	if _, err := os.Stat(filepath.Join(sk.Dir, "helper.sh")); err != nil {
		t.Fatalf("asset not copied: %v", err)
	}
}

func TestStore_InstallNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	// Pre-existing canonical skill with distinct content.
	canon := makeCanonical(t, root, "keep")
	if err := os.WriteFile(filepath.Join(canon, "SKILL.md"), []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A different source with the same name must NOT overwrite.
	src := filepath.Join(t.TempDir(), "keep")
	writeSkillDir(t, src, "REPLACEMENT\n")

	s := NewStore(root)
	if _, err := s.Install(src, "keep"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(canon, "SKILL.md")); string(b) != "ORIGINAL\n" {
		t.Fatalf("Install overwrote existing canonical: %q", b)
	}
}

func TestStore_InstallRejectsNonSkill(t *testing.T) {
	root := t.TempDir()
	notSkill := filepath.Join(t.TempDir(), "plain")
	if err := os.MkdirAll(notSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewStore(root)
	if _, err := s.Install(notSkill, ""); err == nil {
		t.Fatalf("expected error installing a non-skill dir")
	}
}
