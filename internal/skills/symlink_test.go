package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// makeCanonical creates a canonical skill dir <root>/.agents/skills/<name> with a
// SKILL.md and returns its absolute path.
func makeCanonical(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLink_AbsentCreates(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if st != contract.SkillLinked {
		t.Fatalf("state = %q, want linked", st)
	}
	assertSymlinkTo(t, target, canon)
}

func TestLink_CorrectIsNoOp(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	// First link.
	if _, err := Link(canon, target, false); err != nil {
		t.Fatal(err)
	}
	info1, _ := os.Lstat(target)

	// Second link is idempotent: still linked, symlink unchanged.
	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link idempotent: %v", err)
	}
	if st != contract.SkillLinked {
		t.Fatalf("state = %q, want linked", st)
	}
	info2, _ := os.Lstat(target)
	if info1.ModTime() != info2.ModTime() {
		t.Errorf("symlink was recreated on no-op (mtime changed)")
	}
	assertSymlinkTo(t, target, canon)
}

func TestLink_WrongTargetRelinks(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	other := makeCanonical(t, root, "other")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	// Point the link at the WRONG canonical dir first.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatal(err)
	}

	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if st != contract.SkillLinked {
		t.Fatalf("state = %q, want linked", st)
	}
	assertSymlinkTo(t, target, canon)
}

func TestLink_DanglingRelinks(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	// Dangling symlink -> a non-existent path.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "nope", "gone"), target); err != nil {
		t.Fatal(err)
	}
	// LiveState should classify a dangling/broken symlink as dead before we fix
	// it (v0.0.4 verify: a dangling link is NOT linked and NOT wrong-link — its
	// target is missing). Link still heals it back to linked.
	if got, _ := LiveState(canon, target); got != contract.SkillDead {
		t.Fatalf("dangling LiveState = %q, want dead", got)
	}

	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if st != contract.SkillLinked {
		t.Fatalf("state = %q, want linked", st)
	}
	assertSymlinkTo(t, target, canon)
}

func TestLink_RealDirConflictWithoutOverride(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	// A real (non-symlink) directory with content blocks the link.
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if st != contract.SkillConflict {
		t.Fatalf("state = %q, want conflict", st)
	}
	// The real dir must be untouched (not replaced) without override.
	if fi, _ := os.Lstat(target); fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("real dir was replaced by a symlink without override")
	}
}

func TestLink_RealDirOverrideReplaces(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := Link(canon, target, true)
	if err != nil {
		t.Fatalf("Link override: %v", err)
	}
	if st != contract.SkillLinked {
		t.Fatalf("state = %q, want linked", st)
	}
	assertSymlinkTo(t, target, canon)
}

func TestLink_RealFileConflict(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	// A real file (not a dir) at the target.
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Link(canon, target, false)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if st != contract.SkillConflict {
		t.Fatalf("state = %q, want conflict", st)
	}
}

func TestLiveState_Matrix(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")

	// missing
	missing := filepath.Join(root, ".claude", "skills", "alpha")
	if got, _ := LiveState(canon, missing); got != contract.SkillMissing {
		t.Errorf("absent LiveState = %q, want missing", got)
	}
	// linked
	if _, err := Link(canon, missing, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := LiveState(canon, missing); got != contract.SkillLinked {
		t.Errorf("linked LiveState = %q, want linked", got)
	}
	// conflict (real dir)
	cdir := filepath.Join(root, ".opencode", "skills", "alpha")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := LiveState(canon, cdir); got != contract.SkillConflict {
		t.Errorf("real-dir LiveState = %q, want conflict", got)
	}
}

func TestIsSymlinkTo_RelativeAndAbsolute(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "alpha")

	// Relative symlink to the canonical dir.
	rel, err := filepath.Rel(dir, canon)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, target); err != nil {
		t.Fatal(err)
	}
	ok, err := IsSymlinkTo(target, canon)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("relative symlink not recognized as pointing to canonical")
	}
}

// TestLiveState_DeadWhenCanonicalDeleted verifies that after the canonical skill
// dir is removed, an existing symlink that lexically points at it is classified
// SkillDead (not SkillLinked) — the existence check, not just a lexical match.
func TestLiveState_DeadWhenCanonicalDeleted(t *testing.T) {
	root := t.TempDir()
	canon := makeCanonical(t, root, "alpha")
	target := filepath.Join(root, ".claude", "skills", "alpha")

	// Link it correctly first.
	if _, err := Link(canon, target, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := LiveState(canon, target); got != contract.SkillLinked {
		t.Fatalf("pre-delete LiveState = %q, want linked", got)
	}

	// Delete the canonical skill -> the provider symlink is now dangling.
	if err := os.RemoveAll(canon); err != nil {
		t.Fatal(err)
	}
	// LiveState must NOT report linked just because the readlink target lexically
	// equals canon; the target is gone -> dead.
	if got, _ := LiveState(canon, target); got != contract.SkillDead {
		t.Fatalf("post-delete LiveState = %q, want dead", got)
	}
}

// TestIsSymlinkTo_LexicalMatchButMissingTarget guards the fast-path fix: a
// symlink whose readlink target lexically equals want but whose target does NOT
// exist must report false (a dead link is not "linked").
func TestIsSymlinkTo_LexicalMatchButMissingTarget(t *testing.T) {
	root := t.TempDir()
	canon := filepath.Join(root, ".agents", "skills", "alpha") // never created
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "alpha")
	// Absolute symlink that lexically matches canon, but canon does not exist.
	if err := os.Symlink(canon, target); err != nil {
		t.Fatal(err)
	}
	ok, err := IsSymlinkTo(target, canon)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("dangling lexical-match symlink reported as linked; want not-linked")
	}
}

func assertSymlinkTo(t *testing.T, path, want string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", path)
	}
	ok, err := IsSymlinkTo(path, want)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		got, _ := os.Readlink(path)
		t.Fatalf("%s -> %q, want -> %q", path, got, want)
	}
}
