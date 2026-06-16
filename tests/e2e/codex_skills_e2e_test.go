// Package e2e — regression guard for "skills.config applied after graft sync".
//
// These tests verify that the codex provider correctly round-trips the
// skills.config block: canonical->codex serialization writes [[skills.config]]
// entries, and codex->canonical->claude-code propagation preserves skill names.
package e2e

import (
	"strings"
	"testing"
)

// TestCodexSkillsConfig_CanonicalToCodex verifies that a canonical agent.yaml
// with a skills list produces a codex TOML with [[skills.config]] entries,
// all enabled=true, after graft sync agents.
func TestCodexSkillsConfig_CanonicalToCodex(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "init")

	writeFile(t, root, ".graft/agents/researcher/agent.yaml", `name: researcher
description: A research agent
model: gpt-4o
skills:
  - docs-editor
  - code-search
`)
	writeFile(t, root, ".graft/agents/researcher/instructions.md", "You are a research agent.\n")
	gitCommitAll(t, root, "add researcher")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "researcher") {
		t.Fatalf("changed=%v, want researcher", res.Changed)
	}

	toml := readFile(t, root, ".codex/agents/researcher.toml")

	checks := []struct {
		desc    string
		want    string
		present bool
	}{
		{"[[skills.config]] block", "[[skills.config]]", true},
		{"docs-editor skill name", `name = "docs-editor"`, true},
		{"code-search skill name", `name = "code-search"`, true},
		{"enabled = true", "enabled = true", true},
		{"no enabled = false", "enabled = false", false},
	}
	for _, c := range checks {
		got := strings.Contains(toml, c.want)
		if got != c.present {
			if c.present {
				t.Fatalf("codex TOML missing %s (%q);\nactual file:\n%s", c.desc, c.want, toml)
			} else {
				t.Fatalf("codex TOML unexpectedly contains %s (%q);\nactual file:\n%s", c.desc, c.want, toml)
			}
		}
	}
}

// TestCodexSkillsConfig_CodexToCanonicalAndClaude verifies that a codex TOML
// with [[skills.config]] entries round-trips through graft sync agents into the
// canonical agent.yaml (skills list) and the claude-code file (frontmatter
// skills block), and that the codex TOML itself is preserved intact.
func TestCodexSkillsConfig_CodexToCanonicalAndClaude(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "init")

	writeFile(t, root, ".codex/agents/reviewer.toml", `name = "reviewer"
description = "A code reviewer"
model = "gpt-4o"
developer_instructions = "Review code carefully.\n"

[skills]
  [[skills.config]]
    name = "lint-fix"
    enabled = true

  [[skills.config]]
    name = "test-runner"
    enabled = true
`)
	gitCommitAll(t, root, "add reviewer")

	var res runResultJSON
	decodeJSON(t, mustGraft(t, root, "sync", "agents", "-o", "json"), &res)
	if res.Status != "done" {
		t.Fatalf("sync status=%q, want done", res.Status)
	}
	if !containsStr(res.Changed, "reviewer") {
		t.Fatalf("changed=%v, want reviewer", res.Changed)
	}

	canonYAML := readFile(t, root, ".graft/agents/reviewer/agent.yaml")
	claudeMD := readFile(t, root, ".claude/agents/reviewer.md")
	codexTOML := readFile(t, root, ".codex/agents/reviewer.toml")

	// Canonical YAML must list both skills.
	for _, skill := range []string{"lint-fix", "test-runner"} {
		if !strings.Contains(canonYAML, skill) {
			t.Fatalf("canonical agent.yaml missing skill %q;\nactual file:\n%s", skill, canonYAML)
		}
	}

	// Claude-code file must carry both skill names (in frontmatter skills: block).
	for _, skill := range []string{"lint-fix", "test-runner"} {
		if !strings.Contains(claudeMD, skill) {
			t.Fatalf("claude-code agent file missing skill %q;\nactual file:\n%s", skill, claudeMD)
		}
	}

	// Codex TOML round-trips: structure and skill names preserved.
	tomlChecks := []struct {
		desc string
		want string
	}{
		{"[[skills.config]] block", "[[skills.config]]"},
		{"lint-fix skill name", `name = "lint-fix"`},
		{"test-runner skill name", `name = "test-runner"`},
		{"enabled = true", "enabled = true"},
	}
	for _, c := range tomlChecks {
		if !strings.Contains(codexTOML, c.want) {
			t.Fatalf("codex TOML missing %s (%q) after round-trip;\nactual file:\n%s", c.desc, c.want, codexTOML)
		}
	}
}
