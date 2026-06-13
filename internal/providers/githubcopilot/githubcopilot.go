// Package githubcopilot implements contract.Provider for GitHub Copilot custom
// agents. The on-disk format is a Markdown file with YAML frontmatter under
// .github/agents/<name>.agent.md; the Markdown body is the agent's system
// prompt.
//
// Native shape is modeled by copilotFile. Canonical mapping (lossless): name
// (defaults to filename), description, tools, and a string-form model map to
// canonical fields; the body maps to CanonicalAgent.Body. An array-form model
// and every other key (agents, target, disable-model-invocation,
// user-invocable, mcp-servers, handoffs, argument-hint, hooks, metadata, ...)
// travel under ProviderOverrides["github-copilot"].
package githubcopilot

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/fmark"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "github-copilot"

const fileSuffix = ".agent.md"

// copilotFile models the canonical-relevant subset of the native frontmatter.
// tools may be a YAML list or comma-separated string; model may be a string or
// a prioritized array (the array form is preserved through overrides).
type copilotFile struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	Body        string `yaml:"-"`
}

// knownKeys lists keys that have a canonical home (model only when it is a
// string — handled explicitly in ToCanonical/Serialize).
var knownKeys = []string{"name", "description", "tools", "model"}

// Provider implements contract.Provider for GitHub Copilot.
type Provider struct{}

// New returns a GitHub Copilot provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Copilot agent files under root (.github/agents/*.agent.md).
// Only files ending in ".agent.md" are matched — consistent with the suffix
// Serialize writes — so a plain "foo.md" is never picked up, and Detect+Serialize
// always round-trip to the same filename with no duplicates.
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".github", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("githubcopilot: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), fileSuffix) {
			continue
		}
		base := strings.TrimSuffix(e.Name(), fileSuffix)
		refs = append(refs, contract.AgentRef{
			Name:     base,
			Provider: name,
			Path:     filepath.Join(dir, e.Name()),
		})
	}
	return refs, nil
}

// Parse decodes one agent file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("githubcopilot: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	var cf copilotFile
	if err := fmark.Decode(fmBytes, &cf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("githubcopilot: %w", err)
	}
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("githubcopilot: %w", err)
	}
	nm := cf.Name
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), fileSuffix)
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Body:     body,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed agent into canonical form. The model field is
// mapped to canonical only when it is a plain string; an array-form model is
// preserved as an override.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"])),
		Description: povr.String(p.Fields["description"]),
		Tools:       povr.StringSlice(p.Fields["tools"]),
		Body:        p.Body,
	}
	known := knownKeys
	if m, ok := p.Fields["model"].(string); ok {
		ca.Model = m
	} else if _, present := p.Fields["model"]; present {
		// array/other form: keep model as an override (drop it from known).
		known = []string{"name", "description", "tools"}
	}
	if ov := povr.Extras(p.Fields, known); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a
// .github/agents/<name>.agent.md file, restoring overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	fm.Set("name", a.Name)
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}
	if len(a.Tools) > 0 {
		fm.Set("tools", a.Tools)
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields.
	// "name" is protected so agent identity is never overridden.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("githubcopilot: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".github", "agents", a.Name+fileSuffix)
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
