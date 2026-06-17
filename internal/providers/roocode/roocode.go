// Package roocode implements contract.Provider for Roo Code custom modes. The
// on-disk format is a YAML file (.roomodes) holding a customModes array of mode
// objects. graft models one mode per file (the canonical unit is a single
// agent), so a file's customModes array carries exactly one entry on
// round-trip.
//
// Native shape is modeled by mode. Canonical mapping (lossless): slug maps to
// the canonical name, description and model map directly, and roleDefinition
// maps to CanonicalAgent.Body. Other keys (name/display, whenToUse,
// customInstructions, groups, source, iconName, ...) travel under
// ProviderOverrides["roo-code"].
package roocode

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "roo-code"

// file is the top-level .roomodes document.
type file struct {
	CustomModes []map[string]any `yaml:"customModes"`
}

// knownKeys lists mode keys with a canonical home.
var knownKeys = []string{"slug", "description", "model", "roleDefinition"}

// Provider implements contract.Provider for Roo Code.
type Provider struct{}

// New returns a Roo Code provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the .roomodes file(s) under root. graft treats one mode file
// as one agent; the ref name is the mode slug.
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	p := filepath.Join(root, ".roomodes")
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("roocode: detect: %w", err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("roocode: detect parse: %w", err)
	}
	var refs []contract.AgentRef
	for _, m := range f.CustomModes {
		slug := povr.String(m["slug"])
		if slug == "" {
			continue // a mode without a slug has no stable agent identity
		}
		refs = append(refs, contract.AgentRef{Name: slug, Provider: name, Path: p})
	}
	return refs, nil
}

// Parse decodes the first mode of a .roomodes file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("roocode: read %s: %w", path, err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("roocode: decode %s: %w", path, err)
	}
	if len(f.CustomModes) == 0 {
		return contract.ProviderAgent{}, fmt.Errorf("roocode: %s has no customModes", path)
	}
	mode := f.CustomModes[0]
	nm := povr.String(mode["slug"])
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), ".roomodes")
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   mode,
		Body:     povr.String(mode["roleDefinition"]),
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed mode into canonical form.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["slug"])),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Body:        povr.String(p.Fields["roleDefinition"]),
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// deriveGroups translates canonical tool names from ca.Tools into the
// de-duplicated set of roo-code native group names. The mapping is:
//
//	read_file  → read
//	file_edit  → edit
//	bash       → command
//	mcp        → mcp
//
// Any canonical tool with no native equivalent is silently skipped. The result
// is sorted for deterministic output. If no tools map to known groups, an empty
// (non-nil) slice is returned so the required `groups` key is always emitted.
func deriveGroups(tools []string) []any {
	seen := make(map[string]bool, len(tools))
	p := Provider{}
	for _, canonical := range tools {
		if native, ok := p.NativeTool(canonical); ok {
			seen[native] = true
		}
	}
	// Stable output: emit in the canonical group order (read, edit, command, mcp).
	ordered := []string{"read", "edit", "command", "mcp"}
	out := make([]any, 0, len(seen))
	for _, g := range ordered {
		if seen[g] {
			out = append(out, g)
		}
	}
	return out
}

// Serialize renders the canonical agent back into a .roomodes file with a
// single-entry customModes array, restoring overrides.
//
// For canonical agents authored in another provider (no roo-code
// ProviderOverrides), Roo Code requires both `name` (display name) and
// `groups` (permission set) to be present in the emitted mode object.
// When they are absent from ProviderOverrides:
//   - `name` defaults to the canonical Name (matching the slug).
//   - `groups` is derived from ca.Tools via the tool mapper; if no tools map to
//     known roo-code groups, an empty array is emitted so the key is present.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	mode := omap.New()
	mode.Set("slug", a.Name)
	if a.Description != "" {
		mode.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		mode.Set("model", m)
	}
	if a.Body != "" {
		mode.Set("roleDefinition", a.Body)
	}

	ovr := a.ProviderOverrides[name]

	// Emit `groups` before RestoreOverrides so the override can win if present.
	// When the override does NOT carry `groups`, derive them from canonical Tools.
	if _, hasGroups := ovr["groups"]; !hasGroups {
		mode.Set("groups", deriveGroups(a.Tools))
	}

	// Emit `name` (display name) before RestoreOverrides. When the override does
	// NOT carry `name`, fall back to the canonical Name so the required key is
	// always present in the emitted mode.
	if _, hasName := ovr["name"]; !hasName {
		mode.Set("name", a.Name)
	}

	// RestoreOverrides lets providerOverrides[name] win over canonical fields
	// (description, model, roleDefinition, groups, name). "slug" is protected —
	// it is the roo-code identity key and maps to canonical name.
	povr.RestoreOverrides(mode, ovr, map[string]bool{"slug": true})

	top := omap.New()
	top.Set("customModes", []any{mode})

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(top); err != nil {
		return nil, fmt.Errorf("roocode: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("roocode: encode: %w", err)
	}
	path := ".roomodes"
	return []contract.FileWrite{{Path: path, Data: buf.Bytes()}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
