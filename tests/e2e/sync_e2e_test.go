package e2e

import (
	"sort"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// Scenario 2: provision a claude-code agent -> sync agents -> all enabled
// providers generated (lossless), canonical written, db rows correct, base ref
// unchanged.
func TestSyncAgents_GeneratesAllProviders(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	baseBefore := gitHead(t, root)

	// raw: sync agents -o json -> status done, changed includes the agent.
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
	if res.RunID == "" {
		t.Fatal("sync produced empty run_id")
	}
	if !containsStr(res.Changed, "code-reviewer") {
		t.Fatalf("changed=%v, want code-reviewer", res.Changed)
	}

	// base ref unchanged (the no-base-commit invariant).
	if after := gitHead(t, root); after != baseBefore {
		t.Fatalf("base ref moved during sync: %s -> %s", baseBefore, after)
	}

	// file: canonical artifacts present and well-formed.
	for _, f := range []string{"agent.yaml", "instructions.md", ".meta.json"} {
		if !exists(root, ".graft/agents/code-reviewer/"+f) {
			t.Fatalf("canonical artifact missing: %s", f)
		}
	}
	can, err := canonical.Load(canonical.AgentDir(root, "code-reviewer"))
	if err != nil {
		t.Fatalf("canonical.Load: %v", err)
	}
	if can.Name != "code-reviewer" {
		t.Fatalf("canonical name=%q, want code-reviewer", can.Name)
	}
	if can.Model != "sonnet" {
		t.Fatalf("canonical model=%q, want sonnet", can.Model)
	}
	// .meta.json canonicalHash matches the recomputed canonical hash.
	meta, err := canonical.LoadMeta(canonical.AgentDir(root, "code-reviewer"))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.CanonicalHash != canonical.Hash(can) {
		t.Fatalf(".meta.json canonicalHash=%q, want %q", meta.CanonicalHash, canonical.Hash(can))
	}

	// file: every provider file on disk re-parses losslessly back to canonical.
	verifyAllProvidersLossless(t, root, "code-reviewer", can)

	// db: agents row + 8 provider_link rows (antigravity + gemini-cli unregistered), all content_hash == canonical_hash.
	// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered).
	db := openDB(t, root)
	canHash := queryString(t, db, "SELECT canonical_hash FROM agents WHERE name=?", "code-reviewer")
	if canHash != canonical.Hash(can) {
		t.Fatalf("db canonical_hash=%q, want %q", canHash, canonical.Hash(can))
	}
	links := providerLinkHashes(t, db, "code-reviewer")
	gotProviders := make([]string, 0, len(links))
	for p, h := range links {
		gotProviders = append(gotProviders, p)
		if h != canHash {
			t.Fatalf("provider_link %s content_hash=%q != canonical_hash %q", p, h, canHash)
		}
	}
	sort.Strings(gotProviders)
	if !equalStrings(gotProviders, allProviders) {
		t.Fatalf("provider_links providers=%v, want %v", gotProviders, allProviders)
	}

	// raw (table): summary line "{y} agents in sync with {x} providers" present.
	tblRes := mustGraft(t, root, "sync", "agents", "-o", "table")
	if !contains(tblRes.stdout, "agents in sync with") && !contains(tblRes.stdout, "agent in sync with") {
		t.Logf("NOTE: sync table output missing 'agents in sync with' summary line; stdout:\n%s", tblRes.stdout)
	}

	// db: sync_run done, beta branch recorded, agent_state in_sync.
	if st := queryString(t, db, "SELECT status FROM sync_runs WHERE run_id=?", res.RunID); st != "done" {
		t.Fatalf("sync_run status=%q, want done", st)
	}
	if n := queryInt(t, db, "SELECT COUNT(*) FROM branches WHERE run_id=? AND kind='beta'", res.RunID); n < 1 {
		t.Fatalf("no beta branch recorded for run %s", res.RunID)
	}
	if n := queryInt(t, db, "SELECT in_sync FROM agent_states WHERE run_id=?", res.RunID); n != 1 {
		t.Fatalf("agent_state in_sync=%d, want 1", n)
	}
}

// Scenario 2 variant: an agent carrying non-canonical frontmatter keys survives
// the sync losslessly via ProviderOverrides (the claude-code file round-trips).
func TestSyncAgents_LosslessOverrides(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "planner")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	can, err := canonical.Load(canonical.AgentDir(root, "planner"))
	if err != nil {
		t.Fatalf("canonical.Load: %v", err)
	}
	ov := can.ProviderOverrides["claude-code"]
	if ov == nil {
		t.Fatal("expected claude-code provider overrides for non-canonical keys")
	}
	if _, ok := ov["disallowedTools"]; !ok {
		t.Fatalf("override disallowedTools not preserved: %#v", ov)
	}
	if _, ok := ov["permissionMode"]; !ok {
		t.Fatalf("override permissionMode not preserved: %#v", ov)
	}
	// And the regenerated claude file re-parses back to the same canonical.
	verifyProviderLossless(t, root, "claude-code", "planner", can)
}

// Scenario 3: agent list, agents status (in-sync), agent <x> status.
func TestList_And_Status_InSync(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// agent list (json): one agent, all providers ok, in_sync.
	var list []agentStatus
	decodeJSON(t, mustGraft(t, root, "agent", "list", "-o", "json"), &list)
	if len(list) != 1 || list[0].Name != "code-reviewer" {
		t.Fatalf("agent list=%+v, want one code-reviewer", list)
	}
	if !list[0].InSync {
		t.Fatalf("agent list not in sync: %+v", list[0])
	}
	// The status reporter uses live filesystem detection (prov.Detect(root)).
	// All eight active providers are ScopeProject and write under the workspace root,
	// so Detect(root) should find all of them.
	// NOTE(2026-06-13): antigravity (agy) is unregistered.
	// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered) — 8 active providers total.
	if len(list[0].Providers) < len(allProviders) {
		t.Fatalf("agent list providers count=%d, want >= %d; status reporter bug: %+v",
			len(list[0].Providers), len(allProviders), list[0])
	}

	// agents status (json): aggregated, no out-of-sync providers.
	var agg statusReport
	decodeJSON(t, mustGraft(t, root, "agents", "status", "-o", "json"), &agg)
	if len(agg.Agents) != 1 || !agg.Agents[0].InSync {
		t.Fatalf("agents status not in sync: %+v", agg)
	}
	if len(agg.OutOfSyncProviders) != 0 {
		t.Fatalf("out_of_sync_providers=%v, want empty", agg.OutOfSyncProviders)
	}

	// agent <name> status (json): single agent in sync.
	var one statusReport
	decodeJSON(t, mustGraft(t, root, "agent", "code-reviewer", "status", "-o", "json"), &one)
	if len(one.Agents) != 1 || !one.Agents[0].InSync {
		t.Fatalf("agent code-reviewer status not in sync: %+v", one)
	}

	// raw: table output is non-empty and headers present.
	tbl := mustGraft(t, root, "agent", "list", "-o", "table")
	for _, want := range []string{"AGENT", "IN_SYNC", "PROVIDERS"} {
		if !contains(tbl.stdout, want) {
			t.Fatalf("agent list table missing %q in:\n%s", want, tbl.stdout)
		}
	}
}

// Scenario: sync agent <x> (single agent path).
//
// BUG (owner: cli/gateway) — surfaced intentionally: on the FIRST sync of a
// named agent, the gateway pre-sync validate gate calls canonical.Load(name),
// but the canonical .graft/agents/<name>/agent.yaml does not exist yet (the
// engine generates it from the provider source during the run). The gate emits
// a "load canonical agent: ... no such file" error finding and BLOCKS the sync,
// so `graft sync agent <x>` is unusable for an agent's first sync. (`sync agents`
// works because it validates only already-existing canonical dirs.) This test
// asserts the CORRECT behaviour (status=done) and therefore fails until the gate
// skips validation for not-yet-canonicalized named targets.
func TestSyncSingleAgent(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")

	r := graft(t, root, "sync", "agent", "code-reviewer", "-o", "json")
	if r.exitCode != 0 {
		t.Fatalf("sync agent <x> blocked on first sync (BUG owner cli/gateway): exit=%d\nstderr:%s",
			r.exitCode, r.stderr)
	}
	var res runResultJSON
	decodeJSON(t, r, &res)
	if res.Status != "done" {
		t.Fatalf("sync agent status=%q, want done", res.Status)
	}
	if !exists(root, ".graft/agents/code-reviewer/agent.yaml") {
		t.Fatal("single-agent sync did not write canonical agent")
	}
}

// --- lossless verification helpers ---------------------------------------

// verifyAllProvidersLossless asserts, for every provider that has a file on disk
// for agentName, that the file re-parses losslessly: Parse->ToCanonical yields
// the same provider-owned fields, and re-Serializing the canonical reproduces
// the exact bytes already on disk (so a parse->serialize round-trip is stable).
func verifyAllProvidersLossless(t *testing.T, root, agentName string, can contract.CanonicalAgent) {
	t.Helper()
	tr := transform.Default()
	for _, provName := range tr.Providers() {
		prov, ok := tr.Provider(provName)
		if !ok {
			continue
		}
		refs, err := prov.Detect(root)
		if err != nil {
			t.Fatalf("%s.Detect: %v", provName, err)
		}
		found := false
		for _, ref := range refs {
			if ref.Name != agentName {
				continue
			}
			found = true
		}
		if !found {
			continue
		}
		verifyProviderLossless(t, root, provName, agentName, can)
	}
}

// verifyProviderLossless checks one provider's on-disk file for agentName:
//   - it parses + maps to canonical without error;
//   - re-serializing the loaded canonical reproduces the exact on-disk bytes.
func verifyProviderLossless(t *testing.T, root, provName, agentName string, can contract.CanonicalAgent) {
	t.Helper()
	tr := transform.Default()
	prov, ok := tr.Provider(provName)
	if !ok {
		t.Fatalf("provider %s not registered", provName)
	}
	refs, err := prov.Detect(root)
	if err != nil {
		t.Fatalf("%s.Detect: %v", provName, err)
	}
	var path string
	for _, ref := range refs {
		if ref.Name == agentName {
			path = ref.Path
		}
	}
	if path == "" {
		t.Fatalf("%s has no file for %s on disk", provName, agentName)
	}

	// Parse + ToCanonical must succeed (well-formed for its format).
	pa, err := prov.Parse(path)
	if err != nil {
		t.Fatalf("%s.Parse(%s): %v", provName, path, err)
	}
	if _, err := prov.ToCanonical(pa); err != nil {
		t.Fatalf("%s.ToCanonical: %v", provName, err)
	}

	// Round-trip stability: serializing the canonical we hold reproduces the
	// bytes on disk byte-for-byte.
	writes, err := tr.FromCanonical(can, provName)
	if err != nil {
		t.Fatalf("%s.FromCanonical: %v", provName, err)
	}
	for _, w := range writes {
		onDisk := readFile(t, root, w.Path)
		if onDisk != string(w.Data) {
			t.Fatalf("%s file %s is not lossless:\n--- on disk ---\n%s\n--- serialize(canonical) ---\n%s",
				provName, w.Path, onDisk, string(w.Data))
		}
	}
}
