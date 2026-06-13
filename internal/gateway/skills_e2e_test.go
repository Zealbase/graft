package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// nonSupportingSkillDirs are the project skill-dir conventions that the 7
// non-supporting providers do NOT use. The skills registry must never create an
// entry under any of these — they are listed so the matrix can assert absence.
var nonSupportingSkillDirs = map[string]string{
	"codex":          ".codex/skills",
	"cursor":         ".cursor/skills",
	"github-copilot": ".github/skills",
	"roo-code":       ".roo/skills",
	"goose":          ".goose/skills",
	"grok-cli":       ".grok/skills",
	"antigravity":    ".antigravity/skills",
}

// homeSkillDirRel maps each supporting provider to one of its home-scope skill
// dirs (relative to HOME) for the home-scope detection matrix.
var homeSkillDirRel = map[string]string{
	"claude-code": ".claude/skills",
	"gemini-cli":  ".gemini/skills",
	"opencode":    ".config/opencode/skills",
}

// assertNoEntry fails if any non-supporting provider grew a skill entry.
func assertNoNonSupportingEntries(t *testing.T, root, name string) {
	t.Helper()
	for prov, rel := range nonSupportingSkillDirs {
		if _, err := os.Lstat(filepath.Join(root, rel, name)); !os.IsNotExist(err) {
			t.Fatalf("non-supporting provider %s grew an entry for %q (err=%v)", prov, name, err)
		}
	}
}

// E2E: install fans out to the 3 supporting providers AND leaves all 7
// non-supporting providers untouched (clean skip, no error).
func TestE2E_NonSupportingProvidersSkip(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	src := writeSkill(t, t.TempDir(), "skipcheck")
	if _, err := g.SkillInstall(src, contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall: %v", err)
	}
	assertLinkedAcross(t, root, "skipcheck")
	assertNoNonSupportingEntries(t, root, "skipcheck")
}

// E2E: a skill present only in a provider's PROJECT dir (not canonical) is an
// install candidate; installing by bare name copies it canonical and links out.
func TestE2E_InstallFromFoundProjectPath(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Seed a real skill dir under opencode's project skills dir only.
	writeSkill(t, filepath.Join(root, ".opencode", "skills"), "found")
	if _, err := g.SkillInstall("found", contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall by name from project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "found", "SKILL.md")); err != nil {
		t.Fatalf("found skill not copied into canonical: %v", err)
	}
	// claude-code + gemini-cli get fresh symlinks.
	for _, rel := range []string{".claude/skills", ".gemini/skills"} {
		fi, err := os.Lstat(filepath.Join(root, rel, "found"))
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s/found not a symlink (err=%v)", rel, err)
		}
	}
}

// E2E: a personal skill in a HOME-scope dir is detected and installable by name;
// install copies it canonical and symlinks into the project providers while the
// home source stays a real (untouched) dir.
func TestE2E_HomeScopeDetectAndInstall(t *testing.T) {
	root := newGitWorkspace(t) // isolates HOME to a temp dir
	home, _ := os.UserHomeDir()
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "personal")

	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := g.SkillInstall("personal", contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall from home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "personal", "SKILL.md")); err != nil {
		t.Fatalf("home skill not copied into canonical: %v", err)
	}
	assertLinkedAcross(t, root, "personal")
	// Home source remains a real dir, never converted to a symlink.
	hi, err := os.Lstat(filepath.Join(home, ".claude", "skills", "personal"))
	if err != nil || hi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("home source changed/converted (err=%v)", err)
	}
}

// E2E: link is created ONLY when absent; a correct existing symlink is a no-op
// (idempotent re-apply), and a stale/broken symlink is repaired.
func TestE2E_SymlinkAbsentRepairAndIdempotent(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	src := writeSkill(t, t.TempDir(), "repair")
	if _, err := g.SkillInstall(src, contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall: %v", err)
	}
	assertLinkedAcross(t, root, "repair")

	// Corrupt one link: point claude-code's entry at a bogus target (stale link).
	claudeLink := filepath.Join(root, ".claude", "skills", "repair")
	if err := os.Remove(claudeLink); err != nil {
		t.Fatalf("rm link: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "nonexistent"), claudeLink); err != nil {
		t.Fatalf("make stale link: %v", err)
	}
	// Re-sync repairs the stale/broken link back to canonical.
	if _, err := g.SkillSync(contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillSync repair: %v", err)
	}
	assertLinkedAcross(t, root, "repair")

	// Out-of-band deletion: remove the gemini link entirely, re-sync re-links it.
	if err := os.Remove(filepath.Join(root, ".gemini", "skills", "repair")); err != nil {
		t.Fatalf("rm gemini link: %v", err)
	}
	if _, err := g.SkillSync(contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillSync relink: %v", err)
	}
	assertLinkedAcross(t, root, "repair")
}

// E2E: a REAL (non-symlink) dir blocking the link path is a conflict; --override
// replaces it with a symlink; without override it is reported as conflict and the
// real dir is preserved.
func TestE2E_ConflictAndOverride(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "clash")

	// Pre-place a REAL dir at claude-code's link path (a user-authored skill).
	real := filepath.Join(root, ".claude", "skills", "clash")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("USER OWNED\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without override: claude-code reports conflict; the real dir is preserved.
	states, err := g.SkillSync(contract.SkillOpts{})
	if err != nil {
		t.Fatalf("SkillSync: %v", err)
	}
	var sawConflict bool
	for _, s := range states {
		if s.Provider == "claude-code" && s.Skill == "clash" {
			if s.State != contract.SkillConflict {
				t.Fatalf("claude-code clash state=%s, want conflict", s.State)
			}
			sawConflict = true
		}
	}
	if !sawConflict {
		t.Fatalf("no conflict reported for the real dir: %+v", states)
	}
	fi, _ := os.Lstat(real)
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("real dir was clobbered without override")
	}
	body, _ := os.ReadFile(filepath.Join(real, "SKILL.md"))
	if string(body) != "USER OWNED\n" {
		t.Fatalf("user content lost without override: %q", body)
	}

	// With override: the real dir is replaced by a symlink to canonical.
	if _, err := g.SkillSync(contract.SkillOpts{Override: true}); err != nil {
		t.Fatalf("SkillSync override: %v", err)
	}
	fi, _ = os.Lstat(real)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("override did not replace real dir with symlink")
	}
}

// E2E: the init/sync hook can be restricted to a SUBSET of supporting providers
// (skills.providers config) — only those get links.
func TestE2E_EnabledProviderSubsetHook(t *testing.T) {
	root := newGitWorkspace(t)
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "subset")

	g := openGate(t, root)
	hookable, ok := g.(gateway.SkillHookConfigurable)
	if !ok {
		t.Fatal("gate does not implement SkillHookConfigurable")
	}
	hookable.SetSkillHookConfig(gateway.SkillHookConfig{
		Enabled:   true,
		Providers: []string{"opencode"},
	})
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// opencode linked; claude-code + gemini-cli NOT.
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills", "subset")); err != nil {
		t.Fatalf("opencode not linked under subset hook: %v", err)
	}
	for _, rel := range []string{".claude/skills", ".gemini/skills"} {
		if _, err := os.Lstat(filepath.Join(root, rel, "subset")); !os.IsNotExist(err) {
			t.Fatalf("%s linked despite subset restriction", rel)
		}
	}
}

// E2E: a legacy .agent/skills store is migrated to canonical .agents/skills on
// the first skills operation, then fanned out to supporting providers.
func TestE2E_LegacyStoreMigration(t *testing.T) {
	root := newGitWorkspace(t)
	// Seed the LEGACY singular store before any skills op.
	writeSkill(t, filepath.Join(root, ".agent", "skills"), "legacyskill")

	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// List triggers migration; canonical now holds the skill, legacy is drained.
	skills, err := g.SkillList()
	if err != nil {
		t.Fatalf("SkillList: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "legacyskill" {
		t.Fatalf("SkillList after migrate = %+v, want [legacyskill]", skills)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "legacyskill", "SKILL.md")); err != nil {
		t.Fatalf("legacy skill not migrated to canonical: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "skills", "legacyskill")); !os.IsNotExist(err) {
		t.Fatalf("legacy skill dir not drained after migrate")
	}
	// A sync now links the migrated skill across supporting providers.
	if _, err := g.SkillSync(contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillSync: %v", err)
	}
	assertLinkedAcross(t, root, "legacyskill")
}

// E2E: canonical .agents/skills seeded directly is the source of truth and links
// out on sync (vendor-neutral location matrix).
func TestE2E_CanonicalAgentsLocation(t *testing.T) {
	root := newGitWorkspace(t)
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "vendorneutral")
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := g.SkillSync(contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillSync: %v", err)
	}
	assertLinkedAcross(t, root, "vendorneutral")
	assertNoNonSupportingEntries(t, root, "vendorneutral")
}
