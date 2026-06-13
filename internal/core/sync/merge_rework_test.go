package sync

// Tests for the combined merge-engine rework (plan-sync tasks 1 & 5 +
// v0.0.3 task 2):
//
//  1. canonical-as-source : a direct edit to .graft/agents/<n>/* propagates to
//     EVERY enabled provider on the next sync.
//  2. ingestion           : a provider-only agent (no .graft canonical) is
//     ingested and fanned out to the OTHER enabled providers; gated on Ingest.
//  3. deletion-aware merge : a key removed from one provider STAYS removed, does
//     not resurrect on re-sync, and other providers are untouched. The deletion
//     is scoped to the owning provider's override bucket.
//  4. honest no-op         : a truly-clean re-sync reports an empty Changed set
//     (no silent clear, no spurious work).
//
// All tests drive the REAL engine over a real git workspace + real transform.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// writeClaudeRaw writes a raw claude-code agent file (so a test can include
// arbitrary extra frontmatter keys that land in ProviderOverrides["claude-code"]).
func writeClaudeRaw(t *testing.T, dir, name, frontmatter, body string) {
	t.Helper()
	content := "---\n" + frontmatter + "---\n" + body + "\n"
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"), content)
}

// detectHas reports whether a provider currently detects an agent of the given
// name under the workspace root.
func detectHas(t *testing.T, eng *Engine, provName, agent string) (string, bool) {
	t.Helper()
	prov, ok := eng.tr.Provider(provName)
	if !ok {
		t.Fatalf("provider %q not registered", provName)
	}
	refs, err := prov.Detect(eng.root)
	if err != nil {
		t.Fatalf("%s detect: %v", provName, err)
	}
	for _, r := range refs {
		if r.Name == agent {
			return r.Path, true
		}
	}
	return "", false
}

// readClaude returns the on-disk claude-code agent file content.
func readClaude(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", name+".md"))
	if err != nil {
		t.Fatalf("read claude %s: %v", name, err)
	}
	return string(b)
}

// -----------------------------------------------------------------------------
// 1. canonical-as-source: a direct canonical edit fans out to all providers.
// -----------------------------------------------------------------------------

func TestCanonicalEditPropagatesToAllProviders(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "Original body.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	// Initial sync establishes the canonical + fans out to every provider.
	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	// Hand-edit the CANONICAL agent.yaml + instructions.md directly (the merge
	// surface), NOT any provider file, and NOT the .meta.json sidecar — exactly
	// what a user editing .graft/agents/<n>/* by hand does. The stale .meta.json
	// (old CanonicalHash) is what makes the canonical a drift source.
	aDir := canonical.AgentDir(dir, "reviewer")
	can, err := canonical.Load(aDir)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	can.Description = "REVIEWS CODE CAREFULLY (canonical edit)"
	can.Body = "Brand new canonical body.\n"
	// Render agent.yaml + instructions.md only (drop the regenerated .meta.json).
	writes, err := canonical.Save(dir, can)
	if err != nil {
		t.Fatalf("save canonical: %v", err)
	}
	for _, w := range writes {
		if filepath.Base(w.Path) == ".meta.json" {
			continue // leave the stale sidecar so the canonical reads as drifted
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}

	// Re-sync: canonical drifted from meta.CanonicalHash -> it is a change source.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("re-sync status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "reviewer" {
		t.Fatalf("re-sync Changed=%v, want [reviewer] (canonical edit must be reported)", res.Changed)
	}

	// EVERY enabled (project-scoped) provider's on-disk file must now be the
	// lossless rendering of the EDITED canonical. We compare each provider's
	// re-rendered file against FromCanonical(editedCanonical) — a field-agnostic
	// check that proves the edit propagated regardless of which fields a given
	// provider can express (capability variance is not a failure).
	for _, provName := range eng.tr.Providers() {
		prov, _ := eng.tr.Provider(provName)
		if sp, ok := prov.(contract.ScopedProvider); ok && sp.PathScope() == contract.ScopeHome {
			continue // home-scoped (antigravity) detects under a temp HOME, skip
		}
		path, ok := detectHas(t, eng, provName, "reviewer")
		if !ok {
			t.Errorf("provider %s lost reviewer after canonical edit", provName)
			continue
		}
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s read: %v", provName, err)
			continue
		}
		want, err := eng.tr.FromCanonical(can, provName)
		if err != nil || len(want) == 0 {
			t.Errorf("%s FromCanonical: %v", provName, err)
			continue
		}
		if string(onDisk) != string(want[0].Data) {
			t.Errorf("%s file not re-rendered from edited canonical (canonical-as-source did not propagate)\n--- on disk ---\n%s\n--- want ---\n%s",
				provName, onDisk, want[0].Data)
		}
	}

	// meta.CanonicalHash refreshed to the edited canonical (loop closed).
	meta2, _ := canonical.LoadMeta(aDir)
	if meta2.CanonicalHash != canonical.Hash(can) {
		t.Errorf("meta.CanonicalHash=%q not refreshed to edited canonical %q", meta2.CanonicalHash, canonical.Hash(can))
	}
}

// -----------------------------------------------------------------------------
// 2. ingestion: a provider-only agent is ingested + fanned out; Ingest gate.
// -----------------------------------------------------------------------------

func TestProviderOnlyAgentIngestsAndFansOut(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	// An agent that exists ONLY in the claude provider dir, with NO .graft canonical.
	writeClaudeAgent(t, dir, "scout", "scouts the repo", "You scout.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{Ingest: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "scout" {
		t.Fatalf("Changed=%v, want [scout] (ingested agent must be reported)", res.Changed)
	}

	// Canonical was created from the provider-only file.
	if _, err := canonical.Load(canonical.AgentDir(dir, "scout")); err != nil {
		t.Fatalf("canonical not ingested: %v", err)
	}

	// Fanned out to OTHER providers (e.g. opencode, codex), not just claude.
	if _, ok := detectHas(t, eng, "opencode", "scout"); !ok {
		t.Errorf("ingested agent not fanned out to opencode")
	}
	if _, ok := detectHas(t, eng, "codex", "scout"); !ok {
		t.Errorf("ingested agent not fanned out to codex")
	}
}

func TestIngestDisabledSkipsProviderOnlyAgent(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "scout", "scouts the repo", "You scout.")

	eng, st := newEngine(t, dir)
	defer st.Close()
	eng.SetIngest(false) // explicit --no-ingest

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done", res.Status)
	}
	if len(res.Changed) != 0 {
		t.Fatalf("Changed=%v, want empty (ingestion disabled)", res.Changed)
	}
	// No canonical created, no fan-out.
	if _, err := os.Stat(canonical.AgentDir(dir, "scout")); !os.IsNotExist(err) {
		t.Errorf("ingestion disabled but canonical was created: %v", err)
	}
	if _, ok := detectHas(t, eng, "opencode", "scout"); ok {
		t.Errorf("ingestion disabled but agent fanned out to opencode")
	}
}

// -----------------------------------------------------------------------------
// 3. deletion-aware merge (THE BUG): a key removed from one provider's override
//    bucket stays removed, does not resurrect on re-sync, and other providers
//    are untouched.
// -----------------------------------------------------------------------------

func TestProviderKeyRemovalStaysRemovedAndScoped(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// claude file carries an EXTRA frontmatter key `temperature` (no canonical
	// home) -> lands in ProviderOverrides["claude-code"]. opencode also defines
	// the agent (no temperature). Same body/desc/model so nothing else conflicts.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: a dev\nmodel: sonnet\ntemperature: 0.7\n",
		"Shared body.")
	writeOpencodeAgent(t, dir, "dev", "a dev", "sonnet", "Shared body.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	// Sanity: temperature was captured in claude-code's override bucket.
	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if b := can.ProviderOverrides["claude-code"]; b == nil || b["temperature"] == nil {
		t.Fatalf("setup: temperature not captured in claude-code override bucket: %+v", can.ProviderOverrides)
	}

	// Now REMOVE temperature from the claude file and re-sync.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: a dev\nmodel: sonnet\n",
		"Shared body.")

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("re-sync after removal: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("re-sync status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// (a) canonical no longer carries claude-code's temperature key.
	can2, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("reload canonical: %v", err)
	}
	if b := can2.ProviderOverrides["claude-code"]; b != nil && b["temperature"] != nil {
		t.Errorf("temperature RESURRECTED in canonical bucket after removal: %+v", b)
	}

	// (b) the claude provider file no longer contains temperature.
	if c := readClaude(t, dir, "dev"); strings.Contains(c, "temperature") {
		t.Errorf("claude file still has temperature after removal:\n%s", c)
	}

	// (c) opencode (the OTHER provider) is untouched / still present.
	if _, ok := detectHas(t, eng, "opencode", "dev"); !ok {
		t.Errorf("opencode agent disappeared (removal not scoped to owning provider)")
	}

	// (d) re-sync once more: removal must STAY gone (no resurrection loop).
	res3, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("third sync: %v", err)
	}
	if res3.Status != contract.RunDone {
		t.Fatalf("third sync status=%s, want done", res3.Status)
	}
	can3, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("reload canonical 3: %v", err)
	}
	if b := can3.ProviderOverrides["claude-code"]; b != nil && b["temperature"] != nil {
		t.Errorf("temperature resurrected on the SECOND re-sync: %+v", b)
	}
	if strings.Contains(readClaude(t, dir, "dev"), "temperature") {
		t.Errorf("temperature reappeared in claude file on second re-sync")
	}
}

// TestRemovalScopedToOwningBucketOnly verifies the deletion-aware guard: a
// provider NOT synced this run keeps its prior override bucket verbatim. We seed
// two providers' buckets, then re-sync only claude after clearing its key; the
// opencode bucket (not the target of the deletion) must be preserved through the
// merge.
func TestRemovalScopedToOwningBucketOnly(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// Both providers carry their own extra (bucket) key.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: a dev\nmodel: sonnet\nclaudeExtra: keep-me\n",
		"Shared body.")
	writeFile(t, dir, filepath.Join(".opencode", "agents", "dev.md"),
		"---\nname: dev\ndescription: a dev\nmodel: sonnet\nopencodeExtra: also-keep\n---\nShared body.\n")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	can, _ := canonical.Load(canonical.AgentDir(dir, "dev"))
	if can.ProviderOverrides["claude-code"]["claudeExtra"] == nil {
		t.Fatalf("setup: claudeExtra not in claude bucket: %+v", can.ProviderOverrides)
	}
	if can.ProviderOverrides["opencode"]["opencodeExtra"] == nil {
		t.Fatalf("setup: opencodeExtra not in opencode bucket: %+v", can.ProviderOverrides)
	}

	// Remove claudeExtra from the claude file ONLY; leave opencode untouched.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: a dev\nmodel: sonnet\n",
		"Shared body.")

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("re-sync: res=%+v err=%v", res, err)
	}

	can2, _ := canonical.Load(canonical.AgentDir(dir, "dev"))
	// claude's key gone...
	if b := can2.ProviderOverrides["claude-code"]; b != nil && b["claudeExtra"] != nil {
		t.Errorf("claudeExtra not removed: %+v", b)
	}
	// ...but opencode's bucket key PRESERVED (scoped to owning provider).
	if can2.ProviderOverrides["opencode"]["opencodeExtra"] == nil {
		t.Errorf("opencodeExtra dropped — deletion leaked across provider buckets: %+v", can2.ProviderOverrides)
	}
}

// -----------------------------------------------------------------------------
// 4. honest no-op: a clean re-sync reports an empty Changed (no silent clear,
//    no spurious work).
// -----------------------------------------------------------------------------

func TestCleanResyncReportsNoChanges(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	// Nothing edited since: a re-sync must report DONE with an EMPTY Changed set
	// (CLI prints "already in sync"). This is the honest no-op, not a silent clear.
	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("clean re-sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("clean re-sync status=%s, want done", res.Status)
	}
	if len(res.Changed) != 0 {
		t.Fatalf("clean re-sync Changed=%v, want empty (nothing drifted)", res.Changed)
	}
}
