// Package kilocode implements contract.Provider for Kilo Code agents. It
// supports two on-disk formats:
//
// MODERN format (detect + parse + serialize target):
//   - Project: .kilo/agents/<name>.md
//   - Home: ~/.config/kilo/agent/<name>.md
//   - Markdown body = system prompt; YAML frontmatter holds description, model,
//     mode, color, steps, permission (allow/deny/ask arrays).
//
// LEGACY format (parse only — migrated to modern on Serialize):
//   - Files: .kilocodemodes and/or custom_modes.yaml in workspace root
//   - Structure: customModes array (slug, name, roleDefinition,
//     customInstructions, description, groups, whenToUse, source, iconName)
//
// Canonical mapping (lossless): name (filename / slug), description, model,
// permission.allow items (translated via toolMap) → canonical Tools. Full
// permission object + mode/color/steps and any unknown keys travel under
// ProviderOverrides["kilo-code"]. Serialize always writes modern format.
package kilocode

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/fmark"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

// name is the canonical provider id.
const name = "kilo-code"

// modernFile models the frontmatter of a modern .kilo/agents/<name>.md file.
type modernFile struct {
	Description string `yaml:"description,omitempty"`
	Model       string `yaml:"model,omitempty"`
	// Mode, Color, Steps, Permission travel as extra keys via DecodeMap.
}

// permission models the permission block in modern kilo-code frontmatter.
type permission struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
	Ask   []string `yaml:"ask"`
}

// legacyFile is the top-level .kilocodemodes / custom_modes.yaml document.
type legacyFile struct {
	CustomModes []map[string]any `yaml:"customModes"`
}

// knownKeys are the frontmatter keys with a canonical home in modern format.
// Everything else becomes ProviderOverrides["kilo-code"].
var knownKeys = []string{"description", "model"}

// Provider implements contract.Provider for Kilo Code.
type Provider struct{}

// New returns a Kilo Code provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns all kilo-code agent refs found under root (modern project +
// legacy) and under ~/.config/kilo/agent/ (modern home).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	var refs []contract.AgentRef

	// Modern project: .kilo/agents/*.md
	modernDir := filepath.Join(root, ".kilo", "agents")
	entries, err := os.ReadDir(modernDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("kilocode: detect modern: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		refs = append(refs, contract.AgentRef{
			Name:     strings.TrimSuffix(e.Name(), ".md"),
			Provider: name,
			Path:     filepath.Join(modernDir, e.Name()),
		})
	}

	// Legacy project: .kilocodemodes and custom_modes.yaml
	for _, fname := range []string{".kilocodemodes", "custom_modes.yaml"} {
		p := filepath.Join(root, fname)
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("kilocode: detect legacy %s: %w", fname, err)
		}
		var lf legacyFile
		if err := yaml.Unmarshal(raw, &lf); err != nil {
			continue // skip unparseable files
		}
		for _, m := range lf.CustomModes {
			slug := povr.String(m["slug"])
			if slug == "" {
				continue
			}
			refs = append(refs, contract.AgentRef{
				Name:     slug,
				Provider: name,
				Path:     p,
			})
		}
	}

	// Modern home: ~/.config/kilo/agent/*.md
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		homeDir := filepath.Join(home, ".config", "kilo", "agent")
		homeEntries, err := os.ReadDir(homeDir)
		if err == nil {
			for _, e := range homeEntries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				refs = append(refs, contract.AgentRef{
					Name:     strings.TrimSuffix(e.Name(), ".md"),
					Provider: name,
					Path:     filepath.Join(homeDir, e.Name()),
				})
			}
		}
	}

	return refs, nil
}

// Parse decodes one agent file into a ProviderAgent. Dispatch is by filename:
// *.md → modern parse; *.kilocodemodes or *.yaml → legacy parse.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".md") {
		return parseModern(path)
	}
	return parseLegacy(path)
}

// parseModern decodes a modern .kilo/agents/<name>.md file.
func parseModern(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("kilocode: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("kilocode: %w", err)
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

// parseLegacy decodes the first mode of a .kilocodemodes / custom_modes.yaml.
func parseLegacy(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("kilocode: read %s: %w", path, err)
	}
	var lf legacyFile
	if err := yaml.Unmarshal(raw, &lf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("kilocode: decode %s: %w", path, err)
	}
	if len(lf.CustomModes) == 0 {
		return contract.ProviderAgent{}, fmt.Errorf("kilocode: %s has no customModes", path)
	}
	mode := lf.CustomModes[0]
	nm := povr.String(mode["slug"])
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	// Body = roleDefinition + optional customInstructions
	body := povr.String(mode["roleDefinition"])
	if ci := povr.String(mode["customInstructions"]); ci != "" {
		body = body + "\n\n" + ci
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   mode,
		Body:     body,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed agent into canonical form.
// Modern: permission.allow → canonical Tools; full permission + mode/color/steps/unknowns → overrides.
// Legacy: slug → Name, roleDefinition(+customInstructions) → Body, rest → overrides.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	base := filepath.Base(p.Ref.Path)
	if strings.HasSuffix(base, ".md") {
		return toCanonicalModern(p)
	}
	return toCanonicalLegacy(p)
}

func toCanonicalModern(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	description := povr.String(p.Fields["description"])
	model := povr.String(p.Fields["model"])

	// Extract canonical tools from permission.allow
	var tools []string
	if perm, ok := p.Fields["permission"]; ok && perm != nil {
		if permMap, ok := perm.(map[string]any); ok {
			allow := povr.StringSlice(permMap["allow"])
			canonical := toolMap.MapToCanonical(allow)
			sort.Strings(canonical)
			tools = canonical
		}
	}

	ca := contract.CanonicalAgent{
		Name:        p.Ref.Name,
		Description: description,
		Model:       model,
		Tools:       tools,
		Body:        p.Body,
	}

	// Everything except description and model goes to overrides
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

func toCanonicalLegacy(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	// knownLegacyKeys: fields with canonical homes
	knownLegacyKeys := []string{"slug", "description", "model", "roleDefinition", "customInstructions"}
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["slug"])),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownLegacyKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent into a modern .kilo/agents/<name>.md
// file, restoring overrides. Canonical fields first, then overrides sorted.
// description and model are NOT overwritten by RestoreOverrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}
	// RestoreOverrides: override values WIN over canonical already written.
	// "name" is not a frontmatter field (identity is the filename) so no keys
	// need protecting — overrides for description/model win over canonical.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("kilocode: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".kilo", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
