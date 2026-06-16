// Package claudecode implements contract.Provider for Anthropic Claude Code
// subagents. The on-disk format is a Markdown file with a YAML frontmatter
// block under .claude/agents/<name>.md. The Markdown body is the agent's
// system prompt.
//
// The native file shape is modeled by the typed claudeFile struct. Canonical
// mapping (lossless): name, description, model, the comma-separated `tools`
// field, and the `skills` array all map to first-class canonical fields; the
// Markdown body maps to CanonicalAgent.Body. Every other frontmatter key
// (disallowedTools, permissionMode, maxTurns, mcpServers, hooks, memory,
// background, effort, isolation, color, initialPrompt, ...) is preserved
// verbatim under ProviderOverrides["claude-code"] so a parse→serialize
// round-trip is lossless.
package claudecode

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

// name is the canonical provider id.
const name = "claude-code"

// claudeFile models the native Claude Code agent file's frontmatter. Tools is a
// comma-separated string on disk. Body is the Markdown document (not part of
// the frontmatter; carried separately).
type claudeFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Model       string `yaml:"model,omitempty"`
	Tools       string `yaml:"tools,omitempty"`
	Body        string `yaml:"-"`
}

// knownKeys are the frontmatter keys modeled by claudeFile (i.e. with a
// canonical home); all others travel through ProviderOverrides.
var knownKeys = []string{"name", "description", "model", "tools", "skills"}

// Provider implements contract.Provider for Claude Code.
type Provider struct{}

// New returns a Claude Code provider.
func New() *Provider { return &Provider{} }

// Name returns the canonical provider id.
func (Provider) Name() string { return name }

// Schema returns the provider's research JSON schema bytes.
func (Provider) Schema() []byte { return schema }

// Detect returns the Claude Code agent files under root (.claude/agents/*.md).
func (Provider) Detect(root string) ([]contract.AgentRef, error) {
	dir := filepath.Join(root, ".claude", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("claudecode: detect: %w", err)
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

// Parse decodes one .md file into a ProviderAgent. It decodes the frontmatter
// into the typed claudeFile struct (format contract) and into a generic map
// (to capture any extra keys for lossless overrides).
func (Provider) Parse(path string) (contract.ProviderAgent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("claudecode: read %s: %w", path, err)
	}
	fmBytes, body := fmark.Split(raw)
	var cf claudeFile
	if err := fmark.Decode(fmBytes, &cf); err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("claudecode: %w", err)
	}
	all, err := fmark.DecodeMap(fmBytes)
	if err != nil {
		return contract.ProviderAgent{}, fmt.Errorf("claudecode: %w", err)
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
// non-canonical frontmatter keys under ProviderOverrides["claude-code"].
func (Provider) ToCanonical(p contract.ProviderAgent) (contract.CanonicalAgent, error) {
	var cf claudeFile
	// Rebuild the typed view from the decoded frontmatter for canonical fields.
	cf.Name = povr.String(p.Fields["name"])
	cf.Description = povr.String(p.Fields["description"])
	cf.Model = povr.String(p.Fields["model"])
	cf.Tools = povr.String(p.Fields["tools"])

	ca := contract.CanonicalAgent{
		Name:        firstNonEmpty(p.Ref.Name, cf.Name),
		Description: cf.Description,
		Model:       cf.Model,
		Tools:       toolMap.MapToCanonical(commaList(cf.Tools)),
		Skills:      povr.StringSlice(p.Fields["skills"]),
		Body:        p.Body,
	}
	if ov := povr.Extras(p.Fields, knownKeys); len(ov) > 0 {
		ca.ProviderOverrides = map[string]map[string]any{name: ov}
	}
	return ca, nil
}

// Serialize renders the canonical agent back into a .claude/agents/<name>.md
// file, restoring overrides. Output field order: canonical fields first, then
// overrides sorted by key.
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
		fm.Set("tools", strings.Join(toolMap.MapToNative(a.Tools), ", "))
	}
	// Emit skills: use FieldFor so providerOverrides[claude-code]["skills"] wins
	// over the canonical Skills list when set.
	if sv, ok := a.FieldFor(name, "skills"); ok {
		if skills := povr.StringSlice(sv); len(skills) > 0 {
			fm.Set("skills", skills)
		}
	}
	// RestoreOverrides lets providerOverrides[name] win over canonical fields
	// already written above (description, model, tools). "name" is protected so
	// agent identity is never overridden.
	povr.RestoreOverrides(fm, a.ProviderOverrides[name], map[string]bool{"name": true})

	fmBytes, err := fmark.MarshalYAML(fm)
	if err != nil {
		return nil, fmt.Errorf("claudecode: %w", err)
	}
	data := fmark.Join(fmBytes, povr.NormalizeBody(a.Body))
	path := filepath.Join(".claude", "agents", a.Name+".md")
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
