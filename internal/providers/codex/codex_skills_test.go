package codex

import (
	"testing"
)

// TestSkillsSupported verifies that codex now declares skills support and
// native canonical discovery (A-D1: codex auto-scans .agents/skills/).
func TestSkillsSupported(t *testing.T) {
	sp := SkillProvider()

	if !sp.SkillsSupported() {
		t.Fatal("codex must support skills (SkillsSupported() == true)")
	}
	if sp.Name() != name {
		t.Errorf("Name()=%q want %q", sp.Name(), name)
	}
}

// TestNativeCanonicalDiscovery verifies that codex signals native canonical
// discovery, meaning the skills manager must skip the symlink step and report
// SkillNativeLinked rather than creating a symlink.
func TestNativeCanonicalDiscovery(t *testing.T) {
	sp := SkillProvider()
	if !sp.NativeCanonicalDiscovery() {
		t.Fatal("codex must return NativeCanonicalDiscovery() == true")
	}
}

// TestSkillDir_Empty verifies that codex SkillDir returns "" — there is no
// separate provider-scoped skills directory; codex reads the canonical store
// directly, so no symlink target directory should ever be created.
func TestSkillDir_Empty(t *testing.T) {
	sp := SkillProvider()
	if d := sp.SkillDir("/ws"); d != "" {
		t.Errorf("SkillDir=%q want empty (native discovery, no symlink dir)", d)
	}
}

// TestDetectSkills_NoProjectRefs verifies that DetectSkills returns no project-
// scope refs (the canonical store is handled by the manager, not scanned here).
func TestDetectSkills_NoProjectRefs(t *testing.T) {
	sp := SkillProvider()
	refs, err := sp.DetectSkills("/ws")
	if err != nil {
		t.Fatalf("DetectSkills err: %v", err)
	}
	if refs != nil {
		t.Errorf("expected nil refs for native-discovery provider, got %+v", refs)
	}
}

// TestHomeSkillDirs verifies that codex reports both its native home skill dir
// (~/.codex/skills) and the vendor-neutral home dir (~/.agents/skills) as
// home-scope candidates.
func TestHomeSkillDirs(t *testing.T) {
	sp := SkillProvider()
	home := "/home/user"
	dirs := sp.HomeSkillDirs(home)
	if len(dirs) != 2 {
		t.Fatalf("HomeSkillDirs=%v, want 2 dirs", dirs)
	}
	if dirs[0] != codexNativeSkillDir(home) {
		t.Errorf("HomeSkillDirs[0]=%q, want %q", dirs[0], codexNativeSkillDir(home))
	}
	if dirs[1] != canonicalHomeSkillDir(home) {
		t.Errorf("HomeSkillDirs[1]=%q, want %q", dirs[1], canonicalHomeSkillDir(home))
	}
	// Empty home yields nil.
	if sp.HomeSkillDirs("") != nil {
		t.Errorf("HomeSkillDirs(\"\") not nil, want nil")
	}
}
