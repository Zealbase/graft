// Package canonical owns the provider-neutral, on-disk representation of one
// agent under .graft/agents/<name>/. It is the source of truth for how a
// contract.CanonicalAgent is (de)serialized, content-hashed, and validated.
//
// On-disk layout (see plan 02):
//
//	.graft/agents/<name>/
//	├─ agent.yaml        # canonical fields (provider-neutral)
//	├─ instructions.md   # body / system prompt (contract.CanonicalAgent.Body)
//	└─ .meta.json        # per-provider source hash + last commit hash
//
// The contract.CanonicalAgent struct (internal/contract) is frozen and narrower
// than the research common-agent-definition schema. This package maps between
// the two: the frozen fields are first-class YAML keys; the richer schema fields
// that have no frozen home travel inside providerOverrides / instructions only.
package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

const (
	agentFile = "agent.yaml"
	bodyFile  = "instructions.md"
	metaFile  = ".meta.json"
	agentsDir = "agents"
	graftDir  = ".graft"
)

// agentDoc is the exact on-disk shape of agent.yaml. Field order here is the
// serialization order (yaml.v3 marshals struct fields in declaration order),
// giving deterministic output. Maps are sorted by yaml.v3 on marshal.
type agentDoc struct {
	Name              string                            `yaml:"name"`
	Description       string                            `yaml:"description,omitempty"`
	Model             string                            `yaml:"model,omitempty"`
	Tools             []string                          `yaml:"tools,omitempty"`
	MCP               []string                          `yaml:"mcp,omitempty"`
	Permissions       map[string]string                 `yaml:"permissions,omitempty"`
	ProviderOverrides map[string]map[string]any `yaml:"providerOverrides,omitempty"`
}

func toDoc(a contract.CanonicalAgent) agentDoc {
	return agentDoc{
		Name:              a.Name,
		Description:       a.Description,
		Model:             a.Model,
		Tools:             a.Tools,
		MCP:               a.MCP,
		Permissions:       a.Permissions,
		ProviderOverrides: pruneOverrides(a.ProviderOverrides),
	}
}

func fromDoc(d agentDoc, body string) contract.CanonicalAgent {
	return contract.CanonicalAgent{
		Name:              d.Name,
		Description:       d.Description,
		Model:             d.Model,
		Tools:             d.Tools,
		MCP:               d.MCP,
		Permissions:       d.Permissions,
		Body:              body,
		ProviderOverrides: pruneOverrides(d.ProviderOverrides),
	}
}

// pruneOverrides returns a copy of the provider overrides map with empty
// buckets removed. An empty bucket (map[string]any{}) is functionally
// equivalent to the bucket being absent; keeping it would cause empty-bucket
// YAML (`claude: {}`) to round-trip back as a non-nil entry, which is
// indistinguishable from a deliberately-set empty bucket and will resurrect
// deleted keys on next Save. Returns nil when the result would be empty.
func pruneOverrides(m map[string]map[string]any) map[string]map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(m))
	for k, v := range m {
		if len(v) > 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// defaultAgentTemplate is the body written when BuildDefault is called with an
// empty prompt. It is a short sensible placeholder that makes the resulting
// agent immediately usable while clearly inviting the author to fill it in.
const defaultAgentTemplate = `You are a helpful agent.

Describe your agent's purpose and behaviour here. This file is
instructions.md — edit it to customise what this agent does.
`

// BuildDefault constructs a minimal canonical agent with the given name and
// body. If prompt is empty the default template is used. All optional fields
// (Model, Tools, MCP, Permissions, ProviderOverrides) are left at their zero
// values so the agent is clean for the caller to populate later.
//
// The gateway calls BuildDefault then SaveWithMeta(dir, agent, Meta{}) to
// write the three on-disk files with an empty Meta (no provider hashes). That
// signals to the sync engine that this agent is new and canonical-drifted,
// causing it to fan out to all enabled providers on the next sync.
func BuildDefault(name, prompt string) contract.CanonicalAgent {
	body := prompt
	if body == "" {
		body = defaultAgentTemplate
	}
	return contract.CanonicalAgent{
		Name: name,
		Body: body,
	}
}

// AgentDir returns the directory holding a named agent's files:
// <dir>/.graft/agents/<name>/.
func AgentDir(dir, name string) string {
	return filepath.Join(dir, graftDir, agentsDir, name)
}

// Load reads the canonical agent named by the last path segment of dir, where
// dir is the agent's own directory (.../.graft/agents/<name>). It reads
// agent.yaml, folds instructions.md into Body, and ignores .meta.json (use
// LoadMeta for that). The on-disk name field is authoritative.
func Load(dir string) (contract.CanonicalAgent, error) {
	raw, err := os.ReadFile(filepath.Join(dir, agentFile))
	if err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("canonical: read %s: %w", agentFile, err)
	}
	var d agentDoc
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("canonical: parse %s: %w", agentFile, err)
	}

	var body string
	if b, err := os.ReadFile(filepath.Join(dir, bodyFile)); err == nil {
		// Normalize line endings at load so a CRLF-sourced instructions.md does
		// not propagate CRLF downstream to provider files (whose Serialize only
		// trims trailing newlines) or cause spurious merge non-agreement.
		body = normalizeBody(string(b))
	} else if !os.IsNotExist(err) {
		return contract.CanonicalAgent{}, fmt.Errorf("canonical: read %s: %w", bodyFile, err)
	}

	return fromDoc(d, body), nil
}

// LoadMeta reads the .meta.json sidecar from an agent directory. A missing file
// yields a zero Meta and no error.
func LoadMeta(dir string) (Meta, error) {
	b, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, nil
		}
		return Meta{}, fmt.Errorf("canonical: read %s: %w", metaFile, err)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, fmt.Errorf("canonical: parse %s: %w", metaFile, err)
	}
	return m, nil
}

// Save renders the three on-disk artifacts (agent.yaml, instructions.md,
// .meta.json) for the agent under <dir>/.graft/agents/<name>/ and returns them
// as contract.FileWrite values (paths absolute-relative to dir; the caller owns
// the actual write so Save itself performs no IO). Output is deterministic:
// stable field order in agent.yaml, sorted map keys, and a recomputed
// canonical_hash in .meta.json.
func Save(dir string, a contract.CanonicalAgent) ([]contract.FileWrite, error) {
	return SaveWithMeta(dir, a, Meta{})
}

// SaveWithMeta is Save but lets the caller supply the provider source-hash map
// to embed in .meta.json. CanonicalHash is always recomputed from a.
func SaveWithMeta(dir string, a contract.CanonicalAgent, meta Meta) ([]contract.FileWrite, error) {
	if a.Name == "" {
		return nil, fmt.Errorf("canonical: cannot save agent with empty name")
	}
	base := AgentDir(dir, a.Name)

	yamlBytes, err := marshalAgentYAML(a)
	if err != nil {
		return nil, err
	}

	meta.CanonicalHash = Hash(a)
	metaBytes, err := marshalMeta(meta)
	if err != nil {
		return nil, err
	}

	writes := []contract.FileWrite{
		{Path: filepath.Join(base, agentFile), Data: yamlBytes},
		{Path: filepath.Join(base, bodyFile), Data: []byte(normalizeBody(a.Body))},
		{Path: filepath.Join(base, metaFile), Data: metaBytes},
	}
	return writes, nil
}

// marshalAgentYAML produces deterministic agent.yaml bytes.
func marshalAgentYAML(a contract.CanonicalAgent) ([]byte, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(toDoc(a)); err != nil {
		return nil, fmt.Errorf("canonical: encode %s: %w", agentFile, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("canonical: encode %s: %w", agentFile, err)
	}
	return []byte(buf.String()), nil
}

// marshalMeta produces deterministic .meta.json bytes (sorted keys via
// json.Marshal of maps, indented for readability/diff-friendliness).
func marshalMeta(m Meta) ([]byte, error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("canonical: encode %s: %w", metaFile, err)
	}
	b = append(b, '\n')
	return b, nil
}

// normalizeBody ensures the instructions body ends with exactly one trailing
// newline (so round-trips are stable and files are POSIX-clean), while an empty
// body stays empty.
func normalizeBody(body string) string {
	if body == "" {
		return ""
	}
	// Normalize CRLF/CR to LF first so a body sourced from a Windows-checked-out
	// provider file hashes identically to its LF counterpart (no false drift).
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimRight(body, "\n") + "\n"
}

// nilToEmpty coerces a nil slice to a non-nil empty slice so an absent section
// (nil) and an explicitly-empty section ([]) produce the SAME canonical hash —
// otherwise providers returning []string{} drift forever against a stored nil.
func nilToEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Hash returns a stable, deterministic sha256 hex digest of the canonical agent.
// It is the value stored as AGENT.canonical_hash / Meta.CanonicalHash. The body
// is included so instruction edits change the hash. The serialization is a
// canonical JSON form with sorted keys, independent of in-memory map ordering.
func Hash(a contract.CanonicalAgent) string {
	h := sha256.Sum256(canonicalBytes(a))
	return hex.EncodeToString(h[:])
}

// canonicalBytes is the stable byte form fed to the hash. It is its own format
// (not agent.yaml) so cosmetic YAML changes never shift the hash; only semantic
// field values do. Body is normalized so trailing-newline churn is invisible.
func canonicalBytes(a contract.CanonicalAgent) []byte {
	// Build a fully-sorted structure then JSON-encode it via orderedMap.
	doc := map[string]any{
		"name":              a.Name,
		"description":       a.Description,
		"model":             a.Model,
		"tools":             nilToEmpty(a.Tools),
		"mcp":               nilToEmpty(a.MCP),
		"permissions":       sortedStringMap(a.Permissions),
		"providerOverrides": canonicalizeOverrides(a.ProviderOverrides),
		"body":              normalizeBody(a.Body),
	}
	b, _ := json.Marshal(orderedMap(doc))
	return b
}

// orderedMap converts a map into a json.Marshaler that emits keys in sorted
// order, recursively, so encoding is deterministic regardless of Go map order.
type orderedMap map[string]any

func (o orderedMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(deepOrder(o[k]))
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// deepOrder wraps nested maps in orderedMap so sorting recurses.
func deepOrder(v any) any {
	switch m := v.(type) {
	case map[string]any:
		return orderedMap(m)
	case orderedMap:
		return m
	case []any:
		out := make([]any, len(m))
		for i := range m {
			out[i] = deepOrder(m[i])
		}
		return out
	default:
		return v
	}
}

func sortedStringMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func canonicalizeOverrides(m map[string]map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		inner := make(map[string]any, len(v))
		for ik, iv := range v {
			inner[ik] = iv
		}
		out[k] = inner
	}
	return out
}
