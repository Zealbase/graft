// Package continueprov implements contract.Provider for Continue (continue.dev)
// agents. The on-disk format is a Markdown file with a YAML frontmatter block
// under .continue/agents/<name>.md. The Markdown body is the agent's system
// prompt.
package continueprov

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

const name = "continue"

type continueFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Model       string `yaml:"model,omitempty"`
	Tools       string `yaml:"tools,omitempty"`
	Body        string `yaml:"-"`
}

var knownKeys = []string{"name", "description", "model", "tools"}

// Provider implements contract.Provider for Continue.
type Provider struct{}

// New returns a Continue provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns Continue agent files under root (.continue/agents/*.md) and
// the user home directory (~/.continue/agents/*.md).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	var refs []contract.AgentRef

	projDir := filepath.Join(root, ".continue", "agents")
	if entries, err := os.ReadDir(projDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			refs = append(refs, contract.AgentRef{
				Name:     strings.TrimSuffix(e.Name(), ".md"),
				Provider: name,
				Path:     filepath.Join(projDir, e.Name()),
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("continueprov: detect: %w", err)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		homeDir := filepath.Join(home, ".continue", "agents")
		if entries, err := os.ReadDir(homeDir); err == nil {
			for _, e := range entries {
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

// Parse decodes one .md file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("continueprov: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	var cf continueFile
	if err := fmark.Decode(fmBytes, &cf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("continueprov: %w", err)
	}
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("continueprov: %w", err)
	}
	cf.Body = body

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

// ToCanonical maps the parsed agent into canonical form, stashing all
// non-canonical frontmatter keys under ProviderOverrides["continue"].
//
// Tool routing: tokens that have a clean entry in toolMap (plain native names
// like "Read", "Bash") are translated to canonical names and placed in
// ca.Tools. Tokens that do NOT map — constrained forms like "Bash(git diff:*)"
// and MCP hub slugs like "org/pkg:tool" — are preserved verbatim under the
// synthetic key "_passthrough_tools" in ProviderOverrides["continue"]. Using a
// dedicated key (rather than "tools") avoids the po-continue schema validation
// on that field while still surviving a lossless round-trip.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	cf := continueFile{
		Name:        povr.String(p.Fields["name"]),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Tools:       povr.String(p.Fields["tools"]),
	}

	// Split tool tokens into mappable built-ins vs. constrained/MCP tokens.
	var canonicalTools []string
	var passthroughTools []string
	for _, tok := range commaList(cf.Tools) {
		if _, ok := toolMap.CanonicalTool(tok); ok {
			canonicalTools = append(canonicalTools, toolMap.MapToCanonical([]string{tok})[0])
		} else {
			passthroughTools = append(passthroughTools, tok)
		}
	}

	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, cf.Name),
		Description: cf.Description,
		Model:       cf.Model,
		Tools:       canonicalTools,
		Body:        p.Body,
	}

	// Build overrides: extra frontmatter keys + constrained/MCP tool tokens.
	// Passthrough tokens are stashed under "_passthrough_tools" (not "tools")
	// so they bypass the po-continue schema validation on the "tools" property.
	ov := povr.Extras(p.Fields, knownKeys)
	if len(passthroughTools) > 0 {
		if ov == nil {
			ov = map[string]any{}
		}
		ov["_passthrough_tools"] = passthroughTools
	}
	if len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .continue/agents/<name>.md
// file, restoring overrides.
//
// Tool reconstruction: canonical tools are mapped back to native names, then
// any constrained/MCP tokens stashed in ProviderOverrides["continue"]["_passthrough_tools"]
// are appended verbatim to preserve the original order and form.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	fm.Set("name", a.Name)
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("model", m)
	}

	// Reconstruct the full tools list: canonical-mapped natives + passthrough tokens.
	var allTools []string
	if len(a.Tools) > 0 {
		allTools = append(allTools, toolMap.MapToNative(a.Tools)...)
	}
	if pt := povr.StringSlice(a.ProviderOverrides[name]["_passthrough_tools"]); len(pt) > 0 {
		allTools = append(allTools, pt...)
	}
	if len(allTools) > 0 {
		fm.Set("tools", strings.Join(allTools, ", "))
	}

	// Restore remaining overrides, skipping internal stash keys and "name".
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true, "_passthrough_tools": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("continueprov: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".continue", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

// commaList splits a comma-separated tools string into a trimmed slice.
func commaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
