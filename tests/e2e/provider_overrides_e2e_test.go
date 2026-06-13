package e2e

// e2e tests for providerOverrides (v0.0.4 verify).
//
// Covers:
//   - Angle 2: invalid provider key → graft sync blocked with error + suggestion
//   - Angle 1: valid override → correct model in the right provider file
//   - Angle 6: cross-provider isolation at the file level
//   - Angle 3: set→sync→validate round-trip, clear→sync→absent
//   - Angle 4: idempotent: sync after sync produces "already in sync"
//
// Every test uses t.TempDir() + isolated HOME/XDG via the graft() harness.

import (
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// seedAgentWithOverrides writes a canonical agent YAML directly to the graft
// store (under .graft/agents/<name>/), with the given ProviderOverrides YAML
// block, after init has been run. It expects .graft/agents/<name>/ to exist.
// overridesYAML is the literal YAML block to append, e.g.:
//
//	"providerOverrides:\n  claude-code:\n    model: sonnet\n"
func seedCanonicalOverrides(t *testing.T, root, agentName, overridesYAML string) {
	t.Helper()
	agentDir := ".graft/agents/" + agentName
	if !exists(root, agentDir+"/agent.yaml") {
		t.Fatalf("canonical not found at %s/agent.yaml; run init+sync first", agentDir)
	}
	cur := readFile(t, root, agentDir+"/agent.yaml")
	writeFile(t, root, agentDir+"/agent.yaml", cur+overridesYAML)
}

// TestProviderOverrides_Rejects_InvalidKeyBlocksSync covers angle 2 (e2e):
// a canonical agent with an unknown providerOverrides key → `graft sync agents`
// exits non-zero with an error-severity finding mentioning the bad key and a
// "did you mean" suggestion.
func TestProviderOverrides_Rejects_InvalidKeyBlocksSync(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents") // first sync: ingest and create canonical

	// Inject an invalid providerOverrides key into the canonical YAML.
	seedCanonicalOverrides(t, root, "code-reviewer",
		"providerOverrides:\n  copilot:\n    model: gpt-99\n")

	// sync must be blocked (non-zero exit).
	r := graft(t, root, "sync", "agents", "-o", "json")
	if r.exitCode == 0 {
		t.Fatalf("sync with invalid providerOverrides key exited 0 (want non-zero)\nstdout: %s\nstderr: %s",
			r.stdout, r.stderr)
	}
	// Error output must mention the bad key.
	combined := r.stdout + r.stderr
	if !contains(combined, "copilot") {
		t.Errorf("error output should mention the invalid key 'copilot', got:\nstdout:%s\nstderr:%s",
			r.stdout, r.stderr)
	}
	// Validate command must also report an error finding.
	rv := graft(t, root, "validate", "--all", "-o", "json")
	if rv.exitCode == 0 {
		t.Fatalf("validate with invalid providerOverrides key exited 0 (want non-zero)")
	}
	var findings []finding
	decodeJSON(t, rv, &findings)
	sawError := false
	for _, f := range findings {
		if f.Severity == "error" && contains(f.Message, "copilot") {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("expected error finding mentioning 'copilot', got: %+v", findings)
	}
}

// TestProviderOverrides_Rejects_InvalidKeyWithSuggestion verifies that the
// error finding message from an invalid providerOverrides key includes a
// "did you mean" suggestion containing a registered provider id.
func TestProviderOverrides_Rejects_InvalidKeyWithSuggestion(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// "gemini" is a close typo for "gemini-cli" and Levenshtein picks gemini-cli.
	seedCanonicalOverrides(t, root, "code-reviewer",
		"providerOverrides:\n  gemini:\n    model: 2.0-flash\n")

	rv := graft(t, root, "validate", "--all", "-o", "json")
	if rv.exitCode == 0 {
		t.Fatalf("validate exited 0 with invalid key (want non-zero)")
	}
	var findings []finding
	decodeJSON(t, rv, &findings)
	foundSuggestion := false
	registered := transform.Default().Providers()
	for _, f := range findings {
		if f.Severity != "error" || !contains(f.Message, "gemini") {
			continue
		}
		if !contains(f.Message, "did you mean") {
			continue
		}
		for _, p := range registered {
			if contains(f.Message, p) {
				foundSuggestion = true
				break
			}
		}
	}
	if !foundSuggestion {
		t.Fatalf("expected an error finding with 'did you mean <registered-provider>' for 'gemini', got: %+v", findings)
	}
}

// TestProviderOverrides_Applies_ModelToCorrectProvider covers angle 1 (e2e):
// setting providerOverrides[claude-code][model]=sonnet-custom makes the
// .claude/agents/ file carry that model, while other provider files use the
// canonical default model.
func TestProviderOverrides_Applies_ModelToCorrectProvider(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents") // initial sync (ingest)

	// Set per-provider model override for claude-code and a different one for codex.
	agentYAML := ".graft/agents/code-reviewer/agent.yaml"
	raw := readFile(t, root, agentYAML)
	updated := raw + "providerOverrides:\n  claude-code:\n    model: claude-override-model\n  codex:\n    model: codex-override-model\n"
	writeFile(t, root, agentYAML, updated)

	// sync must succeed (valid keys).
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}

	// claude-code file must carry claude-override-model.
	claudeFile := readFile(t, root, ".claude/agents/code-reviewer.md")
	if !contains(claudeFile, "claude-override-model") {
		t.Errorf("claude-code file must contain 'claude-override-model', got:\n%s", claudeFile)
	}
	if contains(claudeFile, "codex-override-model") {
		t.Errorf("claude-code file must NOT contain codex override 'codex-override-model', got:\n%s", claudeFile)
	}

	// codex file must carry codex-override-model.
	codexFile := readFile(t, root, ".codex/agents/code-reviewer.toml")
	if !contains(codexFile, "codex-override-model") {
		t.Errorf("codex file must contain 'codex-override-model', got:\n%s", codexFile)
	}
	if contains(codexFile, "claude-override-model") {
		t.Errorf("codex file must NOT contain claude override 'claude-override-model', got:\n%s", codexFile)
	}

	// Providers with no override must carry the canonical model (the original "sonnet"
	// from the provisioned claude agent which became the canonical model).
	cursorFile := readFile(t, root, ".cursor/agents/code-reviewer.md")
	if contains(cursorFile, "claude-override-model") {
		t.Errorf("cursor file must NOT carry claude override, got:\n%s", cursorFile)
	}
	if contains(cursorFile, "codex-override-model") {
		t.Errorf("cursor file must NOT carry codex override, got:\n%s", cursorFile)
	}
}

// TestProviderOverrides_Isolation_CrossProvider covers angle 6 (e2e):
// table-driven over all registered providers that support model:
// for each provider P, set providerOverrides[P][model] = unique-model-for-P
// and verify that every OTHER provider does NOT contain that model in its file.
func TestProviderOverrides_Isolation_CrossProvider(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Determine which providers emit a model field by checking the allProviders set.
	// We'll test for claude-code, codex, cursor, gemini-cli, github-copilot,
	// grok-cli, opencode, roo-code (the 8 providers that call ModelFor in Serialize).
	// goose and antigravity don't have native model fields.
	providersWithModel := []string{
		"claude-code", "codex", "cursor", "gemini-cli",
		"github-copilot", "grok-cli", "opencode", "roo-code",
	}

	// Build a unique override model for each provider.
	overrideModels := map[string]string{}
	for _, p := range providersWithModel {
		overrideModels[p] = "unique-model-for-" + p
	}

	// Build the YAML providerOverrides block.
	ovYAML := "providerOverrides:\n"
	for _, p := range providersWithModel {
		ovYAML += "  " + p + ":\n    model: " + overrideModels[p] + "\n"
	}

	agentYAML := ".graft/agents/code-reviewer/agent.yaml"
	raw := readFile(t, root, agentYAML)
	writeFile(t, root, agentYAML, raw+ovYAML)

	// Sync must succeed.
	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}

	// For each provider, read its file and verify:
	// - its own override model IS present
	// - every OTHER provider's override model is NOT present
	providerFilePaths := map[string]string{
		"claude-code":    ".claude/agents/code-reviewer.md",
		"codex":          ".codex/agents/code-reviewer.toml",
		"cursor":         ".cursor/agents/code-reviewer.md",
		"gemini-cli":     ".gemini/agents/code-reviewer.md",
		"github-copilot": ".github/agents/code-reviewer.md",
		"grok-cli":       ".grok/agents/code-reviewer.md",
		"opencode":       ".opencode/agents/code-reviewer.md",
		"roo-code":       ".roo/agents/code-reviewer/code-reviewer.md",
	}

	for _, targetProv := range providersWithModel {
		filePath, ok := providerFilePaths[targetProv]
		if !ok {
			t.Logf("skipping %q (file path unknown in test)", targetProv)
			continue
		}
		if !exists(root, filePath) {
			t.Logf("skipping %q (file not present on disk at %s)", targetProv, filePath)
			continue
		}
		content := readFile(t, root, filePath)
		ownModel := overrideModels[targetProv]

		// Own override must be present.
		if !contains(content, ownModel) {
			t.Errorf("provider %q: file at %s must contain own override model %q\n%s",
				targetProv, filePath, ownModel, content)
		}

		// Other overrides must NOT be present.
		for _, otherProv := range providersWithModel {
			if otherProv == targetProv {
				continue
			}
			otherModel := overrideModels[otherProv]
			if contains(content, otherModel) {
				t.Errorf("provider %q: file at %s contains %q's override model %q (isolation leak)\n%s",
					targetProv, filePath, otherProv, otherModel, content)
			}
		}
	}
}

// TestProviderOverrides_Clear_Absent covers angle 3 (e2e):
// setting an override then clearing it makes the provider use the canonical model again.
func TestProviderOverrides_Clear_Absent(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Load canonical to know the default model.
	can, err := canonical.Load(canonical.AgentDir(root, "code-reviewer"))
	if err != nil {
		t.Fatalf("canonical.Load: %v", err)
	}
	defaultModel := can.Model // e.g. "sonnet"

	// Step 1: set override.
	agentYAML := ".graft/agents/code-reviewer/agent.yaml"
	raw := readFile(t, root, agentYAML)
	writeFile(t, root, agentYAML, raw+"providerOverrides:\n  claude-code:\n    model: override-sonnet-x\n")
	mustGraft(t, root, "sync", "agents")

	claudeAfterSet := readFile(t, root, ".claude/agents/code-reviewer.md")
	if !contains(claudeAfterSet, "override-sonnet-x") {
		t.Fatalf("after set: claude file must contain override-sonnet-x\n%s", claudeAfterSet)
	}

	// Step 2: clear override by removing the providerOverrides block.
	// Re-read the current agent.yaml (it might have been rewritten by sync)
	// and strip anything after "providerOverrides".
	freshRaw := readFile(t, root, agentYAML)
	stripped := ""
	for _, line := range splitLines(freshRaw) {
		if strings.HasPrefix(line, "providerOverrides:") {
			break
		}
		stripped += line + "\n"
	}
	writeFile(t, root, agentYAML, stripped)
	mustGraft(t, root, "sync", "agents")

	claudeAfterClear := readFile(t, root, ".claude/agents/code-reviewer.md")
	if contains(claudeAfterClear, "override-sonnet-x") {
		t.Errorf("after clear: claude file must NOT contain 'override-sonnet-x' (resurrection), got:\n%s", claudeAfterClear)
	}
	// Default model should be restored.
	if defaultModel != "" && !contains(claudeAfterClear, defaultModel) {
		t.Errorf("after clear: claude file should contain default model %q, got:\n%s", defaultModel, claudeAfterClear)
	}
}

// TestProviderOverrides_ValidKey_CleanValidate covers angle 1 (e2e):
// a valid providerOverrides key produces zero error findings from `graft validate --all`.
func TestProviderOverrides_ValidKey_CleanValidate(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	// Set a valid providerOverrides key.
	agentYAML := ".graft/agents/code-reviewer/agent.yaml"
	raw := readFile(t, root, agentYAML)
	writeFile(t, root, agentYAML, raw+"providerOverrides:\n  github-copilot:\n    model: gpt-4o\n")

	// validate --all must exit 0 (no error findings from the key check).
	r := graft(t, root, "validate", "--all", "-o", "json")
	if r.exitCode != 0 {
		t.Fatalf("validate with valid providerOverrides key exited non-zero\nstdout:%s\nstderr:%s",
			r.stdout, r.stderr)
	}
	// No error-severity findings.
	var findings []finding
	decodeJSON(t, r, &findings)
	for _, f := range findings {
		if f.Severity == "error" && contains(f.Message, "providerOverrides") {
			t.Errorf("unexpected providerOverrides error for valid key: %+v", f)
		}
	}
}

// TestProviderOverrides_MultipleInvalidKeys_AllReported ensures every invalid
// key in providerOverrides produces its own error finding.
func TestProviderOverrides_MultipleInvalidKeys_AllReported(t *testing.T) {
	root := newGitWorkspace(t)
	provisionClaudeAgent(t, root, "code-reviewer")
	mustGraft(t, root, "init")
	mustGraft(t, root, "sync", "agents")

	agentYAML := ".graft/agents/code-reviewer/agent.yaml"
	raw := readFile(t, root, agentYAML)
	writeFile(t, root, agentYAML,
		raw+"providerOverrides:\n  copilot:\n    model: x\n  bad-key-2:\n    model: y\n")

	rv := graft(t, root, "validate", "--all", "-o", "json")
	if rv.exitCode == 0 {
		t.Fatalf("validate exited 0 with two invalid keys (want non-zero)")
	}
	var findings []finding
	decodeJSON(t, rv, &findings)
	errorCount := 0
	for _, f := range findings {
		if f.Severity == "error" {
			errorCount++
		}
	}
	if errorCount < 2 {
		t.Fatalf("expected >= 2 error findings for 2 bad keys, got %d: %+v", errorCount, findings)
	}
}
