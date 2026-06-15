package gateway_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// supportingSkillDirs are the per-provider skills dirs for the symlink-based
// skill-supporting providers (claude-code, opencode). codex is supporting too but
// uses native canonical discovery (no symlink dir).
// NOTE(2026-06-15): gemini-cli removed — dewired (kept in code, unregistered).
var supportingSkillDirs = map[string]string{
	"claude-code": ".claude/skills",
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
	canonical := filepath.Join(root, ".agents", "skills", name)
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
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "commit", "SKILL.md")); err != nil {
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
		if s.State != contract.SkillLinked && s.State != contract.SkillNativeLinked {
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
		if s.State != contract.SkillLinked && s.State != contract.SkillNativeLinked {
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
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "seed")

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

// enableSkillHook turns the implicit init/sync skill-apply hook on for a gate.
func enableSkillHook(t *testing.T, g contract.EntryGate) {
	t.Helper()
	hookable, ok := g.(gateway.SkillHookConfigurable)
	if !ok {
		t.Fatal("gate does not implement SkillHookConfigurable")
	}
	hookable.SetSkillHookConfig(gateway.SkillHookConfig{Enabled: true})
}

// hasPair reports whether "provider/skill" is present in the slice.
func hasPair(pairs []string, provider, skill string) bool {
	want := provider + "/" + skill
	for _, p := range pairs {
		if p == want {
			return true
		}
	}
	return false
}

// TestSyncReportsSkillLinkedWhenMissing: a canonical skill missing its provider
// symlink is healed by the sync hook and reported in RunResult.SkillsLinked
// (so the CLI does NOT say "already in sync").
func TestSyncReportsSkillLinkedWhenMissing(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	enableSkillHook(t, g)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Seed a canonical skill AFTER init's hook ran, so its provider links are
	// missing at the start of the upcoming sync.
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "fresh")
	// Remove any links the init hook might have created (defensive — fresh was
	// not present at init time, so there should be none).
	for _, rel := range supportingSkillDirs {
		os.RemoveAll(filepath.Join(root, rel, "fresh"))
	}

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.SkillsLinked) == 0 {
		t.Fatalf("expected SkillsLinked to report the healed skill, got %+v", res)
	}
	if !hasPair(res.SkillsLinked, "claude-code", "fresh") {
		t.Fatalf("claude-code/fresh not in SkillsLinked: %v", res.SkillsLinked)
	}
	if len(res.SkillsConflicted) != 0 {
		t.Fatalf("unexpected conflicts: %v", res.SkillsConflicted)
	}
	assertLinkedAcross(t, root, "fresh")
}

// TestSyncPrunesDeadSkillLinks: a canonical skill that was linked into the
// providers is deleted, leaving dangling provider symlinks. The next sync's
// skills pre-check detects them as dead and prunes them (reported in
// RunResult.SkillsPruned); the provider symlinks are gone afterward.
func TestSyncPrunesDeadSkillLinks(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	enableSkillHook(t, g)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Seed + link a canonical skill via a sync, then delete the canonical dir so
	// the provider symlinks dangle.
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "doomed")
	if _, err := g.Sync(contract.SyncOpts{Ingest: true}); err != nil {
		t.Fatalf("Sync (link): %v", err)
	}
	assertLinkedAcross(t, root, "doomed")
	if err := os.RemoveAll(filepath.Join(root, ".agents", "skills", "doomed")); err != nil {
		t.Fatal(err)
	}

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync (prune): %v", err)
	}
	if !hasPair(res.SkillsPruned, "claude-code", "doomed") {
		t.Fatalf("claude-code/doomed not in SkillsPruned: %v", res.SkillsPruned)
	}
	// Every supporting provider's dangling link is gone.
	for prov, rel := range supportingSkillDirs {
		if _, err := os.Lstat(filepath.Join(root, rel, "doomed")); !os.IsNotExist(err) {
			t.Errorf("%s: dangling link not pruned", prov)
		}
	}
}

// TestSyncReportsSkillConflict: a real (non-symlink) dir occupying the provider
// link path is reported in RunResult.SkillsConflicted (Apply cannot replace it
// without --override), so the run is NOT claimed fully in sync.
func TestSyncReportsSkillConflict(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	enableSkillHook(t, g)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "blocked")
	// Plant a REAL directory at the claude-code link path so the apply hook sees
	// a conflict it cannot resolve without --override.
	conflictPath := filepath.Join(root, ".claude", "skills", "blocked")
	if err := os.MkdirAll(conflictPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflictPath, "REAL.md"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !hasPair(res.SkillsConflicted, "claude-code", "blocked") {
		t.Fatalf("claude-code/blocked not in SkillsConflicted: %v", res.SkillsConflicted)
	}
	// The conflicting real dir is left untouched.
	if _, err := os.Stat(filepath.Join(conflictPath, "REAL.md")); err != nil {
		t.Fatalf("conflict dir should be preserved: %v", err)
	}
}

// TestSyncSkipsSkillClaimsWhenDisabled: with the hook disabled, sync makes no
// skill claims even when canonical skills are missing their links.
func TestSyncSkipsSkillClaimsWhenDisabled(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root) // hook disabled (zero value)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "ignored")

	res, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.SkillsLinked) != 0 || len(res.SkillsConflicted) != 0 {
		t.Fatalf("disabled hook made skill claims: linked=%v conflicted=%v", res.SkillsLinked, res.SkillsConflicted)
	}
}

func TestInitSkillHookSkippedWhenDisabled(t *testing.T) {
	root := newGitWorkspace(t)
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "seed")

	g := openGate(t, root)
	// Default zero-value hook config => disabled; do not enable.
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "seed")); !os.IsNotExist(err) {
		t.Fatalf("hook ran while disabled: %v", err)
	}
}

// TestSyncNativeLinkedSkillCountedOnce verifies that a provider reporting
// SkillNativeLinked (e.g. codex, which uses native canonical discovery instead
// of a symlink dir) is counted as "newly linked" on the FIRST sync but NOT
// re-reported on a second sync (idempotency — must not over-report on every sync).
//
// This is the regression guard for the gateway.applySkillsHookOutcome fix that
// widened the prev-state guard from (prev != SkillLinked) to
// (prev != SkillLinked && prev != SkillNativeLinked).
func TestSyncNativeLinkedSkillCountedOnce(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	enableSkillHook(t, g)
	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Seed a canonical skill.
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "native-skill")

	// First sync: the skill should be reported as newly linked for symlink-based
	// providers (claude-code, opencode). codex uses native canonical discovery
	// (Status() always returns SkillNativeLinked) so it is never in SkillsLinked —
	// the gateway cannot detect a "first sync" vs "already discovered" transition
	// for native-discovery providers.
	res1, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync 1: %v", err)
	}
	if len(res1.SkillsLinked) == 0 {
		t.Fatalf("first sync: expected SkillsLinked to contain native-skill; got %v", res1.SkillsLinked)
	}
	// codex must NEVER appear in SkillsLinked — native-discovery providers have
	// no first-sync detection at this layer (Status() always returns SkillNativeLinked).
	if hasPair(res1.SkillsLinked, "codex", "native-skill") {
		t.Fatal("codex must never appear in SkillsLinked — native providers have no first-sync detection")
	}

	// Second sync (nothing changed): the skill MUST NOT be re-reported as newly
	// linked — its state is already SkillLinked or SkillNativeLinked from the
	// first run. SkillsLinked must be empty.
	res2, err := g.Sync(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("Sync 2: %v", err)
	}
	if len(res2.SkillsLinked) != 0 {
		t.Fatalf("second sync (idempotent): native-linked skill must NOT be re-reported; got SkillsLinked=%v",
			res2.SkillsLinked)
	}
}
