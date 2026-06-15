// Package providers_test contains cross-provider alignment tests for the
// ToolMapper contract. These tests verify that all providers converge on the
// same canonical name for logically equivalent tools.
package providers_test

import (
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/antigravity"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/claudecode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/codex"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/cursor"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/geminicli"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/githubcopilot"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/goose"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/grokcli"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/opencode"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/roocode"
)

// allMappers returns every ToolMapper implementation in the codebase.
func allMappers(t *testing.T) map[string]contract.ToolMapper {
	t.Helper()
	return map[string]contract.ToolMapper{
		"claude-code":    claudecode.New(),
		"codex":          codex.New(),
		"cursor":         cursor.New(),
		"gemini-cli":     geminicli.New(),
		"github-copilot": githubcopilot.New(),
		"goose":          goose.New(),
		"grok-cli":       grokcli.New(),
		"opencode":       opencode.New(),
		"roo-code":       roocode.New(),
		"antigravity":    antigravity.New(),
	}
}

// TestAllProvidersImplementToolMapper asserts that every provider returned by
// New() satisfies contract.ToolMapper at compile time (verified via type
// assertion at runtime with a helpful error).
func TestAllProvidersImplementToolMapper(t *testing.T) {
	providers := map[string]contract.Provider{
		"claude-code":    claudecode.New(),
		"codex":          codex.New(),
		"cursor":         cursor.New(),
		"gemini-cli":     geminicli.New(),
		"github-copilot": githubcopilot.New(),
		"goose":          goose.New(),
		"grok-cli":       grokcli.New(),
		"opencode":       opencode.New(),
		"roo-code":       roocode.New(),
		"antigravity":    antigravity.New(),
	}
	for name, p := range providers {
		if _, ok := p.(contract.ToolMapper); !ok {
			t.Errorf("provider %q does not implement contract.ToolMapper", name)
		}
	}
}

// TestCrossProviderWebSearch verifies that the four spellings of "web search"
// all canonicalize to the same "web_search".
func TestCrossProviderWebSearch(t *testing.T) {
	wantCanonical := "web_search"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "WebSearch"},
		{"codex", "web_search"},
		{"opencode", "websearch"},
		{"grok-cli", "search_web"},
		{"gemini-cli", "google_web_search"},
		{"cursor", "web_search"},
		{"antigravity", "web_search"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderWebFetch verifies the web_fetch canonical across providers.
func TestCrossProviderWebFetch(t *testing.T) {
	wantCanonical := "web_fetch"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "WebFetch"},
		{"gemini-cli", "web_fetch"},
		{"github-copilot", "web_fetch"},
		{"opencode", "webfetch"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderBash verifies the bash canonical across providers.
func TestCrossProviderBash(t *testing.T) {
	wantCanonical := "bash"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "Bash"},
		{"codex", "exec_command"},
		{"cursor", "run_terminal_command"},
		{"gemini-cli", "run_shell_command"},
		{"github-copilot", "bash"},
		{"goose", "shell"},
		{"opencode", "bash"},
		{"roo-code", "command"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderReadFile verifies the read_file canonical across providers.
func TestCrossProviderReadFile(t *testing.T) {
	wantCanonical := "read_file"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "Read"},
		{"cursor", "read_file"},
		{"gemini-cli", "read_file"},
		{"github-copilot", "view"},
		{"github-copilot", "read"},
		{"opencode", "read"},
		{"roo-code", "read"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderFileEdit verifies the file_edit canonical across providers.
func TestCrossProviderFileEdit(t *testing.T) {
	wantCanonical := "file_edit"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "Edit"},
		{"cursor", "edit_file"},
		{"gemini-cli", "edit"},
		{"gemini-cli", "replace"},
		{"goose", "edit"},
		{"opencode", "edit"},
		{"roo-code", "edit"},
		{"antigravity", "edit_file"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderImageGeneration verifies image_generation canonical.
func TestCrossProviderImageGeneration(t *testing.T) {
	wantCanonical := "image_generation"
	cases := []struct {
		provider string
		native   string
	}{
		{"codex", "image_generation"},
		{"cursor", "image_generation"},
		{"grok-cli", "generate_image"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderComputerUse verifies computer_use canonical.
func TestCrossProviderComputerUse(t *testing.T) {
	wantCanonical := "computer_use"
	cases := []struct {
		provider string
		native   string
	}{
		{"codex", "computer_use"},
		{"grok-cli", "computer"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderGlob verifies glob canonical across providers.
func TestCrossProviderGlob(t *testing.T) {
	wantCanonical := "glob"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "Glob"},
		{"gemini-cli", "glob"},
		{"github-copilot", "glob"},
		{"opencode", "glob"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderGrep verifies grep canonical across providers.
func TestCrossProviderGrep(t *testing.T) {
	wantCanonical := "grep"
	cases := []struct {
		provider string
		native   string
	}{
		{"claude-code", "Grep"},
		{"cursor", "grep_search"},
		{"gemini-cli", "search_file_content"},
		{"github-copilot", "grep"},
		{"github-copilot", "rg"},
		{"opencode", "grep"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestCrossProviderListDirectory verifies list_directory canonical across providers.
func TestCrossProviderListDirectory(t *testing.T) {
	wantCanonical := "list_directory"
	cases := []struct {
		provider string
		native   string
	}{
		{"cursor", "list_dir"},
		{"gemini-cli", "list_directory"},
		{"antigravity", "list_dir"},
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q): not found", tc.provider, tc.native)
			continue
		}
		if got != wantCanonical {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, wantCanonical)
		}
	}
}

// TestRoundTripNativeCanonicalNative verifies native→canonical→native identity
// for every provider's full tool set.
func TestRoundTripNativeCanonicalNative(t *testing.T) {
	// For each provider, for each canonical tool, verify that:
	//   NativeTool(canonical) → native, then CanonicalTool(native) == canonical.
	// Note: aliased tools (multiple natives → one canonical) only have the
	// "first wins" native in the reverse direction, so we test canonical→native→canonical.
	mappers := allMappers(t)
	for providerName, m := range mappers {
		for _, canonical := range m.Tools() {
			native, ok := m.NativeTool(canonical)
			if !ok {
				t.Errorf("[%s] NativeTool(%q): not found", providerName, canonical)
				continue
			}
			got, ok := m.CanonicalTool(native)
			if !ok {
				t.Errorf("[%s] CanonicalTool(%q) (from NativeTool(%q)): not found", providerName, native, canonical)
				continue
			}
			if got != canonical {
				t.Errorf("[%s] round-trip canonical=%q → native=%q → canonical=%q; want %q",
					providerName, canonical, native, got, canonical)
			}
		}
	}
}

// TestToolsReturnNonEmpty verifies every provider exposes at least one tool.
func TestToolsReturnNonEmpty(t *testing.T) {
	mappers := allMappers(t)
	for providerName, m := range mappers {
		tools := m.Tools()
		if len(tools) == 0 {
			t.Errorf("[%s] Tools() returned empty slice", providerName)
		}
	}
}

// TestCaseInsensitiveLookupAcrossProviders verifies that mixed-case native
// names resolve correctly in each provider.
func TestCaseInsensitiveLookupAcrossProviders(t *testing.T) {
	cases := []struct {
		provider  string
		native    string // wrong-case variant
		wantCanon string
	}{
		{"claude-code", "websearch", "web_search"},   // native is "WebSearch"
		{"claude-code", "BASH", "bash"},               // native is "Bash"
		{"claude-code", "read", "read_file"},          // native is "Read"
		{"codex", "EXEC_COMMAND", "bash"},              // native is "exec_command"
		{"opencode", "WEBSEARCH", "web_search"},       // native is "websearch"
		{"cursor", "GREP_SEARCH", "grep"},             // native is "grep_search"
		{"grok-cli", "SEARCH_WEB", "web_search"},      // native is "search_web"
		{"gemini-cli", "GOOGLE_WEB_SEARCH", "web_search"}, // native is "google_web_search"
	}
	mappers := allMappers(t)
	for _, tc := range cases {
		m := mappers[tc.provider]
		got, ok := m.CanonicalTool(tc.native)
		if !ok {
			t.Errorf("[%s] CanonicalTool(%q) (case-insensitive): not found", tc.provider, tc.native)
			continue
		}
		if got != tc.wantCanon {
			t.Errorf("[%s] CanonicalTool(%q) = %q; want %q", tc.provider, tc.native, got, tc.wantCanon)
		}
	}
}
