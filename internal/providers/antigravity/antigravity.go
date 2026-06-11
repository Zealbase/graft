// Package antigravity implements contract.Provider for Google Antigravity CLI
// file-based custom subagents. The on-disk format is a JSON file
// (agent.json) under ~/.gemini/antigravity-cli/agents/<name>/agent.json. There
// is no Markdown body: the system prompt is carried inside
// config.customAgent.systemPromptSections.
//
// Native shape is modeled by agentFile (with a free-form Config map to preserve
// the nested customAgent structure verbatim). Canonical mapping (lossless):
// name and description map to canonical fields. Because the system prompt and
// tool allowlist live inside the nested `config` object, that whole object —
// plus `hidden` — is preserved under ProviderOverrides["antigravity"] rather
// than flattened. CanonicalAgent.Body and Tools are therefore empty for this
// provider; see the package state note.
package antigravity

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "antigravity"

// agentFile models the native agent.json. Config holds the nested customAgent
// configuration (systemPromptSections, toolNames, systemPromptConfig, ...)
// verbatim so it round-trips losslessly.
type agentFile struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Hidden      bool           `json:"hidden,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
}

// knownKeys lists JSON keys with a canonical home.
var knownKeys = []string{"name", "description"}

// Provider implements contract.Provider for Antigravity.
type Provider struct{}

// New returns an Antigravity provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Antigravity agent.json files under root
// (.gemini/antigravity-cli/agents/<name>/agent.json).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".gemini", "antigravity-cli", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("antigravity: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name(), "agent.json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		refs = append(refs, contract.AgentRef{Name: e.Name(), Provider: name, Path: p})
	}
	return refs, nil
}

// Parse decodes one agent.json into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("antigravity: read %s: %w", path, err)
	}
	var af agentFile
	if err := json.Unmarshal(raw, &af); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("antigravity: decode %s: %w", path, err)
	}
	all := map[string]any{}
	if err := json.Unmarshal(raw, &all); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("antigravity: decode map %s: %w", path, err)
	}
	nm := af.Name
	if nm == "" {
		nm = filepath.Base(filepath.Dir(path))
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed agent into canonical form, preserving the nested
// config object and hidden flag under overrides.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"])),
		Description: povr.String(p.Fields["description"]),
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into an agent.json file, restoring
// overrides. Output uses insertion-ordered keys (name, description, then sorted
// overrides) and indented JSON for diff-friendliness.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	doc := omap.New()
	doc.Set("name", a.Name)
	if a.Description != "" {
		doc.Set("description", a.Description)
	}
	povr.Restore(doc, a.ProviderOverrides[name])

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("antigravity: encode: %w", err)
	}
	path := filepath.Join(".gemini", "antigravity-cli", "agents", a.Name, "agent.json")
	return []contract.FileWrite{{Path: path, Data: buf.Bytes()}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
