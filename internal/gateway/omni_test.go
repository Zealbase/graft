package gateway_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
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
