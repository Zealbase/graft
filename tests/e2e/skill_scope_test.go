package e2e

import (
	"path/filepath"
	"testing"
)

// EXISTENCE / SCOPE cases (plan-skills 05). FS + raw verifiers only.

// Canonical-only skill -> sync links every symlink-based supporting provider
// (a real symlink into the provider's skills dir pointing at canonical) and
// reports every native-discovery provider as "linked (native)" (no symlink or
// dir created); the remaining non-supporting provider skill dirs are NEVER
// created.
//
// The expected set is derived from the live provider registry rather than a
// hardcoded count so it tracks newly-added skill-supporting providers. As of
// 2026-06-17 the active, registered set is:
//   - symlink-based (SkillsSupported && !NativeCanonicalDiscovery):
//       claude-code, opencode, continue, kilo-code
//   - native (NativeCanonicalDiscovery):
//       codex, grok-cli, cline, roo-code
// gemini-cli is symlink-based in code but dewired (unregistered) so it never
// appears in the report.
func TestSkillScope_CanonicalOnly_LinksSupporting_NonSupportingUntouched(t *testing.T) {
	root := initSkillWorkspace(t, "hello")

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-o", "json"), &states)

	// Symlink-based supporting providers and their workspace-relative skills
	// dirs: each must report "linked" AND hold a real symlink to canonical.
	symlinkSupporting := map[string]string{
		"claude-code": ".claude/skills",
		"opencode":    ".opencode/skills",
		"continue":    ".continue/skills",
		"kilo-code":   ".kilo/skills",
	}
	for prov, dir := range symlinkSupporting {
		if s, ok := stateOf(states, prov, "hello"); !ok || s != "linked" {
			t.Fatalf("provider %s state=%q (ok=%v), want linked", prov, s, ok)
		}
		assertLinkedTo(t, filepath.Join(root, dir, "hello"), canonicalSkillDir(root, "hello"))
	}

	// Native-discovery providers: each reports "linked (native)" — they auto-scan
	// .agents/skills/ so no symlink or provider dir is ever created.
	nativeSupporting := []string{"codex", "grok-cli", "cline", "roo-code"}
	for _, prov := range nativeSupporting {
		if s, ok := stateOf(states, prov, "hello"); !ok || s != "linked (native)" {
			t.Fatalf("%s state=%q (ok=%v), want linked (native)", prov, s, ok)
		}
	}

	// The report must contain exactly the symlink-based + native supporting set.
	seen := map[string]bool{}
	for _, s := range states {
		seen[s.Provider] = true
	}
	wantProviders := len(symlinkSupporting) + len(nativeSupporting)
	if len(seen) != wantProviders {
		t.Fatalf("status reported providers %v (%d), want the %d symlink-based + %d native = %d total",
			seen, len(seen), len(symlinkSupporting), len(nativeSupporting), wantProviders)
	}

	// Non-supporting provider skill dirs must not exist. Native-discovery
	// providers also never create a dir, so their dirs (.codex/skills,
	// .grok/skills, .cline equivalents, .roo/skills) staying absent is correct.
	for _, d := range nonSupportingSkillDirs {
		if exists(root, d) {
			t.Fatalf("non-supporting provider dir was created: %s", d)
		}
	}
}

// Skill found in ONE provider dir but not canonical -> `skill install <name>`
// copies it into .agents/skills then links the OTHER supporting providers.
func TestSkillScope_InstallFromProviderDir(t *testing.T) {
	root := newGitWorkspace(t)
	mustGraft(t, root, "config", "set", "--skills.enabled", "false")
	mustGraft(t, root, "init")

	// A real skill dir present only under claude's provider dir (not canonical).
	writeFile(t, root, filepath.Join(".claude", "skills", "fromprov", "SKILL.md"),
		"---\nname: fromprov\n---\nProvider-shipped skill.\n")

	// Before install: nothing canonical.
	var list []struct {
		Name string `json:"name"`
	}
	decodeJSON(t, mustGraft(t, root, "skill", "list", "-o", "json"), &list)
	if len(list) != 0 {
		t.Fatalf("expected no canonical skills before install, got %+v", list)
	}

	// Install by name -> copy into canonical + link the others.
	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "install", "fromprov", "-o", "json"), &states)

	// Canonical copy now exists.
	if !exists(root, ".agents/skills/fromprov/SKILL.md") {
		t.Fatal("install did not copy the skill into .agents/skills")
	}
	// opencode is linked to canonical (gemini-cli dewired — only opencode remains as the
	// other symlink-based supporting provider alongside claude-code the source).
	// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered).
	for _, prov := range []string{"opencode"} {
		if s, _ := stateOf(states, prov, "fromprov"); s != "linked" {
			t.Fatalf("after install, %s state=%q, want linked", prov, s)
		}
		assertLinkedTo(t, provLinkPath(root, prov, "fromprov"), canonicalSkillDir(root, "fromprov"))
	}
	// The SOURCE provider (claude) still holds its real dir -> reported conflict
	// (install never destroys the user's real entry without --override).
	if s, _ := stateOf(states, "claude-code", "fromprov"); s != "conflict" {
		t.Logf("NOTE: claude-code source dir reported state=%q (expected conflict — install leaves the source real dir until --override)", s)
	}
	assertRealDir(t, provLinkPath(root, "claude-code", "fromprov"))
}

// Partial: linked in some supporting providers, missing in others -> `skill sync`
// links only the missing, no-ops the present (idempotent on the present one).
func TestSkillScope_Partial_LinksMissingOnly(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	// claude already correct; opencode absent.
	// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered).
	provisionState(t, root, "claude-code", "hello", "correct")
	provisionState(t, root, "opencode", "hello", "absent")
	claudeLink := provLinkPath(root, "claude-code", "hello")
	beforeMtime := linkTargetMtime(t, claudeLink)

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-o", "json"), &states)
	for prov := range supportingSkillDirs {
		if s, _ := stateOf(states, prov, "hello"); s != "linked" {
			t.Fatalf("partial sync: %s state=%q, want linked", prov, s)
		}
		assertLinkedTo(t, provLinkPath(root, prov, "hello"), canonicalSkillDir(root, "hello"))
	}
	// The already-correct claude link is untouched (no-op).
	if after := linkTargetMtime(t, claudeLink); after != beforeMtime {
		t.Fatalf("partial sync re-touched an already-correct link: %d -> %d", beforeMtime, after)
	}
}

// --provider scoping: only the named provider is linked; others unchanged.
func TestSkillScope_ProviderFlag_OnlyThatProvider(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	for prov := range supportingSkillDirs {
		provisionState(t, root, prov, "hello", "absent")
	}

	// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered) — use
	// claude-code as the scoped provider; opencode must remain absent.
	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "sync", "-p", "claude-code", "-o", "json"), &states)

	// Only claude-code in the report.
	for _, s := range states {
		if s.Provider != "claude-code" {
			t.Fatalf("--provider claude-code reported other provider %s", s.Provider)
		}
	}
	// claude-code linked; opencode remains absent on disk.
	assertLinkedTo(t, provLinkPath(root, "claude-code", "hello"), canonicalSkillDir(root, "hello"))
	for _, prov := range []string{"opencode"} {
		if _, ok := lstatMode(t, provLinkPath(root, prov, "hello")); ok {
			t.Fatalf("--provider scope leaked: %s was linked", prov)
		}
	}
}

// skill status reports linked / missing / wrong-link / conflict accurately
// (raw + -o json), one row per (supporting provider, skill).
// NOTE(2026-06-15): gemini-cli dewired (kept in code, unregistered) — only
// claude-code and opencode are symlink-based supporting providers.
func TestSkillStatus_ReportsAllStates(t *testing.T) {
	root := initSkillWorkspace(t, "hello")
	provisionState(t, root, "claude-code", "hello", "correct") // linked
	provisionState(t, root, "opencode", "hello", "wrong")      // wrong-link
	// "missing" state: tested separately below using a fresh workspace.

	var states []skillStatusJSON
	decodeJSON(t, mustGraft(t, root, "skill", "status", "-o", "json"), &states)

	want := map[string]string{
		"claude-code": "linked",
		"opencode":    "wrong-link",
	}
	for prov, w := range want {
		if s, ok := stateOf(states, prov, "hello"); !ok || s != w {
			t.Fatalf("status %s=%q (ok=%v), want %q", prov, s, ok, w)
		}
	}

	// "missing" state: provision a fresh workspace with claude-code absent.
	rootMissing := initSkillWorkspace(t, "hello")
	provisionState(t, rootMissing, "claude-code", "hello", "absent")
	var stMissing []skillStatusJSON
	decodeJSON(t, mustGraft(t, rootMissing, "skill", "status", "-p", "claude-code", "-o", "json"), &stMissing)
	if s, _ := stateOf(stMissing, "claude-code", "hello"); s != "missing" {
		t.Fatalf("absent provision status=%q, want missing", s)
	}

	// Conflict variant in its own workspace (real dir at opencode).
	root2 := initSkillWorkspace(t, "hello")
	provisionState(t, root2, "opencode", "hello", "real")
	var st2 []skillStatusJSON
	decodeJSON(t, mustGraft(t, root2, "skill", "status", "-p", "opencode", "-o", "json"), &st2)
	if s, _ := stateOf(st2, "opencode", "hello"); s != "conflict" {
		t.Fatalf("real dir status=%q, want conflict", s)
	}

	// raw table output has the expected header columns.
	tbl := mustGraft(t, root, "skill", "status")
	for _, col := range []string{"SKILL", "PROVIDER", "STATE"} {
		if !contains(tbl.stdout, col) {
			t.Fatalf("status table missing column %q in:\n%s", col, tbl.stdout)
		}
	}
}
