package transform

// Track C — tool transform through the sync stages + per-provider tool override.
//
// Level: unit/transform — pure ToolMapper / FromCanonical logic, no IO.
// Covers:
//   - Per-provider transformer round-trip over ALL tools the provider maps:
//     canonical -> native -> canonical is identity (includes opencode).
//   - C-D3: providerOverrides[p]["tools"] (canonical spelling) REPLACES the
//     canonical Tools for provider p only, mapped to native on apply, and is
//     isolated from other providers.
//   - Unknown native tool with no canonical mapping → pass-through + warning
//     (never dropped).

import (
	"os"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// toolMapperProviders returns the registered providers that implement
// contract.ToolMapper, keyed by id.
func toolMapperProviders(r *Registry) map[string]contract.ToolMapper {
	out := map[string]contract.ToolMapper{}
	for _, id := range r.Providers() {
		p, _ := r.Provider(id)
		if tm, ok := p.(contract.ToolMapper); ok {
			out[id] = tm
		}
	}
	return out
}

// TestToolMapper_RoundTripAllTools asserts that for every ToolMapper provider,
// every canonical tool it supports survives canonical -> native -> canonical as
// an identity. This is the per-provider transformer round-trip over ALL tools
// (opencode included, the regression target for the previously-unmapped gap).
func TestToolMapper_RoundTripAllTools(t *testing.T) {
	r := Default()
	mappers := toolMapperProviders(r)
	if len(mappers) == 0 {
		t.Fatal("no ToolMapper providers registered")
	}
	for id, tm := range mappers {
		id, tm := id, tm
		t.Run("provider="+id, func(t *testing.T) {
			tools := tm.Tools()
			if len(tools) == 0 {
				t.Skipf("provider %q maps no tools", id)
			}
			for _, canon := range tools {
				native, ok := tm.NativeTool(canon)
				if !ok {
					t.Errorf("%s: NativeTool(%q) not found although listed in Tools()", id, canon)
					continue
				}
				back, ok := tm.CanonicalTool(native)
				if !ok {
					t.Errorf("%s: CanonicalTool(%q) (native of %q) not found", id, native, canon)
					continue
				}
				if back != canon {
					t.Errorf("%s: round-trip mismatch %q -> %q -> %q", id, canon, native, back)
				}
			}
		})
	}
}

// TestOpencodeRegression_ToolsRoundTripThroughTransform asserts opencode tools
// now enter and leave the canonical pipeline (the C gap: opencode `tools` field
// was never mapped). A canonical agent with a tool is serialized; the native
// `tools:` object map must carry the native spelling; re-parsing must recover
// the canonical tool.
func TestOpencodeRegression_ToolsRoundTripThroughTransform(t *testing.T) {
	r := Default()
	prov, _ := r.Provider("opencode")
	ca := contract.CanonicalAgent{
		Name:  "rt",
		Body:  "body",
		Tools: []string{"web_search", "read_file"},
	}
	writes, err := r.FromCanonical(ca, "opencode")
	if err != nil {
		t.Fatalf("FromCanonical: %v", err)
	}
	out := string(writes[0].Data)
	// opencode native spellings: web_search->websearch, read_file->read.
	if !strings.Contains(out, "websearch") || !strings.Contains(out, "read") {
		t.Fatalf("opencode output missing native tool spellings:\n%s", out)
	}
	// Re-parse and confirm canonical recovery.
	dir := t.TempDir()
	path := dir + "/rt.md"
	if err := writeFile(path, writes[0].Data); err != nil {
		t.Fatal(err)
	}
	pa, err := prov.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ca2, err := r.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if !containsAll(ca2.Tools, "web_search", "read_file") {
		t.Errorf("opencode re-canonicalized tools = %v, want web_search & read_file", ca2.Tools)
	}
}

// TestToolOverride_PerProvider_ReplacesAndIsolates covers C-D3: a per-provider
// tools override in CANONICAL spelling replaces the canonical Tools for that
// provider only, is mapped to native on apply, and never leaks to others.
func TestToolOverride_PerProvider_ReplacesAndIsolates(t *testing.T) {
	r := Default()
	// claude-code maps web_search->WebSearch, bash->Bash, read_file->Read.
	ca := contract.CanonicalAgent{
		Name:  "ovr",
		Body:  "body",
		Tools: []string{"bash", "read_file"},
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"tools": []any{"web_search"}},
		},
	}
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatalf("FromCanonical claude-code: %v", err)
	}
	out := string(writes[0].Data)
	// Override (web_search -> WebSearch) must be present; canonical set replaced.
	if !strings.Contains(out, "WebSearch") {
		t.Errorf("claude-code missing overridden tool WebSearch:\n%s", out)
	}
	if strings.Contains(out, "Bash") || strings.Contains(out, "Read") {
		t.Errorf("claude-code still carries the replaced canonical tools (Bash/Read):\n%s", out)
	}

	// Isolation: gemini-cli (no override) must keep the canonical set, mapped to
	// its own native spelling, and NOT carry the override tool.
	gw, err := r.FromCanonical(ca, "gemini-cli")
	if err != nil {
		t.Fatalf("FromCanonical gemini-cli: %v", err)
	}
	gout := string(gw[0].Data)
	if strings.Contains(gout, "google_web_search") {
		t.Errorf("claude-code tool override leaked into gemini-cli:\n%s", gout)
	}
	// gemini native: bash->run_shell_command, read_file->read_file.
	if !strings.Contains(gout, "run_shell_command") || !strings.Contains(gout, "read_file") {
		t.Errorf("gemini-cli missing canonical tools in native spelling:\n%s", gout)
	}
	// The raw override must NOT be re-written verbatim as a canonical name.
	if strings.Contains(out, "web_search") {
		t.Errorf("claude-code emitted raw canonical override spelling instead of native:\n%s", out)
	}
}

// TestToolOverride_EmptyMeansNoTools verifies an explicit empty tools override
// suppresses all tools for that provider (distinct from an absent override).
func TestToolOverride_EmptyMeansNoTools(t *testing.T) {
	r := Default()
	ca := contract.CanonicalAgent{
		Name:  "empty",
		Body:  "body",
		Tools: []string{"bash"},
		ProviderOverrides: map[string]map[string]any{
			"claude-code": {"tools": []any{}},
		},
	}
	writes, err := r.FromCanonical(ca, "claude-code")
	if err != nil {
		t.Fatalf("FromCanonical: %v", err)
	}
	out := string(writes[0].Data)
	if strings.Contains(out, "tools:") {
		t.Errorf("claude-code emitted a tools field despite empty override:\n%s", out)
	}
}

// TestUnknownTool_PassthroughWithWarning verifies an unmapped native tool is
// kept verbatim in canonical (never dropped) and triggers a warning.
func TestUnknownTool_PassthroughWithWarning(t *testing.T) {
	r := Default()
	var warnings []string
	r.SetWarnf(func(format string, args ...any) {
		warnings = append(warnings, format)
	})
	// claude-code with a native tool that has no canonical mapping.
	prov, _ := r.Provider("claude-code")
	pa := contract.ProviderAgent{
		Provider: "claude-code",
		Ref:      contract.AgentRef{Name: "u", Provider: "claude-code"},
		Fields:   map[string]any{"name": "u", "tools": "Bash, TotallyUnknownTool"},
		Body:     "body",
	}
	_ = prov // ensure provider exists
	ca, err := r.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if !containsAll(ca.Tools, "bash", "TotallyUnknownTool") {
		t.Errorf("unknown tool dropped or canonical wrong: %v", ca.Tools)
	}
	if len(warnings) == 0 {
		t.Errorf("expected a pass-through warning for the unmapped tool, got none")
	}
}

// TestUnknownTool_WildcardNoWarning verifies wildcard / MCP / spawn entries are
// passed through WITHOUT a spurious unmapped-tool warning.
func TestUnknownTool_WildcardNoWarning(t *testing.T) {
	r := Default()
	var warnings int
	r.SetWarnf(func(string, ...any) { warnings++ })
	pa := contract.ProviderAgent{
		Provider: "claude-code",
		Ref:      contract.AgentRef{Name: "w", Provider: "claude-code"},
		Fields:   map[string]any{"name": "w", "tools": "Bash, *, mcp__server__tool, Agent(foo)"},
		Body:     "body",
	}
	ca, err := r.ToCanonical(pa)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if !containsAll(ca.Tools, "bash", "*", "mcp__server__tool", "Agent(foo)") {
		t.Errorf("wildcard/MCP entries dropped: %v", ca.Tools)
	}
	if warnings != 0 {
		t.Errorf("wildcard/MCP entries should not warn, got %d warnings", warnings)
	}
}

func containsAll(s []string, want ...string) bool {
	set := map[string]bool{}
	for _, v := range s {
		set[v] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
