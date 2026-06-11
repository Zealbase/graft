// Package codex implements contract.Provider for OpenAI Codex subagents. The
// on-disk format is a TOML file under .codex/agents/<name>.toml. There is no
// separate Markdown body: the system prompt lives in the developer_instructions
// string field.
//
// Native shape is modeled by codexFile. Canonical mapping (lossless): name,
// description, model map to canonical fields; developer_instructions maps to
// CanonicalAgent.Body. Other keys (model_reasoning_effort, sandbox_mode,
// nickname_candidates, mcp_servers, skills, ...) travel under
// ProviderOverrides["codex"].
package codex

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "codex"

// codexFile models the canonical-relevant subset of the native TOML.
type codexFile struct {
	Name                  string `toml:"name"`
	Description           string `toml:"description"`
	DeveloperInstructions string `toml:"developer_instructions"`
	Model                 string `toml:"model"`
}

// knownKeys lists the TOML keys with a canonical home.
var knownKeys = []string{"name", "description", "developer_instructions", "model"}

// Provider implements contract.Provider for Codex.
type Provider struct{}

// New returns a Codex provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Codex agent files under root (.codex/agents/*.toml).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".codex", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("codex: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		refs = append(refs, contract.AgentRef{
			Name:     strings.TrimSuffix(e.Name(), ".toml"),
			Provider: name,
			Path:     filepath.Join(dir, e.Name()),
		})
	}
	return refs, nil
}

// Parse decodes one .toml file into a ProviderAgent. It decodes into the typed
// struct (format contract) and into a generic map (lossless extras capture).
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("codex: read %s: %w", path, err)
	}
	var cf codexFile
	if _, err := toml.Decode(string(raw), &cf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("codex: decode %s: %w", path, err)
	}
	all := map[string]any{}
	if _, err := toml.Decode(string(raw), &all); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("codex: decode map %s: %w", path, err)
	}
	nm := cf.Name
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), ".toml")
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Body:     cf.DeveloperInstructions,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed agent into canonical form.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"])),
		Description: povr.String(p.Fields["description"]),
		Model:       povr.String(p.Fields["model"]),
		Body:        povr.String(p.Fields["developer_instructions"]),
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .codex/agents/<name>.toml
// file, restoring overrides. The TOML encoder emits keys in sorted order, so
// output is deterministic.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	doc := map[string]any{
		"name": a.Name,
	}
	if a.Description != "" {
		doc["description"] = a.Description
	}
	if a.Model != "" {
		doc["model"] = a.Model
	}
	if a.Body != "" {
		doc["developer_instructions"] = a.Body
	}
	for k, v := range a.ProviderOverrides[name] {
		if _, exists := doc[k]; !exists {
			doc[k] = v
		}
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return nil, fmt.Errorf("codex: encode: %w", err)
	}
	path := filepath.Join(".codex", "agents", a.Name+".toml")
	return []contract.FileWrite{{Path: path, Data: buf.Bytes()}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
