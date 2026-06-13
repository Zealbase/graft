package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestStore_MigrateLegacy moves skills from the legacy .agent/skills store into
// the canonical .agents/skills store, never clobbering an existing canonical
// skill, and removes the drained legacy dir.
func TestStore_MigrateLegacy(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)

	// Legacy store has two skills; canonical already has one of them with
	// distinct content that must NOT be overwritten.
	legacy := s.LegacyDir()
	writeSkillDir(t, filepath.Join(legacy, "moveme"), "legacy-moveme")
	writeSkillDir(t, filepath.Join(legacy, "keepcanon"), "legacy-keepcanon")
	writeSkillDir(t, s.SkillDir("keepcanon"), "canonical-keepcanon")

	migrated, err := s.MigrateLegacy()
	if err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}
	if len(migrated) != 1 || migrated[0] != "moveme" {
		t.Fatalf("migrated = %v, want [moveme]", migrated)
	}

	// moveme is now canonical.
	if !s.Has("moveme") {
		t.Fatalf("moveme not in canonical store after migrate")
	}
	// keepcanon canonical content is untouched (not clobbered by legacy).
	got, _ := os.ReadFile(filepath.Join(s.SkillDir("keepcanon"), "SKILL.md"))
	if string(got) != "canonical-keepcanon" {
		t.Fatalf("keepcanon clobbered: %q", got)
	}
	// Legacy moveme dir is gone (renamed away); keepcanon legacy may remain but
	// the legacy root is only removed when fully drained -- here keepcanon stays.
	if _, err := os.Stat(filepath.Join(legacy, "moveme")); !os.IsNotExist(err) {
		t.Fatalf("legacy moveme still present after migrate")
	}

	// Idempotent: a second run is a no-op (moveme already canonical, keepcanon
	// skipped) and migrates nothing new.
	again, err := s.MigrateLegacy()
	if err != nil {
		t.Fatalf("MigrateLegacy (2): %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second migrate moved %v, want none", again)
	}
}

// TestStore_MigrateLegacyRemovesEmptyDir verifies that a legacy store fully
// drained of skills has its .agent/skills (and .agent) dirs removed.
func TestStore_MigrateLegacyRemovesEmptyDir(t *testing.T) {
	root := t.TempDir()
	s := NewStore(root)
	writeSkillDir(t, filepath.Join(s.LegacyDir(), "only"), "x")
	if _, err := s.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, legacyAgentDir)); !os.IsNotExist(err) {
		t.Fatalf("empty legacy .agent dir not removed")
	}
	if !s.Has("only") {
		t.Fatalf("only not migrated to canonical")
	}
}

// TestStore_MigrateLegacyMissing is a no-op when no legacy dir exists.
func TestStore_MigrateLegacyMissing(t *testing.T) {
	s := NewStore(t.TempDir())
	migrated, err := s.MigrateLegacy()
	if err != nil || migrated != nil {
		t.Fatalf("MigrateLegacy on missing legacy = (%v, %v), want (nil, nil)", migrated, err)
	}
}

// TestDetect_HomeScopeCandidates surfaces a personal (home-scope) skill as a
// provider-only install candidate, and never as a link target.
func TestDetect_HomeScopeCandidates(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	// A personal claude-code skill under ~/.claude/skills.
	writeSkillDir(t, filepath.Join(home, ".claude", "skills", "personal"), "p")

	reg := Default()
	store := NewStore(root)
	got, err := Detect(reg, store, root, home)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	var found *DetectedSkill
	for i := range got {
		if got[i].Name == "personal" {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("home-scope skill not detected; got %+v", got)
	}
	if !found.InstallCandidate() {
		t.Fatalf("home-scope skill should be a provider-only install candidate, got origin %q", found.Origin)
	}
	if len(found.Sources) == 0 || found.Sources[0].Provider != "claude-code" {
		t.Fatalf("home-scope source not recorded correctly: %+v", found.Sources)
	}
}

// TestDetect_HomeScopeDisabled: empty home skips home-scope discovery entirely.
func TestDetect_HomeScopeDisabled(t *testing.T) {
	root := t.TempDir()
	got, err := Detect(Default(), NewStore(root), root, "")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("home disabled but detected %+v", got)
	}
}

// TestManager_InstallFromHomeScope installs a personal skill by bare name (found
// only in ~/.claude/skills), copying it into the canonical store and linking it
// into the supporting providers' project dirs.
func TestManager_InstallFromHomeScope(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeSkillDir(t, filepath.Join(home, ".claude", "skills", "homed"), "h")

	m := New(root)
	if _, err := m.Install("homed", contract.SkillOpts{}); err != nil {
		t.Fatalf("Install from home scope: %v", err)
	}
	// Canonical copy exists.
	if !m.Store().Has("homed") {
		t.Fatalf("homed not copied into canonical store")
	}
	// Linked into claude-code project dir as a symlink (not a real dir).
	link := filepath.Join(root, ".claude", "skills", "homed")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat claude link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("claude-code homed is not a symlink")
	}
	// The home source must remain a real (untouched) dir, never symlinked.
	hi, err := os.Lstat(filepath.Join(home, ".claude", "skills", "homed"))
	if err != nil {
		t.Fatalf("lstat home source: %v", err)
	}
	if hi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("home source was turned into a symlink (must stay real)")
	}
}

// TestProviderHomeSkillDirs spot-checks the documented home dirs per provider.
func TestProviderHomeSkillDirs(t *testing.T) {
	home := "/h"
	cases := map[string][]string{
		"claude-code": {filepath.Join(home, ".claude", "skills")},
		"gemini-cli":  {filepath.Join(home, ".gemini", "skills"), filepath.Join(home, ".agents", "skills")},
		"opencode": {
			filepath.Join(home, ".config", "opencode", "skills"),
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".agents", "skills"),
		},
	}
	reg := Default()
	for _, p := range reg.Supporting() {
		want := cases[p.Name()]
		got := p.HomeSkillDirs(home)
		if len(got) != len(want) {
			t.Fatalf("%s HomeSkillDirs = %v, want %v", p.Name(), got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s HomeSkillDirs[%d] = %q, want %q", p.Name(), i, got[i], want[i])
			}
		}
		// Empty home yields nil.
		if p.HomeSkillDirs("") != nil {
			t.Errorf("%s HomeSkillDirs(\"\") not nil", p.Name())
		}
	}
}
