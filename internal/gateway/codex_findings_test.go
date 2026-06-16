package gateway

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

func TestCodexToolFindingsWarning(t *testing.T) {
	g := &gate{tr: transform.Default()}

	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Tools:       []string{"bash", "read_file"},
	}
	findings := g.codexToolFindings(a)
	if len(findings) != 1 {
		t.Fatalf("expected 1 warning finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.Severity != "warning" {
		t.Errorf("expected severity=warning, got %q", f.Severity)
	}
	if f.Provider != "codex" {
		t.Errorf("expected provider=codex, got %q", f.Provider)
	}
	// Warning should not be in errorFindings (must not block sync)
	if errs := errorFindings(findings); len(errs) != 0 {
		t.Errorf("codex tool warning should be non-blocking; got error findings: %+v", errs)
	}
}

func TestCodexToolFindingsNoToolsNoWarning(t *testing.T) {
	g := &gate{tr: transform.Default()}

	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Tools:       nil,
	}
	findings := g.codexToolFindings(a)
	if len(findings) != 0 {
		t.Errorf("expected no findings when agent has no tools, got: %+v", findings)
	}
}

func TestCodexToolFindingsDisabledProvider(t *testing.T) {
	// When codex is NOT in enabledProviders, no warning.
	g := &gate{
		tr:               transform.Default(),
		enabledProviders: []string{"claude-code", "cursor"},
	}

	a := contract.CanonicalAgent{
		Name:        "my-agent",
		Description: "Does something useful.",
		Tools:       []string{"bash"},
	}
	findings := g.codexToolFindings(a)
	if len(findings) != 0 {
		t.Errorf("expected no warning when codex not in enabledProviders, got: %+v", findings)
	}
}
