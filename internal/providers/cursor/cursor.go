// Package cursor implements contract.Provider for Cursor subagents. The
// on-disk format is a Markdown file with YAML frontmatter under
// .cursor/agents/<name>.md; the Markdown body is the system prompt.
//
// Native shape is modeled by cursorFile. Canonical mapping (lossless): name
// (defaults to filename), description, and model map to canonical fields; the
// body maps to CanonicalAgent.Body. Cursor has no tools list (access is gated
// by the readonly bool); readonly and is_background travel under
// ProviderOverrides["cursor"].
package cursor

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

const name = "cursor"

// cursorFile models the native Cursor agent frontmatter.
type cursorFile struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	Model       string `yaml:"model,omitempty"`
	Body        string `yaml:"-"`
}

var knownKeys = []string{"name", "description", "model"}

// Provider implements contract.Provider for Cursor.
type Provider struct{}

// New returns a Cursor provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Cursor agent files under root (.cursor/agents/*.md).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".cursor", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cursor: detect: %w", err)
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
		return contract.ProviderAgent{}, fmt.Errorf("cursor: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	var cf cursorFile
	if err := fmark.Decode(fmBytes, &cf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("cursor: %w", err)
	}
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("cursor: %w", err)
	}
	nm := cf.Name
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), ".md")
	}
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
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"])),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .cursor/agents/<name>.md
// file, restoring overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	fm.Set("name", a.Name)
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}
	povr.Restore(fm, a.ProviderOverrides[name])

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("cursor: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".cursor", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
