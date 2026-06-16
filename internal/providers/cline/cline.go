// Package clineprov implements contract.Provider for Cline agents. The
// on-disk format is a YAML file with frontmatter + Markdown body under
// .cline/agents/<name>.yaml (or .yml). The Markdown body is the agent's
// system prompt.
package clineprov

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

const name = "cline"

// knownKeys are the frontmatter keys with a canonical home; all others travel
// through ProviderOverrides["cline"].
var knownKeys = []string{"name", "description", "modelId", "tools", "skills"}

// Provider implements contract.Provider for Cline.
type Provider struct{}

// New returns a Cline provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's embedded JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns Cline agent files under root (.cline/agents/*.yaml|*.yml) and
// under the user home directory (~/.cline/agents/*.yaml|*.yml).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	var refs []contract.AgentRef

	projDir := filepath.Join(root, ".cline", "agents")
	if entries, err := os.ReadDir(projDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
				continue
			}
			agentName := strings.TrimSuffix(strings.TrimSuffix(n, ".yaml"), ".yml")
			refs = append(refs, contract.AgentRef{
				Name:     agentName,
				Provider: name,
				Path:     filepath.Join(projDir, n),
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("clineprov: detect project: %w", err)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		homeDir := filepath.Join(home, ".cline", "agents")
		if entries, err := os.ReadDir(homeDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				n := e.Name()
				if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
					continue
				}
				agentName := strings.TrimSuffix(strings.TrimSuffix(n, ".yaml"), ".yml")
				refs = append(refs, contract.AgentRef{
					Name:     agentName,
					Provider: name,
					Path:     filepath.Join(homeDir, n),
				})
			}
		}
	}

	return refs, nil
}

// Parse decodes one .yaml/.yml file into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("clineprov: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("clineprov: %w", err)
	}

	nm := povr.String(all["name"])
	if nm == "" {
		base := filepath.Base(path)
		nm = strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
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
// non-canonical frontmatter keys under ProviderOverrides["cline"].
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	nm := firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"]))
	description := povr.String(p.Fields["description"])
	model := povr.String(p.Fields["modelId"])

	tools := toolMap.MapToCanonical(anyToStringSlice(p.Fields["tools"]))
	skills := anyToStringSlice(p.Fields["skills"])

	ca := contract.CanonicalAgent{
		Name:        nm,
		Description: description,
		Model:       model,
		Tools:       tools,
		Skills:      skills,
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .cline/agents/<name>.yaml
// file, restoring overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	fm := omap.New()
	fm.Set("name", a.Name)
	if a.Description != "" {
		fm.Set("description", a.Description)
	}
	if m := a.ModelFor(name); m != "" {
		fm.Set("modelId", m)
	}
	if len(a.Tools) > 0 {
		fm.Set("tools", toolMap.MapToNative(a.Tools))
	}
	if len(a.Skills) > 0 {
		fm.Set("skills", a.Skills)
	}
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("clineprov: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".cline", "agents", a.Name+".yaml")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

// anyToStringSlice coerces a decoded YAML value to []string.
func anyToStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
