// Package opencode implements contract.Provider for opencode agents. The
// on-disk format is a Markdown file with YAML frontmatter under
// .opencode/agents/<name>.md; the agent id/name is the filename. The Markdown
// body is the system prompt (unless a `prompt` field overrides it).
//
// Native shape is modeled by opencodeFile. Canonical mapping (lossless): name
// (from filename), description, model map to canonical fields; the body maps to
// CanonicalAgent.Body; the per-tool `permission` string map maps to canonical
// Permissions. Other keys (mode, temperature, top_p, steps, prompt, tools,
// disable, hidden, color, ...) travel under ProviderOverrides["opencode"].
package opencode

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

const name = "opencode"

// opencodeFile models the canonical-relevant subset of the native frontmatter.
type opencodeFile struct {
	Description string            `yaml:"description,omitempty"`
	Model       string            `yaml:"model,omitempty"`
	Permission  map[string]string `yaml:"permission,omitempty"`
	Body        string            `yaml:"-"`
}

// knownKeys lists keys with a canonical home (opencode has no `name` key —
// identity is the filename).
var knownKeys = []string{"description", "model", "permission"}

// Provider implements contract.Provider for opencode.
type Provider struct{}

// New returns an opencode provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the opencode agent files under root (.opencode/agents/*.md).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".opencode", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opencode: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		refs = append(refs, contract.AgentRef{
			Name:     strings.TrimSuffix(e.Name(), ".md"),
			Provider: name,
			Path:     filepath.Join(dir, e.Name()),
		})
	}
	return refs, nil
}

// Parse decodes one .md file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("opencode: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("opencode: %w", err)
	}
	nm := strings.TrimSuffix(filepath.Base(path), ".md")
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Body:     body,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed agent into canonical form.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        p.Ref.Name,
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Permissions: stringMap(p.Fields["permission"]),
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .opencode/agents/<name>.md
// file, restoring overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}
	if len(a.Permissions) > 0 {
		fm.Set("permission", a.Permissions)
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields
	// (description, model, permission). "name" is protected — opencode uses the
	// filename as agent identity, so a "name" override must be ignored.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".opencode", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

// stringMap coerces a decoded value into map[string]string (permission map).
func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
