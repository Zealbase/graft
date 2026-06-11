package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// The three supporting providers and their workspace skill dirs.
var supportingDirs = map[string]string{
	"claude-code": filepath.Join(".claude", "skills"),
	"gemini-cli":  filepath.Join(".gemini", "skills"),
	"opencode":    filepath.Join(".opencode", "skills"),
}

func TestRegistry_SupportingIsThreeProviders(t *testing.T) {
	reg := Default()
	sup := reg.Supporting()
	if len(sup) != 3 {
		t.Fatalf("Supporting() = %d providers, want 3", len(sup))
	}
	got := map[string]bool{}
	for _, p := range sup {
		got[p.Name()] = true
		if !p.SkillsSupported() {
			t.Errorf("%s in Supporting() but SkillsSupported()=false", p.Name())
		}
	}
	for name := range supportingDirs {
		if !got[name] {
			t.Errorf("supporting provider %q missing from Supporting()", name)
		}
	}
	// All ten are registered; only 3 support.
	if len(reg.All()) != 10 {
		t.Fatalf("All() = %d, want 10 registered providers", len(reg.All()))
	}
}

func TestManager_ApplyFansOutToSupportingOnly(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")
	makeCanonical(t, root, "beta")

	m := New(root)
	states, err := m.Apply(root, contract.SkillOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// 3 providers x 2 skills = 6 link states, all linked.
	if len(states) != 6 {
		t.Fatalf("Apply produced %d states, want 6", len(states))
	}
	for _, s := range states {
		if s.State != contract.SkillLinked {
			t.Errorf("%s/%s state=%q, want linked", s.Provider, s.Skill, s.State)
		}
	}

	// Every supporting provider got both symlinks.
	for prov, rel := range supportingDirs {
		for _, skill := range []string{"alpha", "beta"} {
			link := filepath.Join(root, rel, skill)
			assertSymlinkTo(t, link, filepath.Join(root, ".agent", "skills", skill))
		}
		_ = prov
	}

	// NON-supporting providers' skill dirs must NOT exist (never touched).
	for _, nonsup := range []string{".codex", ".cursor", ".github", ".goose", ".grok", ".antigravity", ".roo"} {
		p := filepath.Join(root, nonsup, "skills")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("non-supporting provider dir %s was created/touched", p)
		}
	}
}

func TestManager_ApplyProviderScope(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")

	m := New(root)
	states, err := m.Apply(root, contract.SkillOpts{Provider: "opencode"})
	if err != nil {
		t.Fatalf("Apply scoped: %v", err)
	}
	if len(states) != 1 || states[0].Provider != "opencode" {
		t.Fatalf("scoped Apply = %+v, want one opencode state", states)
	}
	// opencode linked; the other supporting providers untouched.
	assertSymlinkTo(t, filepath.Join(root, ".opencode", "skills", "alpha"),
		filepath.Join(root, ".agent", "skills", "alpha"))
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "alpha")); !os.IsNotExist(err) {
		t.Errorf("claude-code was linked despite provider scope=opencode")
	}
}

func TestManager_ApplyIdempotent(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")
	m := New(root)
	if _, err := m.Apply(root, contract.SkillOpts{}); err != nil {
		t.Fatal(err)
	}
	states, err := m.Apply(root, contract.SkillOpts{})
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	for _, s := range states {
		if s.State != contract.SkillLinked {
			t.Errorf("idempotent Apply %s/%s = %q, want linked", s.Provider, s.Skill, s.State)
		}
	}
}

func TestManager_ApplyConflictAndOverride(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")

	// Pre-place a real (non-symlink) dir at claude-code's link path.
	real := filepath.Join(root, ".claude", "skills", "alpha")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)
	states, err := m.Apply(root, contract.SkillOpts{})
	if err != nil {
		t.Fatal(err)
	}
	claudeState := findState(states, "claude-code", "alpha")
	if claudeState != contract.SkillConflict {
		t.Fatalf("claude-code/alpha = %q, want conflict", claudeState)
	}
	// Other providers still linked.
	if s := findState(states, "opencode", "alpha"); s != contract.SkillLinked {
		t.Errorf("opencode/alpha = %q, want linked", s)
	}

	// With override, the conflict is replaced by a symlink.
	states2, err := m.Apply(root, contract.SkillOpts{Override: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := findState(states2, "claude-code", "alpha"); s != contract.SkillLinked {
		t.Fatalf("override claude-code/alpha = %q, want linked", s)
	}
	assertSymlinkTo(t, real, filepath.Join(root, ".agent", "skills", "alpha"))
}

func TestManager_Status_LiveNoMutation(t *testing.T) {
	root := t.TempDir()
	makeCanonical(t, root, "alpha")
	m := New(root)

	// Before any apply: every supporting provider reports missing.
	st, err := m.Status(root, contract.SkillOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 3 {
		t.Fatalf("Status = %d entries, want 3 (one per supporting provider)", len(st))
	}
	for _, s := range st {
		if s.State != contract.SkillMissing {
			t.Errorf("%s/alpha = %q, want missing", s.Provider, s.State)
		}
	}
	// Status must NOT have created any link.
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "alpha")); !os.IsNotExist(err) {
		t.Errorf("Status mutated the filesystem (created a link)")
	}

	// After apply: all linked.
	if _, err := m.Apply(root, contract.SkillOpts{}); err != nil {
		t.Fatal(err)
	}
	st2, _ := m.Status(root, contract.SkillOpts{})
	for _, s := range st2 {
		if s.State != contract.SkillLinked {
			t.Errorf("post-apply %s/alpha = %q, want linked", s.Provider, s.State)
		}
	}

	// Break one link out of band -> Status reports wrong-link, others linked.
	other := makeCanonical(t, root, "decoy")
	link := filepath.Join(root, ".gemini", "skills", "alpha")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	st3, _ := m.Status(root, contract.SkillOpts{})
	if s := findState(st3, "gemini-cli", "alpha"); s != contract.SkillWrongLink {
		t.Errorf("out-of-band gemini-cli/alpha = %q, want wrong-link", s)
	}
}

func TestManager_InstallCopyInThenLinks(t *testing.T) {
	root := t.TempDir()
	// External skill source dir (not yet canonical).
	src := filepath.Join(t.TempDir(), "writer")
	writeSkillDir(t, src, "# writer\n")

	m := New(root)
	sk, err := m.Install(src, contract.SkillOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if sk.Name != "writer" {
		t.Fatalf("installed = %q, want writer", sk.Name)
	}
	// Canonical copy exists and is symlinked into all 3 supporting providers.
	if !m.Store().Has("writer") {
		t.Fatalf("writer not in canonical store after Install")
	}
	for _, rel := range supportingDirs {
		assertSymlinkTo(t, filepath.Join(root, rel, "writer"),
			filepath.Join(root, ".agent", "skills", "writer"))
	}
}

func TestManager_InstallFromProviderDir(t *testing.T) {
	root := t.TempDir()
	// A skill present only in a provider dir (opencode), not canonical.
	prov := filepath.Join(root, ".opencode", "skills", "fromprov")
	writeSkillDir(t, prov, "# fromprov\n")

	m := New(root)
	// Install by bare name -> should find it in the opencode dir and copy in.
	sk, err := m.Install("fromprov", contract.SkillOpts{Override: true})
	if err != nil {
		t.Fatalf("Install from provider: %v", err)
	}
	if sk.Name != "fromprov" || !m.Store().Has("fromprov") {
		t.Fatalf("install-from-provider failed: %+v", sk)
	}
	// Now canonical + linked back into opencode (override replaced the real dir).
	assertSymlinkTo(t, prov, filepath.Join(root, ".agent", "skills", "fromprov"))
}

func findState(states []contract.SkillStatus, provider, skill string) contract.SkillLinkState {
	for _, s := range states {
		if s.Provider == provider && s.Skill == skill {
			return s.State
		}
	}
	return ""
}
