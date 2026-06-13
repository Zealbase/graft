// Package geminicli implements contract.Provider for Google Gemini CLI
// subagents. The on-disk format is a Markdown file with YAML frontmatter under
// .gemini/agents/<name>.md; the Markdown body is the agent's system prompt.
//
// Native shape is modeled by geminiFile. Canonical mapping (lossless): name,
// description, model, and the `tools` list map to canonical fields; the body
// maps to CanonicalAgent.Body. Other keys (kind, mcpServers, temperature,
// max_turns, timeout_mins, ...) travel under ProviderOverrides["gemini-cli"].
package geminicli

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

const name = "gemini-cli"

// geminiFile models the native Gemini CLI agent frontmatter. Tools is a YAML
// list of strings on disk.
type geminiFile struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Model       string   `yaml:"model,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
	Body        string   `yaml:"-"`
}

var knownKeys = []string{"name", "description", "model", "tools"}

// Provider implements contract.Provider for Gemini CLI.
type Provider struct{}

// New returns a Gemini CLI provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Gemini CLI agent files under root (.gemini/agents/*.md).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".gemini", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("geminicli: detect: %w", err)
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
		return contract.ProviderAgent{}, fmt.Errorf("geminicli: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	var gf geminiFile
	if err := fmark.Decode(fmBytes, &gf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("geminicli: %w", err)
	}
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("geminicli: %w", err)
	}
	nm := gf.Name
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
		Tools:       povr.StringSlice(p.Fields["tools"]),
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .gemini/agents/<name>.md
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
	if len(a.Tools) > 0 {
		fm.Set("tools", a.Tools)
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields.
	// "name" is protected so agent identity is never overridden.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("geminicli: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".gemini", "agents", a.Name+".md")
	return []contract.FileWrite{{Path: path, Data: data}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
