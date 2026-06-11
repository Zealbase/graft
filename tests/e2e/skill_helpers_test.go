package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// Skills e2e helpers. Skills are reconciled purely on the filesystem (symlinks);
// there is NO db, so every verifier here is lstat/readlink based.

// supportingSkillDirs maps the three supporting providers to their workspace-
// relative skills dirs. The other seven providers support no skills and their
// dirs must never be created by skill operations.
var supportingSkillDirs = map[string]string{
	"claude-code": ".claude/skills",
	"gemini-cli":  ".gemini/skills",
	"opencode":    ".opencode/skills",
}

// nonSupportingSkillDirs are provider dirs that skills must NEVER create. Listed
// as the dirs a skills fan-out could plausibly touch for the other 7 providers.
var nonSupportingSkillDirs = []string{
	".codex/skills", ".cursor/skills", ".github/skills",
	".grok/skills", ".roo/skills", ".goose/skills", ".antigravity/skills",
}

// provLinkPath returns the absolute path where provider's symlink for skill
// should live: <root>/<provDir>/<skill>.
func provLinkPath(root, provider, skill string) string {
	return filepath.Join(root, supportingSkillDirs[provider], skill)
}

// canonicalSkillDir returns the absolute canonical skill dir:
// <root>/.agent/skills/<skill>.
func canonicalSkillDir(root, skill string) string {
	return filepath.Join(root, ".agent", "skills", skill)
}

// writeCanonicalSkill provisions a canonical skill at .agent/skills/<name>/SKILL.md.
func writeCanonicalSkill(t *testing.T, root, name, body string) {
	t.Helper()
	writeFile(t, root, filepath.Join(".agent", "skills", name, "SKILL.md"),
		"---\nname: "+name+"\n---\n"+body+"\n")
}

// provisionState sets the provider link path for (provider, skill) into one of
// the matrix states. root must already contain the canonical skill.
//
//	"absent"   — nothing at the path
//	"correct"  — symlink -> canonical skill dir
//	"wrong"    — symlink -> some other (existing) dir
//	"dangling" — symlink -> a deleted target
//	"real"     — a real directory with user SKILL.md content
func provisionState(t *testing.T, root, provider, skill, state string) {
	t.Helper()
	link := provLinkPath(root, provider, skill)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(link)
	switch state {
	case "absent":
		// nothing
	case "correct":
		if err := os.Symlink(canonicalSkillDir(root, skill), link); err != nil {
			t.Fatal(err)
		}
	case "wrong":
		other := filepath.Join(root, "other-target")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, link); err != nil {
			t.Fatal(err)
		}
	case "dangling":
		gone := filepath.Join(root, "deleted-target")
		if err := os.Symlink(gone, link); err != nil {
			t.Fatal(err)
		}
	case "real":
		if err := os.MkdirAll(link, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("USER CONTENT\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown provision state %q", state)
	}
}

// --- FS verifiers (lstat / readlink) -------------------------------------

// lstatMode returns the FileMode from an lstat (does not follow symlinks).
func lstatMode(t *testing.T, path string) (os.FileMode, bool) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false
		}
		t.Fatalf("lstat %s: %v", path, err)
	}
	return fi.Mode(), true
}

// assertLinkedTo asserts path is a symlink whose target is canonical (the
// linked state). It checks lstat says symlink AND readlink resolves to want.
func assertLinkedTo(t *testing.T, path, want string) {
	t.Helper()
	mode, ok := lstatMode(t, path)
	if !ok {
		t.Fatalf("expected symlink at %s, got nothing", path)
	}
	if mode&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s, got mode %v", path, mode)
	}
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(path), resolved)
	}
	if filepath.Clean(resolved) != filepath.Clean(want) {
		t.Fatalf("symlink %s -> %s, want -> %s", path, resolved, want)
	}
}

// assertNotSymlink asserts path exists but is NOT a symlink (a real dir/file).
func assertRealDir(t *testing.T, path string) {
	t.Helper()
	mode, ok := lstatMode(t, path)
	if !ok {
		t.Fatalf("expected a real dir at %s, got nothing", path)
	}
	if mode&os.ModeSymlink != 0 {
		t.Fatalf("expected a real dir at %s, got a symlink", path)
	}
	if !mode.IsDir() {
		t.Fatalf("expected a dir at %s, got mode %v", path, mode)
	}
}

// linkTargetMtime returns the symlink's own mtime (lstat, not the target's),
// used to assert idempotency leaves the link untouched.
func linkTargetMtime(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return fi.ModTime().UnixNano()
}

// skillStatusJSON decodes a `skill status/sync/install -o json` payload.
type skillStatusJSON struct {
	Skill    string `json:"skill"`
	Provider string `json:"provider"`
	State    string `json:"state"`
	LinkPath string `json:"link_path"`
}

// stateOf returns the reported state for (provider, skill) in a status slice.
func stateOf(states []skillStatusJSON, provider, skill string) (string, bool) {
	for _, s := range states {
		if s.Provider == provider && s.Skill == skill {
			return s.State, true
		}
	}
	return "", false
}
