package sync

// Regression test for the canonical-field override RESURRECTION data-loss bug
// (v0.0.4 conformance r1 HIGH 2).
//
// Scenario: a CANONICAL field (description) has a shared value "real", and one
// provider (claude-code) carries a per-provider OVERRIDE of that field
// ("override"). RestoreOverrides writes "override" into the .claude file, so the
// override reappears on the next parse as a PLAIN canonical field (not inside the
// extras bucket). The previous fold logic promoted that parsed value into the
// SHARED canonical, overwriting "real" for every provider — silent data loss.
//
// The fix uses the PRIOR override bucket as the discriminator: if the ancestor
// already recorded "description" as an override for claude-code, the parsed
// description is that provider's override (kept in its bucket) and never folds
// into the shared canonical.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// saveCanonicalNoMeta renders agent.yaml + instructions.md for the given
// canonical but intentionally drops the regenerated .meta.json so the stale
// sidecar makes the canonical read as drifted on the next sync (same trick the
// canonical-as-source test uses).
func saveCanonicalNoMeta(t *testing.T, dir string, can contract.CanonicalAgent) {
	t.Helper()
	writes, err := canonical.Save(dir, can)
	if err != nil {
		t.Fatalf("save canonical: %v", err)
	}
	for _, w := range writes {
		if filepath.Base(w.Path) == ".meta.json" {
			continue
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}
}

func TestCanonicalFieldOverrideDoesNotResurrectIntoCanonical(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// Two providers define the agent with the SAME shared description "real".
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: real\nmodel: sonnet\n",
		"Shared body.")
	writeOpencodeAgent(t, dir, "dev", "real", "sonnet", "Shared body.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	aDir := canonical.AgentDir(dir, "dev")

	// Establish the per-provider override: canonical.description stays "real", but
	// claude-code carries description="override". Inject it by editing the
	// canonical directly (a user-set providerOverrides), then re-sync so it lands
	// on disk via RestoreOverrides.
	can, err := canonical.Load(aDir)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Description != "real" {
		t.Fatalf("setup: canonical.description=%q, want real", can.Description)
	}
	can.ProviderOverrides = map[string]map[string]any{
		"claude-code": {"description": "override"},
	}
	saveCanonicalNoMeta(t, dir, can)

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("override-injection sync: res=%+v err=%v", res, err)
	}

	// Sanity: the claude file now shows the OVERRIDE, opencode keeps the shared
	// value, and the canonical still carries description="real" + the bucket.
	if c := readClaude(t, dir, "dev"); !strings.Contains(c, "description: override") {
		t.Fatalf("setup: claude file did not get the override description:\n%s", c)
	}
	can, err = canonical.Load(aDir)
	if err != nil {
		t.Fatalf("reload canonical after injection: %v", err)
	}
	if can.Description != "real" {
		t.Fatalf("setup: canonical.description=%q after injection, want real (override leaked already)", can.Description)
	}
	if b := can.ProviderOverrides["claude-code"]; b == nil || b["description"] != "override" {
		t.Fatalf("setup: claude-code override bucket missing description: %+v", can.ProviderOverrides)
	}

	// THE BUG TRIGGER: the user edits an UNRELATED line of the claude provider
	// file (the body), keeping description: override. This makes the claude file a
	// drift source, so its parsed description ("override") flows through foldProvider.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: override\nmodel: sonnet\n",
		"Edited body, unrelated to description.")

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("re-sync after unrelated edit: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("re-sync status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// (a) canonical.description MUST stay "real" — the override must NOT have been
	// promoted into the shared canonical.
	can2, err := canonical.Load(aDir)
	if err != nil {
		t.Fatalf("reload canonical: %v", err)
	}
	if can2.Description != "real" {
		t.Errorf("canonical.description=%q, want \"real\" (per-provider override RESURRECTED into shared canonical — data loss)", can2.Description)
	}

	// (b) claude-code keeps its override.
	if b := can2.ProviderOverrides["claude-code"]; b == nil || b["description"] != "override" {
		t.Errorf("claude-code lost its description override: %+v", can2.ProviderOverrides)
	}

	// (c) the OTHER provider (opencode) still shows the shared "real" value, not
	// the leaked override.
	oc, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", "dev.md"))
	if err != nil {
		t.Fatalf("read opencode: %v", err)
	}
	if !strings.Contains(string(oc), "description: real") {
		t.Errorf("opencode no longer shows shared description \"real\" (override leaked across providers):\n%s", oc)
	}
	if strings.Contains(string(oc), "override") {
		t.Errorf("opencode file contains the claude-only override value:\n%s", oc)
	}

	// (d) the claude file still carries the override (round-trips, not dropped).
	if c := readClaude(t, dir, "dev"); !strings.Contains(c, "description: override") {
		t.Errorf("claude file lost its description override after re-sync:\n%s", c)
	}
}

// TestSharedCanonicalFieldStillPropagatesWithoutPriorOverride proves the guard
// does NOT break the normal shared-field case: when there is NO prior override
// for a field, a provider edit to that field promotes to the shared canonical
// and propagates to every provider exactly as before.
func TestSharedCanonicalFieldStillPropagatesWithoutPriorOverride(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: original\nmodel: sonnet\n",
		"Shared body.")
	writeOpencodeAgent(t, dir, "dev", "original", "sonnet", "Shared body.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("initial sync: res=%+v err=%v", res, err)
	}

	// No per-provider override exists. Edit claude's description -> it is a genuine
	// shared-field change and must promote to the canonical + propagate.
	writeClaudeRaw(t, dir, "dev",
		"name: dev\ndescription: updated\nmodel: sonnet\n",
		"Shared body.")

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("re-sync: res=%+v err=%v", res, err)
	}

	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if can.Description != "updated" {
		t.Errorf("canonical.description=%q, want \"updated\" (shared-field change did not promote)", can.Description)
	}
	if b := can.ProviderOverrides["claude-code"]; b != nil && b["description"] != nil {
		t.Errorf("shared-field change wrongly captured as a claude-code override: %+v", b)
	}
	// opencode receives the propagated change.
	oc, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", "dev.md"))
	if err != nil {
		t.Fatalf("read opencode: %v", err)
	}
	if !strings.Contains(string(oc), "description: updated") {
		t.Errorf("shared-field change did not propagate to opencode:\n%s", oc)
	}
}
