package sync

// Track C — e2e SKILLS propagation through the sync engine (real git, real
// transform.Default(), real sqlite store). This is the regression guard for the
// keystone bug where foldProvider dropped CanonicalAgent.Skills during the fold,
// so an ingested provider's skills never reached the merged canonical, agent.yaml,
// the canonical hash, or the cross-provider fan-out.
//
// Level: e2e/sync — the full detect -> canonicalize -> merge -> apply lifecycle.
// Covers:
//   - PROPAGATION: a skill listed in ONE provider's agent file (claude-code
//     `skills:` frontmatter) canonicalizes into .graft/agents/<name>/agent.yaml,
//     is reflected in the canonical hash, and re-fans to OTHER providers
//     (claude-code `.md` keeps `skills:`, codex `.toml` emits a
//     [[skills.config]] name=<skill> enabled=true block).
//   - CROSS-PROVIDER: a skill added in one provider propagates to others.
//
// Without the foldProvider Skills block (FIX 1) these tests FAIL: the merged
// canonical drops Skills, so agent.yaml has no `skills:`, the codex `.toml` has
// no [[skills.config]], and the cross-provider assertion fails.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gitx"
	"github.com/Shaik-Sirajuddin/graft/internal/store"
	"github.com/Shaik-Sirajuddin/graft/internal/transform"
)

// writeClaudeAgentSkills drops a Claude Code agent file with a `skills:` YAML
// frontmatter list, so a skill enters the pipeline from claude-code.
func writeClaudeAgentSkills(t *testing.T, dir, name, desc string, skills []string, body string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: " + desc + "\nmodel: sonnet\nskills:\n")
	for _, s := range skills {
		b.WriteString("  - " + s + "\n")
	}
	b.WriteString("---\n" + body + "\n")
	writeFile(t, dir, filepath.Join(".claude", "agents", name+".md"), b.String())
}

// TestE2E_SkillsPropagation_ClaudeToOthers seeds a skill ONLY on the claude-code
// agent file, syncs, and asserts the merged canonical carries it (in agent.yaml
// and therefore in the canonical hash), the claude-code file keeps it, and codex
// re-fans it as an enabled [[skills.config]] entry.
//
// FAILS without FIX 1 (foldProvider drops Skills): the merged canonical would have
// no Skills, agent.yaml no `skills:`, and the codex `.toml` no [[skills.config]].
func TestE2E_SkillsPropagation_ClaudeToOthers(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgentSkills(t, dir, "scribe", "a scribe", []string{"docs-editor"}, "You edit docs.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	eng := New(st, tr, gitx.New(dir), dir).SetHomeBase(t.TempDir())

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// 1. Merged canonical store holds the skill — proof it survived the fold.
	can, err := canonical.Load(canonical.AgentDir(dir, "scribe"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if !sliceContains(can.Skills, "docs-editor") {
		t.Fatalf("canonical Skills = %v, want to contain docs-editor (FIX 1: foldProvider must fold Skills)", can.Skills)
	}

	// 2. agent.yaml on disk carries `skills:` (and so feeds the canonical hash —
	//    canonical.Load read it back, and canonical.Hash includes Skills).
	yamlData, err := os.ReadFile(filepath.Join(canonical.AgentDir(dir, "scribe"), "agent.yaml"))
	if err != nil {
		t.Fatalf("read agent.yaml: %v", err)
	}
	if !strings.Contains(string(yamlData), "docs-editor") {
		t.Fatalf("agent.yaml missing skill docs-editor:\n%s", yamlData)
	}
	// The skill is reflected in the canonical hash: a canonical WITHOUT the skill
	// hashes differently from the one we persisted.
	withSkill := canonical.Hash(can)
	noSkill := can
	noSkill.Skills = nil
	if withSkill == canonical.Hash(noSkill) {
		t.Fatal("canonical hash does not reflect Skills — fold/hash wiring broken")
	}

	// 3. claude-code file keeps the skill.
	claudeData, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "scribe.md"))
	if err != nil {
		t.Fatalf("read claude file: %v", err)
	}
	if !strings.Contains(string(claudeData), "docs-editor") {
		t.Fatalf("claude-code file lost the skill:\n%s", claudeData)
	}

	// 4. Cross-provider fan-out: codex emits the skill as an enabled config entry.
	codexData, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "scribe.toml"))
	if err != nil {
		t.Fatalf("read codex file: %v", err)
	}
	cs := string(codexData)
	if !strings.Contains(cs, "[[skills.config]]") ||
		!strings.Contains(cs, `name = "docs-editor"`) ||
		!strings.Contains(cs, "enabled = true") {
		t.Fatalf("codex did not re-fan the skill as an enabled [[skills.config]] entry:\n%s", cs)
	}
}

// TestE2E_SkillsCrossProvider_EditPropagates establishes a synced agent, then
// EDITS the skills list in one provider (claude-code) and asserts the change
// propagates to another provider (codex) on the next sync.
//
// FAILS without FIX 1: the edited skill is dropped during the fold, so codex never
// receives it.
func TestE2E_SkillsCrossProvider_EditPropagates(t *testing.T) {
	requireGit(t)
	dir := newWorkspace(t)
	writeClaudeAgentSkills(t, dir, "dev", "a developer", []string{"docs-editor"}, "Body.")

	st, err := store.Open(filepath.Join(dir, "graft.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tr := transform.Default()
	eng := New(st, tr, gitx.New(dir), dir).SetHomeBase(t.TempDir())

	if res, err := eng.Run(contract.SyncOpts{}); err != nil || res.Status != contract.RunDone {
		t.Fatalf("first sync: res=%+v err=%v", res, err)
	}

	// Edit claude-code: add a second skill on top of the synced one.
	writeClaudeAgentSkills(t, dir, "dev", "a developer", []string{"docs-editor", "linter"}, "Body.")

	res, err := eng.Run(contract.SyncOpts{})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Status != contract.RunDone {
		t.Fatalf("status=%s, want done (conflicts=%v)", res.Status, res.Conflicts)
	}

	// Canonical unions both skills.
	can, err := canonical.Load(canonical.AgentDir(dir, "dev"))
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	if !sliceContains(can.Skills, "docs-editor") || !sliceContains(can.Skills, "linter") {
		t.Fatalf("canonical Skills = %v, want {docs-editor, linter}", can.Skills)
	}

	// codex received the newly-added skill.
	codexData, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "dev.toml"))
	if err != nil {
		t.Fatalf("read codex file: %v", err)
	}
	cs := string(codexData)
	if !strings.Contains(cs, `name = "linter"`) {
		t.Fatalf("edited skill 'linter' did not propagate to codex:\n%s", cs)
	}
}
