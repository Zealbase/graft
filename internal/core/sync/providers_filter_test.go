package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// TestSyncProvidersSubset_OnlyEnabledWritten: a sync with opts.Providers set to a
// two-provider subset writes ONLY those providers' files and leaves the other 8
// untouched. result.Changed reflects the agent (claude-code is the source).
func TestSyncProvidersSubset_OnlyEnabledWritten(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	enabled := []string{"claude-code", "opencode"}
	res, err := eng.Run(contract.SyncOpts{Providers: enabled})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "reviewer" {
		t.Fatalf("changed=%v, want [reviewer]", res.Changed)
	}

	// Canonical store is unchanged by the filter (the merge engine still runs).
	if _, err := canonical.Load(canonical.AgentDir(dir, "reviewer")); err != nil {
		t.Fatalf("canonical missing: %v", err)
	}

	tr := transform.Default()
	enabledSet := map[string]bool{"claude-code": true, "opencode": true}

	// Use each provider's own Detect to assert presence/absence of "reviewer".
	// All providers here are project-scoped except antigravity (ScopeHome), whose
	// home base in this test is an empty temp dir (set by newEngine via
	// SetHomeBase) — so detecting at the workspace root correctly finds nothing
	// for it (it is disabled in this run anyway).
	for _, provName := range tr.Providers() {
		prov, _ := tr.Provider(provName)
		refs, derr := prov.Detect(dir)
		if derr != nil {
			t.Fatalf("%s detect: %v", provName, derr)
		}
		has := false
		for _, r := range refs {
			if r.Name == "reviewer" {
				has = true
				break
			}
		}
		if enabledSet[provName] {
			if !has {
				t.Errorf("enabled provider %s produced NO file for reviewer", provName)
			}
		} else if has {
			t.Errorf("disabled provider %s wrote a file for reviewer (should be untouched)", provName)
		}
	}

	// Direct path checks for the two enabled providers' on-disk files.
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "reviewer.md")); err != nil {
		t.Errorf("claude-code file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode", "agents", "reviewer.md")); err != nil {
		t.Errorf("opencode file missing: %v", err)
	}
	// A representative disabled provider must NOT have written anything.
	for _, rel := range []string{
		filepath.Join(".codex", "agents", "reviewer.toml"),
		filepath.Join(".cursor", "agents", "reviewer.md"),
		filepath.Join(".gemini", "agents", "reviewer.md"),
		filepath.Join(".github", "agents"),
		filepath.Join(".grok", "agents", "reviewer.json"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Errorf("disabled provider artifact present at %s (err=%v)", rel, err)
		}
	}
}

// TestSyncProvidersSubset_MetaOnlyEnabled: .meta.json records source hashes only
// for the enabled providers (the merge/canonical layer is unchanged).
func TestSyncProvidersSubset_MetaOnlyEnabled(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if _, err := eng.Run(contract.SyncOpts{Providers: []string{"claude-code", "opencode"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	meta, err := canonical.LoadMeta(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if len(meta.Providers) != 2 {
		t.Fatalf("meta providers = %v, want exactly the 2 enabled", keysOf(meta.Providers))
	}
	for _, want := range []string{"claude-code", "opencode"} {
		if _, ok := meta.Providers[want]; !ok {
			t.Errorf("meta missing enabled provider %q", want)
		}
	}
}

// TestSyncProvidersEmpty_AllProviders: an empty opts.Providers syncs ALL
// providers (default behavior unchanged).
func TestSyncProvidersEmpty_AllProviders(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if _, err := eng.Run(contract.SyncOpts{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	meta, err := canonical.LoadMeta(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	// All project-scoped providers (every provider except the ScopeHome ones,
	// whose home base is an empty temp dir here) should have written + recorded.
	if len(meta.Providers) < 8 {
		t.Fatalf("empty Providers should sync all; meta has only %d (%v)", len(meta.Providers), keysOf(meta.Providers))
	}
	if _, ok := meta.Providers["codex"]; !ok {
		t.Errorf("default sync did not write codex: %v", keysOf(meta.Providers))
	}
}

// TestSubsetSync_DoesNotStaleDisabledProvider is the review-r2 HIGH regression:
// a `sync --providers=claude-code` that advances the canonical must NOT leave an
// out-of-subset provider (opencode) permanently stale. The next FULL sync must
// heal opencode to the new canonical, detected via per-provider CanonicalHash.
//
// Repro: 2 providers, agent desc 'A'; full sync; edit claude file to desc 'B';
// `sync --providers=claude-code` (canonical -> 'B'); full `sync` -> opencode file
// MUST now reflect 'B' (before the fix it stayed 'A' forever, undetected).
func TestSubsetSync_DoesNotStaleDisabledProvider(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	subset := []string{"claude-code", "opencode"}
	opencodeFile := filepath.Join(dir, ".opencode", "agents", "reviewer.md")

	// 1. Full(ish) sync over the two providers, desc 'A'.
	writeClaudeAgent(t, dir, "reviewer", "descA", "You review code.")
	eng1, st1 := newEngine(t, dir)
	if _, err := eng1.Run(contract.SyncOpts{Providers: subset}); err != nil {
		t.Fatalf("sync1: %v", err)
	}
	st1.Close()
	if got := readFileStr(t, opencodeFile); !contains(got, "descA") {
		t.Fatalf("after sync1 opencode should hold descA, got:\n%s", got)
	}

	// 2. Edit the claude file to desc 'B'.
	writeClaudeAgent(t, dir, "reviewer", "descB", "You review code.")

	// 3. Subset sync over ONLY claude-code: advances the canonical to 'B' but does
	// NOT rewrite opencode (out of subset).
	eng2, st2 := newEngine(t, dir)
	if _, err := eng2.Run(contract.SyncOpts{Providers: []string{"claude-code"}}); err != nil {
		t.Fatalf("sync2: %v", err)
	}
	st2.Close()
	can, err := canonical.Load(canonical.AgentDir(dir, "reviewer"))
	if err != nil {
		t.Fatalf("load canonical after sync2: %v", err)
	}
	if can.Description != "descB" {
		t.Fatalf("canonical after subset sync should be descB, got %q", can.Description)
	}
	// opencode is still descA on disk (it was out of the subset) — that is the
	// transient stale state the next full sync must heal.
	if got := readFileStr(t, opencodeFile); !contains(got, "descA") {
		t.Fatalf("after subset sync opencode should still be descA, got:\n%s", got)
	}

	// 4. Full sync over both providers: opencode MUST now be healed to descB.
	eng3, st3 := newEngine(t, dir)
	res, err := eng3.Run(contract.SyncOpts{Providers: subset})
	if err != nil {
		t.Fatalf("sync3: %v", err)
	}
	st3.Close()
	if len(res.Changed) != 1 || res.Changed[0] != "reviewer" {
		t.Fatalf("sync3 changed=%v, want [reviewer] (stale opencode must be re-detected)", res.Changed)
	}
	got := readFileStr(t, opencodeFile)
	if !contains(got, "descB") || contains(got, "descA") {
		t.Fatalf("after full sync opencode MUST be healed to descB, got:\n%s", got)
	}
}

func readFileStr(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func keysOf(m map[string]canonical.ProviderMeta) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
