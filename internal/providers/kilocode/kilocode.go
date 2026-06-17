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

// legacyPathSep is the separator used to encode slug into a legacy file path,
// allowing Parse to identify which mode to extract from a multi-mode file.
const legacyPathSep = "#"

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
			// Encode the slug into the path so Parse can find the right mode
			// when multiple modes live in the same file (e.g. .kilocodemodes).
			refs = append(refs, contract.AgentRef{
				Name:     slug,
				Provider: name,
				Path:     p + legacyPathSep + slug,
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
// Legacy paths may carry a "#<slug>" suffix (added by Detect) to identify
// which mode in a multi-mode file to return.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	// Strip the slug suffix before checking extension; extract the slug if present.
	filePath := path
	slug := ""
	if i := strings.LastIndex(path, legacyPathSep); i >= 0 {
		candidate := path[i+1:]
		before := path[:i]
		// Only treat as a slug suffix if the part before the separator is an
		// actual file path (not a path element containing the separator).
		if !strings.Contains(before, legacyPathSep) || filepath.IsAbs(before) {
			ext := filepath.Ext(before)
			if ext == ".kilocodemodes" || ext == ".yaml" || strings.HasSuffix(before, ".kilocodemodes") {
				filePath = before
				slug = candidate
			}
		}
	}
	base := filepath.Base(filePath)
	if strings.HasSuffix(base, ".md") {
		return parseModern(filePath)
	}
	return parseLegacy(filePath, slug)
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

// parseLegacy decodes a .kilocodemodes / custom_modes.yaml file. When slug is
// non-empty it finds the mode whose slug field matches; otherwise it falls back
// to the first mode. This makes parsing slug-aware so that Detect + Parse
// correctly round-trips multi-mode files (each slug gets its own mode).
func parseLegacy(path, slug string) (contract.ProviderAgent, error) {
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

	// Find the mode matching the requested slug; fall back to index 0.
	mode := lf.CustomModes[0]
	if slug != "" {
		for _, m := range lf.CustomModes {
			if povr.String(m["slug"]) == slug {
				mode = m
				break
			}
		}
	}

	nm := povr.String(mode["slug"])
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	// Body = roleDefinition + optional customInstructions
	body := povr.String(mode["roleDefinition"])
	if ci := povr.String(mode["customInstructions"]); ci != "" {
		body = body + "\n\n" + ci
	}
	// Store the encoded path (file#slug) in the Ref so ToCanonical/Serialize
	// dispatch works correctly; the raw path without slug is used for reads.
	refPath := path
	if slug != "" {
		refPath = path + legacyPathSep + slug
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: refPath},
		Fields:   mode,
		Body:     body,
		Raw:      raw,
	}, nil
}

// isLegacyPath reports whether the given file path (without any #slug suffix)
// is a legacy kilo file. This mirrors the guard used in Parse so that the two
// methods stay in sync: a path is legacy only when its extension is
// ".kilocodemodes" or ".yaml" (matching how Detect builds legacy refs).
func isLegacyPath(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || strings.HasSuffix(path, ".kilocodemodes")
}

// ToCanonical maps the parsed agent into canonical form.
// Modern: permission.allow → canonical Tools; full permission + mode/color/steps/unknowns → overrides.
// Legacy: slug → Name, roleDefinition(+customInstructions) → Body, groups → canonical Tools, rest → overrides.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	// Strip any "#slug" suffix, but ONLY when the portion before the separator
	// is actually a legacy file. Modern paths whose directory name contains "#"
	// must never be truncated (e.g. /home/user/my#project/.kilo/agents/foo.md).
	refPath := p.Ref.Path
	if i := strings.LastIndex(refPath, legacyPathSep); i >= 0 {
		before := refPath[:i]
		if isLegacyPath(before) {
			refPath = before
		}
	}
	base := filepath.Base(refPath)
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

// legacyGroupToCanonical maps legacy group names to canonical tool names.
// "browser" expands to two canonical tools; "mcp" has no canonical equivalent
// (skipped). These groups appear in .kilocodemodes / custom_modes.yaml.
var legacyGroupToCanonical = map[string][]string{
	"read":    {"read_file"},
	"edit":    {"file_edit"},
	"command": {"bash"},
	"browser": {"web_fetch", "web_search"},
	// mcp: no canonical equivalent — skip
}

func toCanonicalLegacy(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	// knownLegacyKeys: fields with canonical homes (groups is handled here, so also known)
	knownLegacyKeys := []string{"slug", "description", "model", "roleDefinition", "customInstructions", "groups"}
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["slug"])),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Body:        p.Body,
	}

	// Translate legacy groups → canonical tools so the serialized modern file
	// emits a permission block (tools are not silently lost on migration).
	if rawGroups, ok := p.Fields["groups"]; ok {
		groups := povr.StringSlice(rawGroups)
		seen := make(map[string]bool)
		var tools []string
		for _, g := range groups {
			for _, ct := range legacyGroupToCanonical[g] {
				if !seen[ct] {
					seen[ct] = true
					tools = append(tools, ct)
				}
			}
		}
		if len(tools) > 0 {
			sort.Strings(tools)
			ca.Tools = tools
		}
	}

	if ov := povr.Extras(p.Fields, knownLegacyKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent into a modern .kilo/agents/<name>.md
// file, restoring overrides. Canonical fields first, then overrides sorted.
// description and model are NOT overwritten by RestoreOverrides.
//
// Permission handling: if ProviderOverrides["kilo-code"]["permission"] exists it
// is the source of truth (lossless round-trip from a parsed kilo file). If it is
// absent (e.g. agent propagated from another provider), a permission block is
// derived from the canonical Tools slice so that tools are never silently dropped.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}

	// Determine whether the overrides already carry a permission block.
	kiloOvr := a.ProviderOverrides[name]
	_, hasPermOvr := kiloOvr["permission"]

	// RestoreOverrides: override values WIN over canonical already written.
	// "name" is not a frontmatter field (identity is the filename) so no keys
	// need protecting — overrides for description/model win over canonical.
	povr.RestoreOverrides(fm, kiloOvr, map[string]bool{"name": true})

	// If no permission came from overrides, derive one from canonical Tools.
	// This ensures that a canonical agent propagated from another provider (no
	// pre-existing kilo override) still gets a permission block emitted.
	if !hasPermOvr && len(a.Tools) > 0 {
		native := toolMap.MapToNative(a.Tools)
		sort.Strings(native)
		allow := make([]string, 0, len(native))
		for _, n := range native {
			allow = append(allow, n)
		}
		fm.Set("permission", map[string]any{
			"allow": allow,
			"deny":  []string{},
			"ask":   []string{},
		})
	}

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
