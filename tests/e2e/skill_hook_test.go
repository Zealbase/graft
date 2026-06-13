package e2e

import (
	"testing"
)

// INIT / SYNC HOOK (plan-skills 05). The implicit skill-apply hook runs after a
// successful agent init/sync; config skills.enabled=false skips it. FS verifiers.

// `graft init` with a pre-seeded .agents/skills auto-links into all supporting
// providers (the hook is on by default).
func TestSkillHook_InitAutoLinks(t *testing.T) {
	root := newGitWorkspace(t)
	writeCanonicalSkill(t, root, "hello", "Pre-seeded before init.")

	mustGraft(t, root, "init")

	// Without any explicit skill command, all supporting providers are linked.
	for prov := range supportingSkillDirs {
		assertLinkedTo(t, provLinkPath(root, prov, "hello"), canonicalSkillDir(root, "hello"))
	}
	// Non-supporting dirs never created.
	for _, d := range nonSupportingSkillDirs {
		if exists(root, d) {
			t.Fatalf("init hook created a non-supporting dir: %s", d)
		}
	}
}

// `graft sync agents` re-applies the skill hook after agent sync.
func TestSkillHook_SyncAgentsReapplies(t *testing.T) {
	root := newGitWorkspace(t)
	// One agent (so sync agents has work) + one canonical skill.
	provisionClaudeAgent(t, root, "code-reviewer")
	writeCanonicalSkill(t, root, "hello", "body")
	// Disable the hook for init so the link is absent right after init.
	mustGraft(t, root, "config", "set", "--skills.enabled", "false")
	mustGraft(t, root, "init")
	for prov := range supportingSkillDirs {
		if _, ok := lstatMode(t, provLinkPath(root, prov, "hello")); ok {
			t.Fatalf("hook disabled but %s link present after init", prov)
		}
	}

	// Re-enable, commit the agent, run sync agents -> hook re-applies skills.
	mustGraft(t, root, "config", "set", "--skills.enabled", "true")
	gitCommitAll(t, root, "seed agent+skill")
	mustGraft(t, root, "sync", "agents")

	for prov := range supportingSkillDirs {
		assertLinkedTo(t, provLinkPath(root, prov, "hello"), canonicalSkillDir(root, "hello"))
	}
}

// config skills.enabled=false -> the init hook is skipped (no links created),
// while an explicit `skill sync` still works.
func TestSkillHook_DisabledSkipsHook(t *testing.T) {
	root := newGitWorkspace(t)
	writeCanonicalSkill(t, root, "hello", "body")
	mustGraft(t, root, "config", "set", "--skills.enabled", "false")

	mustGraft(t, root, "init")
	// Hook skipped: no provider links exist.
	for prov := range supportingSkillDirs {
		if _, ok := lstatMode(t, provLinkPath(root, prov, "hello")); ok {
			t.Fatalf("skills.enabled=false but init created a %s link", prov)
		}
	}

	// Explicit skill sync is independent of the hook and still links.
	mustGraft(t, root, "skill", "sync")
	for prov := range supportingSkillDirs {
		assertLinkedTo(t, provLinkPath(root, prov, "hello"), canonicalSkillDir(root, "hello"))
	}
}
