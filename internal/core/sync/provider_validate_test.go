package sync

// Runtime provider-schema validation gate tests (companion to
// post_sync_validate_test.go). These prove the new gate is REAL — it runs inside
// the actual Engine.Run finalize path (after the canonical gate, after the merge
// is committed and the run is done) and validates the EMITTED provider files
// against each provider's own (composed) schema:
//
//	(a) a clean sync passes the provider-schema gate (no ProviderSchemaFindings),
//	    proving the conservative prose-type composer never produces false
//	    failures on real emitted files;
//	(b) an emitted provider file missing a REQUIRED frontmatter field is CAUGHT
//	    by validateEmittedProviders, surfaced as a *ProviderSchemaValidationError
//	    tagged with the agent + provider, with no re-sync loop.
//
// Case (b) corrupts the emitted .claude file AFTER a clean Run and then calls
// eng.validateEmittedProviders directly: applyProviders writes provider files via
// os.WriteFile (not git.Copy), so there is no git seam between applyProviders and
// the gate to interpose on. Removing a REQUIRED field (description — required in
// every provider schema) triggers the composed `required` constraint regardless
// of how rich the (concurrently-regenerated) provider type schema is.
//
// Helpers (requireGit, newWorkspace, writeClaudeAgent, writeFile, newEngine) are
// defined in sync_test.go (same package) and reused here.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestProviderSchemaValidationCleanPasses proves a clean sync passes the runtime
// provider-schema gate: Engine.Run returns no error, no ProviderSchemaFindings,
// and the run is done. This also exercises the conservative composer end-to-end
// against every registered provider's real Schema() bytes.
func TestProviderSchemaValidationCleanPasses(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("clean sync returned error: %v", err)
	}
	if fs := ProviderSchemaFindings(err); len(fs) > 0 {
		t.Fatalf("clean sync produced provider-schema findings: %+v", fs)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done", res.Status)
	}

	// Direct call after the clean run must also be clean — proves the composed
	// schemas validate the real emitted files without false positives.
	if verr := eng.validateEmittedProviders(res.Changed); verr != nil {
		t.Fatalf("validateEmittedProviders on clean emitted files: %v", verr)
	}
}

// TestProviderSchemaValidationCatchesCorruption proves the gate is real: an
// emitted Claude Code agent file with the REQUIRED `description` frontmatter key
// removed (after a clean sync) is CAUGHT by validateEmittedProviders, which
// returns a *ProviderSchemaValidationError carrying an error finding tagged with
// the agent + provider. No second Run and no loop is needed.
func TestProviderSchemaValidationCatchesCorruption(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgent(t, dir, "reviewer", "reviews code", "You review code.")

	eng, st := newEngine(t, dir)
	defer st.Close()

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("clean sync returned error: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status = %s, want done", res.Status)
	}

	// Corrupt the EMITTED provider file: drop the required `description` key from
	// the .claude/agents/reviewer.md frontmatter. The canonical store is untouched
	// (canonical gate would still pass); only the provider-schema gate fires.
	emitted := filepath.Join(dir, ".claude", "agents", "reviewer.md")
	broken := "---\nname: reviewer\nmodel: sonnet\n---\nYou review code.\n"
	if werr := os.WriteFile(emitted, []byte(broken), 0o644); werr != nil {
		t.Fatalf("rewrite emitted file: %v", werr)
	}

	gerr := eng.validateEmittedProviders([]string{"reviewer"})
	if gerr == nil {
		t.Fatalf("expected provider-schema validation error, got nil")
	}

	var pse *ProviderSchemaValidationError
	if !errors.As(gerr, &pse) {
		t.Fatalf("error is not *ProviderSchemaValidationError: %T: %v", gerr, gerr)
	}
	fs := ProviderSchemaFindings(gerr)
	if len(fs) == 0 {
		t.Fatalf("ProviderSchemaValidationError carried no findings")
	}
	foundReviewer := false
	for _, f := range fs {
		if f.Severity != "error" {
			t.Fatalf("non-error finding leaked into gate: %+v", f)
		}
		if f.Provider == "" {
			t.Fatalf("finding not tagged with a provider: %+v", f)
		}
		if f.Agent == "reviewer" {
			foundReviewer = true
		}
	}
	if !foundReviewer {
		t.Fatalf("findings did not flag agent reviewer: %+v", fs)
	}
}
