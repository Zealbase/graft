package gateway_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// supportedResolver is a test OmniResolver that supports every ref and emits a
// fixed sys-instructions header. It stands in for the future omni capability so
// the applied path can be exercised (Phase f/g inject the same shape).
type supportedResolver struct{ header string }

func (r supportedResolver) Supported(string) bool { return true }
func (r supportedResolver) Resolve(string) (string, error) {
	return r.header, nil
}

// loadAgentBody reads the on-disk canonical Body (instructions.md) for an agent.
func loadAgentBody(t *testing.T, root, name string) string {
	t.Helper()
	a, err := canonical.Load(canonical.AgentDir(root, name))
	if err != nil {
		t.Fatalf("load %q: %v", name, err)
	}
	return a.Body
}

func loadAgentMeta(t *testing.T, root, name string) canonical.Meta {
	t.Helper()
	m, err := canonical.LoadMeta(canonical.AgentDir(root, name))
	if err != nil {
		t.Fatalf("load meta %q: %v", name, err)
	}
	return m
}

// TestCreateAgentWithOmniUnsupported: the default (honest) resolver is
// unsupported, so the ref is recorded in meta with applied=false/supported=false,
// the Body is left UNCHANGED, and a warning is returned (never an error).
func TestCreateAgentWithOmniUnsupported(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	oc := g.(gateway.AgentOmniCapable)

	a, res, err := oc.CreateAgentWithOmni("fixer", "Body text.", "fixer")
	if err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	if res.Supported || res.Applied {
		t.Fatalf("default resolver must be unsupported/not-applied: %+v", res)
	}
	if res.Warning == "" {
		t.Fatalf("expected a warning on the unsupported path")
	}
	if canonical.HasOmniBlock(a.Body) {
		t.Fatalf("Body must be unchanged on the unsupported path:\n%s", a.Body)
	}
	if got := loadAgentBody(t, root, "fixer"); canonical.HasOmniBlock(got) {
		t.Fatalf("on-disk Body must be unchanged:\n%s", got)
	}
	meta := loadAgentMeta(t, root, "fixer")
	if meta.Omni == nil || meta.Omni.Ref != "fixer" || meta.Omni.Applied || meta.Omni.Supported {
		t.Fatalf("meta.Omni not recorded as unsupported: %+v", meta.Omni)
	}
}

// TestCreateAgentWithOmniSupported: with an injected Supported=true resolver the
// header is prepended into Body and meta is marked applied/supported.
func TestCreateAgentWithOmniSupported(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(supportedResolver{header: "SHARED HEADER"})
	oc := g.(gateway.AgentOmniCapable)

	a, res, err := oc.CreateAgentWithOmni("fixer", "Body text.", "shared")
	if err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	if !res.Supported || !res.Applied || res.Warning != "" {
		t.Fatalf("expected applied+supported, no warning: %+v", res)
	}
	if !canonical.HasOmniBlock(a.Body) || !strings.Contains(a.Body, "SHARED HEADER") {
		t.Fatalf("Body missing omni block/header:\n%s", a.Body)
	}
	if !strings.Contains(a.Body, "Body text.") {
		t.Fatalf("original body must be preserved:\n%s", a.Body)
	}
	meta := loadAgentMeta(t, root, "fixer")
	if meta.Omni == nil || meta.Omni.Ref != "shared" || !meta.Omni.Applied || !meta.Omni.Supported {
		t.Fatalf("meta.Omni not marked applied/supported: %+v", meta.Omni)
	}
}

// TestRefreshOmniUnsupportedNoOp: omni --refresh against the default resolver is
// a clean no-op + warning, exit 0; Body stays unchanged.
func TestRefreshOmniUnsupportedNoOp(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	oc := g.(gateway.AgentOmniCapable)

	if _, _, err := oc.CreateAgentWithOmni("fixer", "Body text.", "fixer"); err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	before := loadAgentBody(t, root, "fixer")

	res, err := oc.RefreshOmni("fixer")
	if err != nil {
		t.Fatalf("RefreshOmni unsupported must not error: %v", err)
	}
	if res.Applied || res.Warning == "" {
		t.Fatalf("expected unsupported no-op + warning: %+v", res)
	}
	if after := loadAgentBody(t, root, "fixer"); after != before {
		t.Fatalf("Body changed on unsupported refresh:\nbefore=%q\nafter=%q", before, after)
	}
}

// TestRefreshOmniSupportedIdempotent: with a supported resolver, refresh applies
// the header and a second refresh replaces it in place (no duplication).
func TestRefreshOmniSupportedIdempotent(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	oc := g.(gateway.AgentOmniCapable)

	// Record ref while unsupported (default resolver).
	if _, _, err := oc.CreateAgentWithOmni("fixer", "Body text.", "fixer"); err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	// Capability ships: inject a supported resolver and refresh.
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(supportedResolver{header: "HDR"})
	if _, err := oc.RefreshOmni("fixer"); err != nil {
		t.Fatalf("RefreshOmni: %v", err)
	}
	once := loadAgentBody(t, root, "fixer")
	if c := strings.Count(once, "<!-- graft:omni"); c != 1 {
		t.Fatalf("expected exactly one omni open marker, got %d:\n%s", c, once)
	}
	// Second refresh: replace in place, byte-identical, still a single block.
	if _, err := oc.RefreshOmni("fixer"); err != nil {
		t.Fatalf("RefreshOmni (2): %v", err)
	}
	twice := loadAgentBody(t, root, "fixer")
	if twice != once {
		t.Fatalf("second refresh not idempotent:\n1=%q\n2=%q", once, twice)
	}
}

// TestRefreshOmniNoRefRecorded: refreshing an agent with no recorded omni ref is
// an error (nothing to refresh).
func TestRefreshOmniNoRefRecorded(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.CreateAgent("plain", "Body."); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := g.(gateway.AgentOmniCapable).RefreshOmni("plain"); err == nil {
		t.Fatalf("expected error refreshing an agent with no omni ref")
	}
}

// TestHydrateProviderScopedSandbox: hydrate exposes model/tools and a
// provider-scoped sandbox (codex sandbox_mode from providerOverrides).
func TestHydrateProviderScopedSandbox(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	if _, err := g.CreateAgent("svc", "Body."); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	// Seed canonical fields + a codex sandbox_mode override directly on disk.
	dir := canonical.AgentDir(root, "svc")
	a, err := canonical.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a.Model = "default-model"
	a.Tools = []string{"read_file", "bash"}
	a.ProviderOverrides = map[string]map[string]any{
		"codex": {"model": "gpt-5-codex", "sandbox_mode": "workspace-write"},
	}
	writes, err := canonical.SaveWithMeta(root, a, canonical.Meta{})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	hc := g.(gateway.HydrateCapable)

	// Provider-scoped: model is the codex override; sandbox_mode surfaces.
	view, err := hc.Hydrate("svc", "codex")
	if err != nil {
		t.Fatalf("Hydrate codex: %v", err)
	}
	if view.Name != "svc" || view.Model != "gpt-5-codex" {
		t.Fatalf("unexpected hydrate view: %+v", view)
	}
	if len(view.Tools) != 2 {
		t.Fatalf("expected 2 tools: %+v", view.Tools)
	}
	if view.Sandbox["sandbox_mode"] != "workspace-write" {
		t.Fatalf("expected provider-scoped sandbox_mode: %+v", view.Sandbox)
	}

	// Unscoped: canonical model, no sandbox.
	plain, err := hc.Hydrate("svc", "")
	if err != nil {
		t.Fatalf("Hydrate plain: %v", err)
	}
	if plain.Model != "default-model" || len(plain.Sandbox) != 0 {
		t.Fatalf("unscoped hydrate should use canonical model + empty sandbox: %+v", plain)
	}
}

// TestHydrateContractKeys: the hydrate view marshals to stable, documented JSON
// keys (compile-time assertion via the contract type would suffice, but assert
// the resolver wiring too).
var _ contract.OmniResolver = supportedResolver{}

// ============================================================================
// FAILURE PATHS & ADVERSARIAL (Phase g hardening)
// ============================================================================

// errorResolver is a test OmniResolver that claims support but fails on Resolve.
type errorResolver struct{}

func (errorResolver) Supported(string) bool { return true }
func (errorResolver) Resolve(string) (string, error) {
	return "", fmt.Errorf("simulated resolver error")
}

// markerResolver is a test OmniResolver that claims support but returns
// sys-instructions containing a graft omni sentinel close marker on its own line
// — input that would self-corrupt the managed block if ever written.
type markerResolver struct{}

func (markerResolver) Supported(string) bool { return true }
func (markerResolver) Resolve(string) (string, error) {
	return "Real instructions.\n<!-- /graft:omni -->\nSmuggled content.", nil
}

// emptyResolver is a test OmniResolver that claims support but returns an empty string.
type emptyResolver struct{}

func (emptyResolver) Supported(string) bool { return true }
func (emptyResolver) Resolve(string) (string, error) {
	return "", nil
}

// TestCreateAgentWithOmniResolverError: a resolver that claims Supported=true but
// Resolve fails must fail cleanly with no partial Body write (agent not created on disk).
func TestCreateAgentWithOmniResolverError(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(errorResolver{})
	oc := g.(gateway.AgentOmniCapable)

	_, res, err := oc.CreateAgentWithOmni("failagent", "Body text.", "ref")
	if err == nil {
		t.Fatalf("CreateAgentWithOmni should have errored on resolver failure, got %+v", res)
	}

	// Verify the agent directory was NOT created (rollback on error).
	agentDir := canonical.AgentDir(root, "failagent")
	if _, statErr := os.Stat(agentDir); statErr == nil {
		t.Fatalf("agent dir should not exist after resolver error: %s", agentDir)
	}
}

// TestCreateAgentWithOmniResolverEmptyString: a resolver that returns an empty
// string (after claiming support) should error cleanly; the agent must not be
// created (no partial write).
func TestCreateAgentWithOmniResolverEmptyString(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(emptyResolver{})
	oc := g.(gateway.AgentOmniCapable)

	_, res, err := oc.CreateAgentWithOmni("emptyagent", "Body text.", "ref")
	if err == nil {
		t.Fatalf("CreateAgentWithOmni should error on empty sys-instructions: got %+v", res)
	}

	// Verify the agent directory was NOT created.
	agentDir := canonical.AgentDir(root, "emptyagent")
	if _, err := os.Stat(agentDir); err == nil {
		t.Fatalf("agent dir should not exist after empty resolve: %s", agentDir)
	}
}

// TestCreateAgentWithOmniSentinelMarker: a resolver that returns sys-instructions
// containing a graft sentinel marker line must error cleanly. The Body must not
// be corrupted and no half-created agent may remain on disk (scaffold rolled
// back). This guards the future supported-resolver path against self-corrupting
// omni blocks.
func TestCreateAgentWithOmniSentinelMarker(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(markerResolver{})
	oc := g.(gateway.AgentOmniCapable)

	_, res, err := oc.CreateAgentWithOmni("markeragent", "Body text.", "ref")
	if err == nil {
		t.Fatalf("CreateAgentWithOmni should error on sentinel-marker sys-instructions, got %+v", res)
	}
	if !strings.Contains(err.Error(), "sentinel marker") {
		t.Fatalf("error should mention sentinel marker, got: %v", err)
	}

	// Scaffold must be rolled back: no half-created agent left behind.
	agentDir := canonical.AgentDir(root, "markeragent")
	if _, statErr := os.Stat(agentDir); statErr == nil {
		t.Fatalf("agent dir should not exist after sentinel-marker error: %s", agentDir)
	}
}

// TestRefreshOmniSentinelMarker: refresh with a resolver that returns a
// sentinel-marker sys-instruction must error cleanly and leave the existing Body
// uncorrupted (no partial write).
func TestRefreshOmniSentinelMarker(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	oc := g.(gateway.AgentOmniCapable)

	if _, _, err := oc.CreateAgentWithOmni("agent", "Body text.", "ref"); err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	before := loadAgentBody(t, root, "agent")

	g.(gateway.OmniResolverConfigurable).SetOmniResolver(markerResolver{})
	if _, err := oc.RefreshOmni("agent"); err == nil {
		t.Fatalf("RefreshOmni should error on sentinel-marker sys-instructions")
	}

	if after := loadAgentBody(t, root, "agent"); after != before {
		t.Fatalf("Body must not change on sentinel-marker refresh error:\nbefore=%q\nafter=%q", before, after)
	}
}

// TestRefreshOmniResolverError: refresh with a failing resolver must error
// cleanly without modifying the Body or marking as applied.
func TestRefreshOmniResolverError(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	// First create unsupported, then try to refresh with an error resolver.
	oc := g.(gateway.AgentOmniCapable)

	if _, _, err := oc.CreateAgentWithOmni("agent", "Body text.", "ref"); err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	before := loadAgentBody(t, root, "agent")
	metaBefore := loadAgentMeta(t, root, "agent")

	// Inject error resolver and attempt refresh.
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(errorResolver{})
	_, err := oc.RefreshOmni("agent")
	if err == nil {
		t.Fatalf("RefreshOmni should error on resolver failure")
	}

	// Verify Body and meta remain unchanged.
	after := loadAgentBody(t, root, "agent")
	if after != before {
		t.Fatalf("Body should not change on refresh error:\nbefore=%q\nafter=%q", before, after)
	}
	metaAfter := loadAgentMeta(t, root, "agent")
	if metaAfter.Omni.Ref != metaBefore.Omni.Ref ||
		metaAfter.Omni.Applied != metaBefore.Omni.Applied ||
		metaAfter.Omni.Supported != metaBefore.Omni.Supported {
		t.Fatalf("meta.Omni should not change on refresh error:\nbefore=%+v\nafter=%+v", metaBefore.Omni, metaAfter.Omni)
	}
}

// TestTransitionUnsupportedToSupportedToUnsupported: create unsupported, refresh
// (inject supported), refresh (back to unsupported) — meta + Body remain consistent.
func TestTransitionUnsupportedToSupportedToUnsupported(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	oc := g.(gateway.AgentOmniCapable)

	// Step 1: Create with unsupported resolver (default).
	if _, _, err := oc.CreateAgentWithOmni("agent", "Original body.", "ref"); err != nil {
		t.Fatalf("CreateAgentWithOmni unsupported: %v", err)
	}
	meta1 := loadAgentMeta(t, root, "agent")
	body1 := loadAgentBody(t, root, "agent")

	if meta1.Omni == nil || meta1.Omni.Applied || meta1.Omni.Supported {
		t.Fatalf("step 1: expected unsupported meta: %+v", meta1.Omni)
	}
	if canonical.HasOmniBlock(body1) {
		t.Fatalf("step 1: body should not have omni block: %s", body1)
	}

	// Step 2: Inject supported resolver and refresh.
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(supportedResolver{header: "HDR"})
	if _, err := oc.RefreshOmni("agent"); err != nil {
		t.Fatalf("RefreshOmni supported: %v", err)
	}
	meta2 := loadAgentMeta(t, root, "agent")
	body2 := loadAgentBody(t, root, "agent")

	if meta2.Omni == nil || !meta2.Omni.Applied || !meta2.Omni.Supported {
		t.Fatalf("step 2: expected applied+supported meta: %+v", meta2.Omni)
	}
	if !canonical.HasOmniBlock(body2) || !strings.Contains(body2, "HDR") {
		t.Fatalf("step 2: body should have omni block with header: %s", body2)
	}
	if !strings.Contains(body2, "Original body.") {
		t.Fatalf("step 2: original body should be preserved: %s", body2)
	}

	// Step 3: Back to default (unsupported) resolver, refresh again.
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(canonical.DefaultOmniResolver{})
	res, err := oc.RefreshOmni("agent")
	if err != nil {
		t.Fatalf("RefreshOmni back to unsupported: %v", err)
	}
	if res.Applied {
		t.Fatalf("step 3: should not be applied on unsupported resolver")
	}

	meta3 := loadAgentMeta(t, root, "agent")
	body3 := loadAgentBody(t, root, "agent")

	// Meta should reflect the unsupported status, but with the recorded ref.
	if meta3.Omni == nil || meta3.Omni.Ref != "ref" {
		t.Fatalf("step 3: meta.Omni should retain ref: %+v", meta3.Omni)
	}
	// Supported is now false again (the meta was updated by applyOmni).
	if meta3.Omni.Supported || meta3.Omni.Applied {
		t.Fatalf("step 3: meta should be unsupported/not-applied: %+v", meta3.Omni)
	}
	// Body should still have the omni block (it was not modified by the unsupported refresh).
	if !canonical.HasOmniBlock(body3) {
		t.Fatalf("step 3: body should still have omni block (unsupported refresh is a no-op): %s", body3)
	}
	// Should be equal to body2 (unsupported refresh doesn't change body).
	if body3 != body2 {
		t.Fatalf("step 3: body should be unchanged by unsupported refresh:\nbody2=%q\nbody3=%q", body2, body3)
	}
}

// largeAdversarialResolver is a test OmniResolver that returns a very large
// sys-instructions string (≥64 KB) containing CRLF and non-ASCII characters.
type largeAdversarialResolver struct{}

func (largeAdversarialResolver) Supported(string) bool { return true }
func (largeAdversarialResolver) Resolve(string) (string, error) {
	// Build a 64+ KB string with CRLF and non-ASCII: café, emoji, etc.
	base := "Adversarial header with CRLF\r\nand non-ASCII: café 🚀 Ω naïve.\r\n" +
		"This line repeats many times to grow the size:\r\n"
	for i := 0; i < 2000; i++ {
		base += "Line " + fmt.Sprintf("%04d", i) + ": repeat for size\r\n"
	}
	return base, nil
}

// TestCreateAgentWithOmniAdversarialPrepend: with a large (≥64 KB) multiline
// sys-instructions string containing CRLF and non-ASCII, init -> sync across all
// providers must result in byte-faithful fan-out to every provider file and a
// re-sync that is byte-identical.
func TestCreateAgentWithOmniAdversarialPrepend(t *testing.T) {
	root := newGitWorkspace(t)
	g := openGate(t, root)
	g.(gateway.OmniResolverConfigurable).SetOmniResolver(largeAdversarialResolver{})

	if _, err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const (
		name = "adversarial"
		ref  = "adv-ref"
	)
	a, res, err := g.(gateway.AgentOmniCapable).CreateAgentWithOmni(name, "Base body.", ref)
	if err != nil {
		t.Fatalf("CreateAgentWithOmni: %v", err)
	}
	if !res.Applied || !res.Supported {
		t.Fatalf("expected applied+supported: %+v", res)
	}

	// Prepare the agent for sync: add description (required for schema validation).
	dir := canonical.AgentDir(root, name)
	a.Description = "Adversarial test agent for large prepend."
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	writes, err := canonical.SaveWithMeta(root, a, meta)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	for _, w := range writes {
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", w.Path, err)
		}
	}

	tr := transform.Default()
	providers := tr.Providers()

	// First sync: fan out the large omni block to every provider.
	if r, err := g.Sync(contract.SyncOpts{Ingest: true}); err != nil {
		t.Fatalf("first Sync: %v", err)
	} else if r.Status != contract.RunDone {
		t.Fatalf("first sync status=%q, want done", r.Status)
	}

	type emitted struct {
		path  string
		bytes []byte
	}
	first := map[string]emitted{}
	for _, provName := range providers {
		path := providerAgentFile(t, tr, root, provName, name)
		if path == "" {
			continue
		}
		// Parse back to ensure the block is still valid (not truncated/corrupted).
		prov, _ := tr.Provider(provName)
		pa, err := prov.Parse(path)
		if err != nil {
			t.Fatalf("%s Parse adversarial file: %v", provName, err)
		}
		can, err := prov.ToCanonical(pa)
		if err != nil {
			t.Fatalf("%s ToCanonical: %v", provName, err)
		}
		if !canonical.HasOmniBlock(can.Body) {
			t.Fatalf("%s: adversarial omni block missing after fan-out:\n%s", provName, can.Body)
		}
		// Verify non-ASCII survived (CRLF may be normalized per-provider,
		// but core content must be preserved).
		if !strings.Contains(can.Body, "café") || !strings.Contains(can.Body, "🚀") {
			t.Fatalf("%s: non-ASCII lost in adversarial block", provName)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s read: %v", provName, err)
		}
		first[provName] = emitted{path: path, bytes: b}
	}

	if len(first) == 0 {
		t.Fatalf("no provider emitted a file — adversarial fan-out assertion vacuous")
	}

	// Second sync: must be byte-identical (no churn, no truncation).
	if r, err := g.Sync(contract.SyncOpts{Ingest: true}); err != nil {
		t.Fatalf("second Sync: %v", err)
	} else if r.Status != contract.RunDone {
		t.Fatalf("second sync status=%q, want done", r.Status)
	}

	for provName, e := range first {
		got, err := os.ReadFile(e.path)
		if err != nil {
			t.Fatalf("%s read after re-sync: %v", provName, err)
		}
		if string(got) != string(e.bytes) {
			t.Fatalf("%s NOT byte-identical after re-sync (adversarial churn detected)\nfirst len=%d, second len=%d",
				provName, len(e.bytes), len(got))
		}
	}
}
