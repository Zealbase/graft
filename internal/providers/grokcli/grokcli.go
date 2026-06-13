// Package grokcli implements contract.Provider for the community
// superagent-ai/grok-cli subagents. Natively a subagent is a JSON object
// ({name, model, instruction}) inside the subAgents[] array of
// ~/.grok/user-settings.json. graft models one subagent per file as a single
// JSON object under .grok/agents/<name>.json (the canonical unit is one
// agent); a sync/merge layer is responsible for splicing these back into
// user-settings.json. See the package state note.
//
// Native shape is modeled by subAgent. Canonical mapping (lossless): name and
// model map directly, and instruction maps to CanonicalAgent.Body. Any extra
// keys travel under ProviderOverrides["grok-cli"].
package grokcli

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/povr"
)

//go:embed schema.json
var schema []byte

const name = "grok-cli"

// subAgent models the native grok-cli subagent JSON object.
type subAgent struct {
	Name        string `json:"name"`
	Model       string `json:"model,omitempty"`
	Instruction string `json:"instruction,omitempty"`
}

// knownKeys lists JSON keys with a canonical home.
var knownKeys = []string{"name", "model", "instruction"}

// Provider implements contract.Provider for grok-cli.
type Provider struct{}

// New returns a grok-cli provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the grok-cli subagent files under root (.grok/agents/*.json).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".grok", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("grokcli: detect: %w", err)
	}
	var refs []contract.AgentRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		refs = append(refs, contract.AgentRef{
			Name:     strings.TrimSuffix(e.Name(), ".json"),
			Provider: name,
			Path:     filepath.Join(dir, e.Name()),
		})
	}
	return refs, nil
}

// Parse decodes one subagent JSON object into a ProviderAgent.
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("grokcli: read %s: %w", path, err)
	}
	var sa subAgent
	if err := json.Unmarshal(raw, &sa); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("grokcli: decode %s: %w", path, err)
	}
	all := map[string]any{}
	if err := json.Unmarshal(raw, &all); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("grokcli: decode map %s: %w", path, err)
	}
	nm := sa.Name
	if nm == "" {
		nm = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	return contract.ProviderAgent{
		Provider: name,
		Ref:      contract.AgentRef{Name: nm, Provider: name, Path: path},
		Fields:   all,
		Body:     sa.Instruction,
		Raw:      raw,
	}, nil
}

// ToCanonical maps the parsed subagent into canonical form.
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	ca := contract.CanonicalAgent{
		Name:  firstNonEmpty(p.Ref.Name, povr.String(p.Fields["name"])),
		Model: povr.String(p.Fields["model"]),
		Body:  povr.String(p.Fields["instruction"]),
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .grok/agents/<name>.json
// subagent object, restoring overrides.
func (Provider) Serialize(a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	doc := omap.New()
	doc.Set("name", a.Name)
	if m := a.ModelFor(name); m != "" {
		doc.Set("model", m)
	}
	if a.Body != "" {
		doc.Set("instruction", a.Body)
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields.
	// "name" is protected so agent identity is never overridden.
	povr.RestoreOverrides(doc, a.ProviderOverrides[name], map[string]bool{"name": true})

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("grokcli: encode: %w", err)
	}
	path := filepath.Join(".grok", "agents", a.Name+".json")
	return []contract.FileWrite{{Path: path, Data: buf.Bytes()}}, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
