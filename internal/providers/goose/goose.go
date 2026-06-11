// Package goose implements contract.Provider for Block Goose recipes. The
// on-disk format is a structured YAML document (recipe.yaml); there is no
// Markdown body — the system-prompt-like text lives in the `instructions`
// string field.
//
// Native shape is modeled by recipe (with a Rest map preserving unmodeled
// keys). Canonical mapping (lossless): title maps to the canonical name,
// description maps directly, and instructions maps to CanonicalAgent.Body.
// The model lives nested under settings.goose_model, so the whole `settings`
// object — and every other key (version, prompt, activities, extensions,
// parameters, sub_recipes, retry, response, ...) — travels under
// ProviderOverrides["goose"]. CanonicalAgent.Model is empty for this provider;
// see the package state note.
package goose

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "goose"

// knownKeys lists recipe keys with a canonical home.
var knownKeys = []string{"title", "description", "instructions"}

// Provider implements contract.Provider for Goose.
type Provider struct{}

// New returns a Goose provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns recipe YAML files under root (recipe.yaml and *.yaml).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("goose: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || (filepath.Ext(e.Name()) != ".yaml" && filepath.Ext(e.Name()) != ".yml") {
			continue
		}
		p := filepath.Join(root, e.Name())
		nm, ok := recipeName(p)
		if !ok {
			continue
		}
		refs = append(refs, contract.AgentRef{Name: nm, Provider: name, Path: p})
	}
	return refs, nil
}

// recipeName reads a recipe's title (its canonical name); ok is false if the
// file is not a recognizable recipe.
func recipeName(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	t := povr.String(m["title"])
	if t == "" {
		return "", false
	}
	// Require `instructions` too, so an unrelated root YAML that merely has a
	// `title` key (mkdocs.yml, a compose file, …) isn't mis-detected as a recipe.
	if povr.String(m["instructions"]) == "" {
		return "", false
	}
	return t, true
}

// Parse decodes one recipe file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("goose: read %s: %w", path, err)
	}
	all := map[string]any{}
	if err := yaml.Unmarshal(raw, &all); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("goose: decode %s: %w", path, err)
	}
	nm := povr.String(all["title"])
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Body:     povr.String(all["instructions"]),
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed recipe into canonical form.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["title"])),
		Description: povr.String(p.Fields["description"]),
		Body:        povr.String(p.Fields["instructions"]),
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a recipe YAML file, restoring
// overrides. Output field order: title, description, instructions, then sorted
// overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	doc := omap.New()
	doc.Set("title", a.Name)
	if a.Description != "" {
		doc.Set("description", a.Description)
	}
	if a.Body != "" {
		doc.Set("instructions", a.Body)
	}
	povr.Restore(doc, a.ProviderOverrides[name])

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("goose: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("goose: encode: %w", err)
	}
	path := a.Name + ".yaml"
	return []contract.FileWrite{{Path: path, Data: buf.Bytes()}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
