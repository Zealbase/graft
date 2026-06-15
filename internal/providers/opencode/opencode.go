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
	"sort"
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
// identity is the filename). `tools` is handled explicitly (split into the
// canonical Tools list for the enabled entries, plus a preserved set of
// disabled entries) so it is excluded from the generic overrides bucket.
var knownKeys = []string{"description", "model", "permission", "tools"}

// disabledToolsKey is the ProviderOverrides["opencode"] key under which the
// native tool names that were explicitly DISABLED (mapped to false in the
// `tools:` object) are preserved across a canonical round-trip. The enabled
// tools travel through the canonical Tools list; the disabled ones have no
// canonical home (canonical Tools is an allow-list), so they are stashed here
// and re-emitted on Serialize. The key is private (underscore-prefixed) so it
// never collides with a real opencode frontmatter field.
const disabledToolsKey = "_opencode_disabled_tools"

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
	// `tools:` is an object map of nativeName -> bool (true = enabled). Enabled
	// tools become the canonical Tools allow-list (mapped native->canonical);
	// explicitly-disabled tools have no canonical home, so they are preserved
	// under a private overrides key for a lossless round-trip.
	enabled, disabled := splitTools(p.Fields["tools"])
	if len(enabled) > 0 {
		ca.Tools = toolMap.MapToCanonical(enabled)
	}
	ov := povr.Extras(p.Fields, knownKeys)
	if len(disabled) > 0 {
		if ov == nil {
			ov = map[string]any{}
		}
		ov[disabledToolsKey] = disabled
	}
	if len(ov) > 0 {
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
	// Render the `tools:` object map (nativeName -> bool). Enabled tools come
	// from the canonical Tools list (mapped canonical->native); disabled tools
	// are restored from the private overrides key. The map is omitted entirely
	// when empty so agents without a tools field stay byte-identical.
	if toolsMap := buildToolsMap(a.Tools, a.ProviderOverrides[name]); toolsMap != nil {
		fm.Set("tools", toolsMap)
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields
	// (description, model, permission). "name" is protected — opencode uses the
	// filename as agent identity, so a "name" override must be ignored. The
	// private disabled-tools key is protected too — it is consumed by
	// buildToolsMap above and must never be emitted as a literal frontmatter key.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name],
		map[string]bool{"name": true, disabledToolsKey: true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".opencode", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

// splitTools reads the native `tools:` object map (nativeName -> bool) into two
// sorted slices of native names: those explicitly enabled (true) and those
// explicitly disabled (false). A non-bool value is treated as enabled (opencode
// historically allowed truthy strings). Returns nils when the field is absent.
func splitTools(v any) (enabled, disabled []string) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, nil
	}
	for k, raw := range m {
		if b, ok := raw.(bool); ok && !b {
			disabled = append(disabled, k)
			continue
		}
		enabled = append(enabled, k)
	}
	sort.Strings(enabled)
	sort.Strings(disabled)
	return enabled, disabled
}

// buildToolsMap reconstructs the native `tools:` object map from the canonical
// Tools allow-list (mapped canonical->native, value true) plus any preserved
// disabled native tools (value false) stashed under disabledToolsKey. Keys are
// emitted in sorted order for deterministic output. Returns an ordered map ready
// to set on the frontmatter, or nil when there are no tool entries.
func buildToolsMap(canonicalTools []string, overrides map[string]any) *omap.OMap {
	type entry struct {
		name    string
		enabled bool
	}
	seen := map[string]bool{}
	var entries []entry
	for _, native := range toolMap.MapToNative(canonicalTools) {
		if seen[native] {
			continue
		}
		seen[native] = true
		entries = append(entries, entry{name: native, enabled: true})
	}
	for _, native := range coerceStringSlice(overrides[disabledToolsKey]) {
		if seen[native] {
			continue // an enabled tool wins over a stale disabled marker
		}
		seen[native] = true
		entries = append(entries, entry{name: native, enabled: false})
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	out := omap.New()
	for _, e := range entries {
		out.Set(e.name, e.enabled)
	}
	return out
}

// coerceStringSlice coerces a decoded YAML/JSON value into a []string. It
// accepts both []string and []any (the shape after a YAML decode round-trip).
func coerceStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
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
