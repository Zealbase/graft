package codex

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
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

// TestParseSkillsConfig_EnabledPathEntry verifies FIX 2/FIX 4: a path-based
// ENABLED [[skills.config]] entry (path points at the SKILL.md inside the skill
// dir, e.g. /Users/me/.agents/skills/docs-editor/SKILL.md) derives the skill name
// from the SKILL.md's parent-dir basename (docs-editor) into the enabled list,
// and is NOT also stashed in the disabled bucket (which would double-emit on
// Serialize).
func TestParseSkillsConfig_EnabledPathEntry(t *testing.T) {
	raw := map[string]any{
		"config": []map[string]any{
			{"path": "/Users/me/.agents/skills/docs-editor/SKILL.md", "enabled": true},
		},
	}
	enabled, disabled := parseSkillsConfig(raw)
	if len(enabled) != 1 || enabled[0] != "docs-editor" {
		t.Fatalf("enabled=%v, want [docs-editor] (basename of SKILL.md parent dir)", enabled)
	}
	if len(disabled) != 0 {
		t.Fatalf("disabled=%v, want empty — an enabled path entry must NOT be stashed (double-emit)", disabled)
	}
}

// TestSerialize_NoDoubleEmitForEnabledPathSkill is the end-to-end guard for the
// double-emit: a TOML with an enabled path-based skill must round-trip
// (ToCanonical -> Serialize) to a SINGLE enabled=true entry for that skill, never
// two entries (one enabled, one disabled).
func TestSerialize_NoDoubleEmitForEnabledPathSkill(t *testing.T) {
	p := contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: "writer", Provider: name},
		Fields: map[string]any{
			"name": "writer",
			"skills": map[string]any{
				"config": []map[string]any{
					{"path": "/Users/me/.agents/skills/docs-editor/SKILL.md", "enabled": true},
				},
			},
		},
	}
	ca, err := Provider{}.ToCanonical(p)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if len(ca.Skills) != 1 || ca.Skills[0] != "docs-editor" {
		t.Fatalf("ca.Skills=%v, want [docs-editor]", ca.Skills)
	}
	writes, err := Provider{}.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	out := string(writes[0].Data)
	if n := strings.Count(out, "[[skills.config]]"); n != 1 {
		t.Fatalf("expected exactly 1 [[skills.config]] entry, got %d:\n%s", n, out)
	}
	if strings.Contains(out, "enabled = false") {
		t.Fatalf("enabled path skill must not emit a disabled entry:\n%s", out)
	}
}

// TestSerialize_StashContradictionSkipped is the guard for FIX 3: a skill present
// in ca.Skills (effective enabled list) that ALSO has a stale enabled=false stash
// for the SAME name must emit only the enabled=true entry — the effective list
// wins, the contradicting stash entry is skipped.
func TestSerialize_StashContradictionSkipped(t *testing.T) {
	ca := contract.CanonicalAgent{
		Name:   "writer",
		Skills: []string{"docs-editor"},
		ProviderOverrides: map[string]map[string]any{
			name: {
				codexSkillsDisabledKey: []map[string]any{
					{"name": "docs-editor", "enabled": false},
				},
			},
		},
	}
	writes, err := Provider{}.Serialize(ca)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	out := string(writes[0].Data)
	if n := strings.Count(out, "[[skills.config]]"); n != 1 {
		t.Fatalf("expected exactly 1 [[skills.config]] entry (contradiction skipped), got %d:\n%s", n, out)
	}
	if strings.Contains(out, "enabled = false") {
		t.Fatalf("effective skill must win; the enabled=false stash must be skipped:\n%s", out)
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
