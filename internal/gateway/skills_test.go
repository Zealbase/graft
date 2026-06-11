package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// supportingSkillDirs are the per-provider skills dirs for the 3 skill-supporting
// providers (claude-code, gemini-cli, opencode).
var supportingSkillDirs = map[string]string{
	"claude-code": ".claude/skills",
	"gemini-cli":  ".gemini/skills",
	"opencode":    ".opencode/skills",
}

// writeSkill creates a skill source dir with a SKILL.md (+ one asset) and returns
// its absolute path.
func writeSkill(t *testing.T, dir, name string) string {
	t.Helper()
	sk := filepath.Join(dir, name)
	if err := os.MkdirAll(sk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("# "+name+"\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sk, "helper.txt"), []byte("asset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return sk
}

// assertLinkedAcross verifies the skill is symlinked into all 3 supporting
// provider dirs and resolves to the canonical SKILL.md.
func assertLinkedAcross(t *testing.T, root, name string) {
	t.Helper()
	canonical := filepath.Join(root, ".agent", "skills", name)
	for prov, rel := range supportingSkillDirs {
		link := filepath.Join(root, rel, name)
		fi, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("%s: no entry at %s: %v", prov, link, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s: %s is not a symlink", prov, link)
		}
		// Resolves to canonical SKILL.md.
		got, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
		if err != nil {
			t.Fatalf("%s: read through link: %v", prov, err)
		}
		want, _ := os.ReadFile(filepath.Join(canonical, "SKILL.md"))
		if string(got) != string(want) {
			t.Fatalf("%s: link content mismatch", prov)
		}
	}
}

func TestSkillInstallFansOutToThreeProviders(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	src := writeSkill(t, t.TempDir(), "commit")
	states, err := g.SkillInstall(src, contract.SkillOpts{})
	if err != nil {
		t.Fatalf("SkillInstall: %v", err)
	}

	// Canonical copy exists.
	if _, err := os.Stat(filepath.Join(root, ".agent", "skills", "commit", "SKILL.md")); err != nil {
		t.Fatalf("canonical skill not copied in: %v", err)
	}
	// Linked across all 3 supporting providers.
	assertLinkedAcross(t, root, "commit")

	// Returned states cover the 3 providers, all linked.
	linked := map[string]bool{}
	for _, s := range states {
		if s.Skill == "commit" && s.State == contract.SkillLinked {
			linked[s.Provider] = true
		}
	}
	for prov := range supportingSkillDirs {
		if !linked[prov] {
			t.Fatalf("provider %s not reported linked: %+v", prov, states)
		}
	}
}

func TestSkillListAndStatus(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	src := writeSkill(t, t.TempDir(), "review")
	if _, err := g.SkillInstall(src, contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall: %v", err)
	}

	skills, err := g.SkillList()
	if err != nil {
		t.Fatalf("SkillList: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "review" {
		t.Fatalf("SkillList = %+v, want [review]", skills)
	}

	states, err := g.SkillStatus(contract.SkillOpts{})
	if err != nil {
		t.Fatalf("SkillStatus: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("SkillStatus returned %d states, want 3 (one per supporting provider): %+v", len(states), states)
	}
	for _, s := range states {
		if s.State != contract.SkillLinked {
			t.Fatalf("provider %s state=%s, want linked", s.Provider, s.State)
		}
	}
}

func TestSkillSyncIdempotent(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	src := writeSkill(t, t.TempDir(), "deploy")
	if _, err := g.SkillInstall(src, contract.SkillOpts{}); err != nil {
		t.Fatalf("SkillInstall: %v", err)
	}

	first, err := g.SkillSync(contract.SkillOpts{})
	if err != nil {
		t.Fatalf("SkillSync 1: %v", err)
	}
	second, err := g.SkillSync(contract.SkillOpts{})
	if err != nil {
		t.Fatalf("SkillSync 2: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("SkillSync not stable: %d vs %d states", len(first), len(second))
	}
	for _, s := range second {
		if s.State != contract.SkillLinked {
			t.Fatalf("idempotent sync left %s/%s in state %s", s.Skill, s.Provider, s.State)
		}
	}
	assertLinkedAcross(t, root, "deploy")
}

func TestSkillSyncProviderScope(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	src := writeSkill(t, t.TempDir(), "lint")
	if _, err := g.SkillInstall(src, contract.SkillOpts{Provider: "claude-code"}); err != nil {
		t.Fatalf("SkillInstall scoped: %v", err)
	}
	// Only claude-code linked; the other two have no entry.
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "lint")); err != nil {
		t.Fatalf("claude-code link missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".gemini", "skills", "lint")); !os.IsNotExist(err) {
		t.Fatalf("gemini-cli should NOT be linked under provider scope, err=%v", err)
	}
}

// TestInitSkillHookGated verifies the init hook links pre-existing canonical
// skills only when the hook config is enabled.
func TestInitSkillHookLinksWhenEnabled(t *testing.T) {
	root := newGitWorkspace(t)
	// Seed a canonical skill BEFORE init so the init hook links it.
	writeSkill(t, filepath.Join(root, ".agent", "skills"), "seed")

	g := openGate(t, root)
	hookable, ok := g.(gateway.SkillHookConfigurable)
	if !ok {
		t.Fatal("gate does not implement SkillHookConfigurable")
	}
	hookable.SetSkillHookConfig(gateway.SkillHookConfig{Enabled: true})

	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	assertLinkedAcross(t, root, "seed")
}

func TestInitSkillHookSkippedWhenDisabled(t *testing.T) {
	root := newGitWorkspace(t)
	writeSkill(t, filepath.Join(root, ".agent", "skills"), "seed")

	g := openGate(t, root)
	// Default zero-value hook config => disabled; do not enable.
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "seed")); !os.IsNotExist(err) {
		t.Fatalf("hook ran while disabled: %v", err)
	}
}
