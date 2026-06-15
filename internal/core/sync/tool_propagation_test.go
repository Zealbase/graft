package sync

// Track C — e2e tool propagation + two-provider tool merge through the sync
// engine (real git, real transform.Default(), real sqlite store).
//
// Level: e2e/sync — the full detect -> canonicalize -> merge -> apply lifecycle.
// Covers:
//   - PROPAGATION: a tool added to ONE provider's agent file canonicalizes into
//     .graft/agents/<name> and re-fans to >=1 OTHER provider in that provider's
//     native spelling.
//   - TWO-PROVIDER MERGE: a tool added in provider X and a DIFFERENT tool added
//     in provider Y union in the canonical store and re-fan to all providers.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// writeClaudeAgentTools drops a Claude Code agent file with a comma-separated
// native `tools:` frontmatter field, so a tool enters the pipeline from claude.
func writeClaudeAgentTools(t *testing.T, dir, name, desc, tools, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: " + desc +
		"\nmodel: sonnet\ntools: " + tools + "\n---\n" + body + "\n"
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"), content)
}

// writeOpencodeAgentTools drops an opencode agent file whose `tools:` object map
// enables a set of native tools.
func writeOpencodeAgentTools(t *testing.T, dir, name, desc string, nativeTools []string, body string) {
	t.Helper()
	var b strings.Builder
	// Same model string as the claude seed so the ONLY divergence between the two
	// providers' canonical forms is the tools set (isolating the tool merge).
	b.WriteString("---\ndescription: " + desc + "\nmodel: sonnet\ntools:\n")
	for _, tool := range nativeTools {
		b.WriteString("  " + tool + ": true\n")
	}
	b.WriteString("---\n" + body + "\n")
	writeFile(t, dir, filepath.Join(".opencode", "agents", name+".md"), b.String())
}

// TestE2E_ToolPropagation_ClaudeToOthers seeds a tool ONLY on the claude-code
// agent file, syncs, and asserts:
//   - .graft/agents/<name> canonical holds the CANONICAL tool name.
//   - at least one OTHER provider's rendered file carries the tool in ITS native
//     spelling (cross-provider propagation).
func TestE2E_ToolPropagation_ClaudeToOthers(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	// claude native "WebSearch" -> canonical "web_search".
	writeClaudeAgentTools(t, dir, "scout", "a scout", "WebSearch", "You search the web.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	eng := New(st, tr, gitx.New(dir), dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// Canonical store holds the canonical tool name.
	can, err := canonical.Load(canonical.AgentDir(dir, "scout"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if !sliceContains(can.Tools, "web_search") {
		t.Fatalf("canonical tools = %v, want to contain web_search", can.Tools)
	}

	// Cross-provider propagation: opencode renders web_search as "websearch"
	// in its native bool-map. This is a genuine claude→canonical→opencode proof
	// (distinct serialisation format, distinct tool spelling). Assert >=1.
	//
	// NOTE: grok-cli is omitted here because its Serialize writes no `tools`
	// field (no per-agent tool-control frontmatter) and the grok-cli file path
	// is .grok/agents/<name>.json (not .md), so looking for tool names inside
	// it is doubly vacuous.
	type want struct {
		rel    string
		native string
	}
	checks := []want{
		{filepath.Join(".opencode", "agents", "scout.md"), "websearch"},
	}
	propagated := 0
	for _, c := range checks {
		data, err := os.ReadFile(filepath.Join(dir, c.rel))
		if err != nil {
			continue // provider may render to a different path; skip
		}
		if strings.Contains(string(data), c.native) {
			propagated++
		}
	}
	if propagated < 1 {
		// Dump what opencode produced to aid debugging.
		var dump strings.Builder
		for _, c := range checks {
			if data, err := os.ReadFile(filepath.Join(dir, c.rel)); err == nil {
				dump.WriteString("--- " + c.rel + " ---\n" + string(data) + "\n")
			}
		}
		t.Fatalf("tool did not propagate to any other provider in native spelling:\n%s", dump.String())
	}
}

// TestE2E_TwoProviderToolMerge seeds the SAME agent from two providers, each
// adding a DIFFERENT tool, and asserts the 3-way merge UNIONS both tools into
// the canonical store and re-fans them to all providers in native spelling.
func TestE2E_TwoProviderToolMerge(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)

	// Establish a common ancestor first: both providers share a base tool
	// (read_file) so the canonical `tools:` block already exists. claude native
	// "Read" and opencode native "read" both canonicalize to read_file.
	writeClaudeAgentTools(t, dir, "dev", "a developer", "Read", "Shared body.")
	writeOpencodeAgentTools(t, dir, "dev", "a developer", []string{"read"}, "Shared body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	eng := New(st, tr, gitx.New(dir), dir).SetHomeBase(t.TempDir())

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("ancestor sync: res=%+v err=%v", res, err)
	}

	// Now each provider adds a DIFFERENT tool on top of the shared base:
	// claude adds WebSearch (-> web_search); opencode adds bash (-> bash).
	// Because both retain the shared read_file, the distinct additions land on
	// non-overlapping lines relative to the ancestor and must UNION (not conflict).
	writeClaudeAgentTools(t, dir, "dev", "a developer", "Read, WebSearch", "Shared body.")
	writeOpencodeAgentTools(t, dir, "dev", "a developer", []string{"read", "bash"}, "Shared body.")

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("merge sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v) — tool additions on different "+
			"providers must UNION, not conflict", res.Status, res.Conflicts)
	}

	// Canonical store unions BOTH tools.
	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if !sliceContains(can.Tools, "web_search") || !sliceContains(can.Tools, "bash") ||
		!sliceContains(can.Tools, "read_file") {
		t.Fatalf("canonical tools = %v, want union {read_file, web_search, bash}", can.Tools)
	}

	// Re-fan: claude-code carries BOTH in its native spelling (WebSearch, Bash).
	claudeData, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "dev.md"))
	if err != nil {
		t.Fatalf("read claude file: %v", err)
	}
	cs := string(claudeData)
	if !strings.Contains(cs, "WebSearch") || !strings.Contains(cs, "Bash") {
		t.Errorf("claude-code did not receive both merged tools (WebSearch, Bash):\n%s", cs)
	}

	// opencode carries both in its native object map (websearch, bash).
	ocData, err := os.ReadFile(filepath.Join(dir, ".opencode", "agents", "dev.md"))
	if err != nil {
		t.Fatalf("read opencode file: %v", err)
	}
	ocs := string(ocData)
	if !strings.Contains(ocs, "websearch") || !strings.Contains(ocs, "bash") {
		t.Errorf("opencode did not receive both merged tools (websearch, bash):\n%s", ocs)
	}
}

func sliceContains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}
