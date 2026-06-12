package gateway

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// fakeProvider is a minimal contract.Provider used to drive modelFindings. When
// models is non-nil it also implements contract.ModelLister; modelsErr lets a
// test simulate ErrUnavailable / offline.
type fakeProvider struct {
	name      string
	models    []string
	modelsErr error
}

func (p fakeProvider) Name() string                               { return p.name }
func (p fakeProvider) Detect(string) ([]contract.AgentRef, error) { return nil, nil }
func (p fakeProvider) Parse(string) (contract.ProviderAgent, error) {
	return contract.ProviderAgent{}, nil
}
func (p fakeProvider) ToCanonical(contract.ProviderAgent) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{}, nil
}
func (p fakeProvider) Serialize(contract.CanonicalAgent) ([]contract.FileWrite, error) {
	return nil, nil
}
func (p fakeProvider) Schema() []byte { return nil }

// Models satisfies contract.ModelLister.
func (p fakeProvider) Models() ([]string, error) {
	if p.modelsErr != nil {
		return nil, p.modelsErr
	}
	return p.models, nil
}

// fakeProviderNoLister is a provider WITHOUT a Models method (no ModelLister).
type fakeProviderNoLister struct{ name string }

func (p fakeProviderNoLister) Name() string                               { return p.name }
func (p fakeProviderNoLister) Detect(string) ([]contract.AgentRef, error) { return nil, nil }
func (p fakeProviderNoLister) Parse(string) (contract.ProviderAgent, error) {
	return contract.ProviderAgent{}, nil
}
func (p fakeProviderNoLister) ToCanonical(contract.ProviderAgent) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{}, nil
}
func (p fakeProviderNoLister) Serialize(contract.CanonicalAgent) ([]contract.FileWrite, error) {
	return nil, nil
}
func (p fakeProviderNoLister) Schema() []byte { return nil }

// fakeTransformer holds a fixed set of providers keyed by name.
type fakeTransformer struct {
	providers map[string]contract.Provider
}

func (t fakeTransformer) ToCanonical(contract.ProviderAgent) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{}, nil
}
func (t fakeTransformer) FromCanonical(contract.CanonicalAgent, string) ([]contract.FileWrite, error) {
	return nil, nil
}
func (t fakeTransformer) Register(contract.Provider) {}
func (t fakeTransformer) Provider(name string) (contract.Provider, bool) {
	p, ok := t.providers[name]
	return p, ok
}
func (t fakeTransformer) Providers() []string {
	out := make([]string, 0, len(t.providers))
	for n := range t.providers {
		out = append(out, n)
	}
	return out
}

func newFakeGate(providers map[string]contract.Provider) *gate {
	return &gate{tr: fakeTransformer{providers: providers}}
}

func TestModelFindingsFlagsUnknownModel(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet", "opus"}},
	})
	a := contract.CanonicalAgent{Name: "x", Model: "gpt-9000"}
	findings := g.modelFindings(a)
	if len(findings) != 1 {
		t.Fatalf("want 1 warning finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != "warning" {
		t.Fatalf("model finding severity = %q, want warning", f.Severity)
	}
	if f.Provider != "claude-code" || f.Agent != "x" {
		t.Fatalf("unexpected finding target: %+v", f)
	}
}

func TestModelFindingsKnownModelClean(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet", "opus"}},
	})
	a := contract.CanonicalAgent{Name: "x", Model: "sonnet"}
	if f := g.modelFindings(a); len(f) != 0 {
		t.Fatalf("known model should produce no findings, got %+v", f)
	}
}

func TestModelFindingsSkipsWhenUnavailable(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", modelsErr: errors.New("models: unavailable (offline and no cache)")},
	})
	a := contract.CanonicalAgent{Name: "x", Model: "anything"}
	if f := g.modelFindings(a); len(f) != 0 {
		t.Fatalf("unavailable model list must skip silently, got %+v", f)
	}
}

func TestModelFindingsSkipsProviderWithoutLister(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"cursor": fakeProviderNoLister{name: "cursor"},
	})
	a := contract.CanonicalAgent{Name: "x", Model: "anything"}
	if f := g.modelFindings(a); len(f) != 0 {
		t.Fatalf("provider without ModelLister must be skipped, got %+v", f)
	}
}

func TestModelFindingsPerProviderOverride(t *testing.T) {
	// The override model is checked against that provider's list, not the default.
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet"}},
	})
	a := contract.CanonicalAgent{
		Name:  "x",
		Model: "sonnet", // default OK
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"model": "bad-override"},
		},
	}
	findings := g.modelFindings(a)
	if len(findings) != 1 || findings[0].Message == "" {
		t.Fatalf("override model should be flagged: %+v", findings)
	}
}

func TestModelFindingsRestrictedToEnabledProviders(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet"}},
		"codex":       fakeProvider{name: "codex", models: []string{"gpt-5"}},
	})
	// Only claude-code enabled -> codex's bad model is NOT checked.
	g.SetEnabledProviders([]string{"claude-code"})
	a := contract.CanonicalAgent{Name: "x", Model: "sonnet"} // ok for claude-code, unknown for codex
	if f := g.modelFindings(a); len(f) != 0 {
		t.Fatalf("disabled provider should not be model-checked, got %+v", f)
	}

	// Enable codex too -> now flagged for codex.
	g.SetEnabledProviders([]string{"claude-code", "codex"})
	f := g.modelFindings(a)
	if len(f) != 1 || f[0].Provider != "codex" {
		t.Fatalf("codex model should be flagged once enabled: %+v", f)
	}
}

func TestModelFindingsNoModelDeclared(t *testing.T) {
	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet"}},
	})
	a := contract.CanonicalAgent{Name: "x"} // no model
	if f := g.modelFindings(a); len(f) != 0 {
		t.Fatalf("no model declared -> nothing to check, got %+v", f)
	}
}

// writeCanonical persists a canonical agent under <root>/.graft/agents/<name>/.
func writeCanonical(t *testing.T, root string, a contract.CanonicalAgent) {
	t.Helper()
	writes, err := canonical.Save(root, a)
	if err != nil {
		t.Fatalf("canonical.Save: %v", err)
	}
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(w.Path, w.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestValidateAgentsModelWarningDoesNotBlock confirms an unknown-model warning
// flows through validateAgents but is NOT an error-severity finding, so the
// pre-sync gate (errorFindings) would not block on it.
func TestValidateAgentsModelWarningDoesNotBlock(t *testing.T) {
	root := t.TempDir()
	// A schema-valid agent (name + description + body) with an unknown model.
	a := contract.CanonicalAgent{
		Name:        "rev",
		Description: "Reviews code changes for correctness.",
		Model:       "totally-made-up-model",
		Body:        "You are a reviewer.",
	}
	writeCanonical(t, root, a)

	g := newFakeGate(map[string]contract.Provider{
		"claude-code": fakeProvider{name: "claude-code", models: []string{"sonnet", "opus"}},
	})
	g.root = root

	findings, err := g.validateAgents([]string{"rev"})
	if err != nil {
		t.Fatalf("validateAgents: %v", err)
	}
	// Exactly one model warning, no error-severity findings.
	var warnings, errs int
	for _, f := range findings {
		switch f.Severity {
		case "warning":
			warnings++
		case "error":
			errs++
		}
	}
	if warnings == 0 {
		t.Fatalf("expected a model warning, got: %+v", findings)
	}
	if errs != 0 {
		t.Fatalf("schema-valid agent should have no error findings: %+v", findings)
	}
	if blocking := errorFindings(findings); len(blocking) != 0 {
		t.Fatalf("model warning must NOT block the pre-sync gate: %+v", blocking)
	}
}
