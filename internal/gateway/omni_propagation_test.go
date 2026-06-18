package gateway_test

// Phase-d integration test: the omni sentinel block fans out UNCHANGED to every
// provider's emitted agent file, and a re-sync is byte-identical (idempotent).
//
// Why this lives here (not in core/sync): driving the SUPPORTED omni path needs
// the in-process resolver seam (SetOmniResolver) — the CLI subprocess always uses
// the honest DefaultOmniResolver (Supported()=false). The gateway is the only
// surface that combines omni init (CreateAgentWithOmni) + a full multi-provider
// Sync + Validate in one in-process call, with HOME/XDG host-isolation from
// newGitWorkspace. core/sync itself has ZERO omni references: it treats the
// sentinel block as ordinary, opaque canonical Body text (foldProvider copies
// Body verbatim; the merge surface is the canonical file), so the block fans out
// through FromCanonical to each provider with no special handling. This test
// PROVES that integrity end-to-end rather than asserting it in sync internals.

import (
	"os"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// omniSysInstr is the fixed multiline sys-instructions the test resolver emits.
// Multiline + blank line + a trailing instruction exercises body fidelity across
// providers that re-encode the body (e.g. codex's TOML developer_instructions).
const omniSysInstr = "You are a shared omni agent.\n" +
	"Always be precise and cite sources.\n" +
	"\n" +
	"Never fabricate tool output."

// providerAgentFile resolves the on-disk path of the agent file a provider
// emitted for `name`, by re-running the provider's own Detect under its scope
// base (root for ScopeProject, $HOME for ScopeHome — same discrimination
// applyProviders uses). Returns "" when the provider produced no file for the
// agent (a provider that cannot express it is legitimately skipped).
func providerAgentFile(t *testing.T, tr *transform.Registry, root, provName, name string) string {
	t.Helper()
	prov, ok := tr.Provider(provName)
	if !ok {
		t.Fatalf("provider %q missing from registry", provName)
	}
	base := root
	if sp, ok := prov.(contract.ScopedProvider); ok && sp.PathScope() == contract.ScopeHome {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir for scope-home provider %q: %v", provName, err)
		}
		base = home
	}
	refs, err := prov.Detect(base)
	if err != nil {
		t.Fatalf("%s Detect: %v", provName, err)
	}
	for _, ref := range refs {
		if ref.Name == name {
			return ref.Path
		}
	}
	return ""
}

// assertOmniBlockRoundTrips parses a provider file back to canonical and asserts
// the graft-managed omni block (and its sys-instructions content) survived the
// fan-out intact. Parsing back is the authoritative per-provider check: it proves
// the block is recoverable even for providers that re-encode the body on the wire
// (TOML escaping, frontmatter folding), which a raw substring scan could miss.
func assertOmniBlockRoundTrips(t *testing.T, tr *transform.Registry, provName, path, ref string) {
	t.Helper()
	prov, _ := tr.Provider(provName)
	pa, err := prov.Parse(path)
	if err != nil {
		t.Fatalf("%s Parse(%s): %v", provName, path, err)
	}
	can, err := prov.ToCanonical(pa)
	if err != nil {
		t.Fatalf("%s ToCanonical: %v", provName, err)
	}
	if !canonical.HasOmniBlock(can.Body) {
		t.Fatalf("%s: omni block missing after fan-out + round-trip:\n%s", provName, can.Body)
	}
	if c := strings.Count(can.Body, "<!-- graft:omni "+ref+" -->"); c != 1 {
		t.Fatalf("%s: expected exactly one omni open marker for ref %q, got %d:\n%s",
			provName, ref, c, can.Body)
	}
	for _, line := range []string{
		"You are a shared omni agent.",
		"Always be precise and cite sources.",
		"Never fabricate tool output.",
	} {
		if !strings.Contains(can.Body, line) {
			t.Fatalf("%s: omni sys-instructions line %q lost in fan-out:\n%s", provName, line, can.Body)
		}
	}
}

// TestIntegration_OmniBlockFansOutToAllProvidersIdempotent is the Phase-d gate.
// With a Supported()=true resolver injected, it creates an agent carrying the
// omni block, runs a full multi-provider sync, and asserts:
//
//  1. EVERY provider that can express the agent emits a file whose parsed-back
//     canonical Body still carries the single, intact omni block + content
//     (the block fanned out UNCHANGED — sync never re-processed/stripped it).
//  2. A SECOND sync leaves every provider file BYTE-IDENTICAL (idempotence; no
//     double-prepend, no churn).
//  3. `Validate("all")` reports no error-severity findings on the generated tree.
func TestIntegration_OmniBlockFansOutToAllProvidersIdempotent(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	// Inject the future omni capability so the applied (prepended) path runs.
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(supportedResolver{header: omniSysInstr})

	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const (
		name = "omnibot"
		ref  = "team-shared"
	)
	a, res, err := g.(gateway.AgentOmniCapable).CreateAgentWithOmni(name, "Base agent body.", ref)
	if err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	if !res.Supported || !res.Applied {
		t.Fatalf("expected supported+applied omni: %+v", res)
	}
	if !canonical.HasOmniBlock(a.Body) {
		t.Fatalf("canonical Body missing omni block after init:\n%s", a.Body)
	}

	// A scaffolded agent has no description; the pre-sync validate gate requires
	// one (providers need it to detect the agent). Set it on disk WITHOUT touching
	// the omni-bearing Body, preserving the recorded omni meta.
	dir := canonical.AgentDir(root, name)
	on, err := canonical.Load(dir)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	on.Description = "Shared omni agent for fan-out integrity."
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	writes, err := canonical.SaveWithMeta(root, on, meta)
	if err != nil {
		t.Fatalf("save canonical: %v", err)
	}
	for _, w := range writes {
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}
	if !canonical.HasOmniBlock(on.Body) {
		t.Fatalf("omni block lost after setting description:\n%s", on.Body)
	}

	// --- First sync: fan the omni-bearing canonical out to every provider. ---
	if r, err := g.Sync(contract.SyncOpts{Ingest: true}); err != nil {
		t.Fatalf("first Sync: %v", err)
	} else if r.Status != contract.RunDone {
		t.Fatalf("first sync status=%q, want done (conflicts=%v)", r.Status, r.Conflicts)
	}

	tr := transform.Default()
	providers := tr.Providers()
	if len(providers) < 8 {
		t.Fatalf("expected the full active provider set, got %d: %v", len(providers), providers)
	}

	// Locate each provider's emitted file, assert the block round-trips, and
	// snapshot the raw bytes for the idempotence comparison. At least one provider
	// must have emitted a file (otherwise the fan-out assertion is vacuous).
	type emitted struct {
		path  string
		bytes []byte
	}
	first := map[string]emitted{}
	for _, provName := range providers {
		path := providerAgentFile(t, tr, root, provName, name)
		if path == "" {
			// A provider that cannot express this agent emits no file — skip it, but
			// record nothing so the re-sync loop does not expect a file either.
			continue
		}
		assertOmniBlockRoundTrips(t, tr, provName, path, ref)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s read emitted file %s: %v", provName, path, err)
		}
		first[provName] = emitted{path: path, bytes: b}
	}
	if len(first) == 0 {
		t.Fatalf("no provider emitted a file for %q — fan-out assertion is vacuous", name)
	}

	// --- Second sync: must be a true no-op for the omni block. Every provider
	// file is byte-identical to the first run (no double-prepend, no churn). ---
	if r, err := g.Sync(contract.SyncOpts{Ingest: true}); err != nil {
		t.Fatalf("second Sync: %v", err)
	} else if r.Status != contract.RunDone {
		t.Fatalf("second sync status=%q, want done (conflicts=%v)", r.Status, r.Conflicts)
	}
	for provName, e := range first {
		path := providerAgentFile(t, tr, root, provName, name)
		if path != e.path {
			t.Fatalf("%s emitted a different path on re-sync: %q != %q", provName, path, e.path)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s read re-synced file %s: %v", provName, path, err)
		}
		if string(got) != string(e.bytes) {
			t.Fatalf("%s NOT byte-identical after re-sync (omni block churned / double-prepended)\n--- first ---\n%s\n--- second ---\n%s",
				provName, e.bytes, got)
		}
		// Belt-and-suspenders: still exactly one managed block per provider file.
		assertOmniBlockRoundTrips(t, tr, provName, path, ref)
	}

	// --- Validate the generated tree: no error-severity findings. ---
	findings, err := g.Validate("all")
	if err != nil {
		t.Fatalf("Validate(all): %v", err)
	}
	for _, f := range findings {
		if f.Severity == "error" {
			t.Fatalf("validate error finding on omni-fanned tree: %+v", f)
		}
	}
}
