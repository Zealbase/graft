package grokcli

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SkillProvider returns the grok-cli skills plugin. grok-cli supports skills via
// native vendor-neutral discovery: it auto-scans .agents/skills/ at startup (the
// vendor-neutral convention from agentskills.io). No symlink or config-entry action
// is required; because the canonical store (.agents/skills/) IS the directory
// grok-cli reads, graft marks grok-cli as "linked (native)" without creating any
// symlink.
//
// Discovery scopes (all auto-scanned by grok-cli, no config required):
//   - Repository (CWD / repo root): .agents/skills/        (vendor-neutral)
//   - User (home, grok-native):     ~/.grok/skills/         (grok-native)
//   - User (home, vendor-neutral):  ~/.agents/skills/       (vendor-neutral)
//
// Source: grok-cli README + src/utils/skills.ts discoverSkills; ships bundled
// .agents/skills/*/SKILL.md entries; catalog skillResolution.supported: true.
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string { return name }

// SkillsSupported returns true: grok-cli fully supports the Agent Skills standard.
func (skills) SkillsSupported() bool { return true }

// NativeCanonicalDiscovery returns true: grok-cli auto-scans .agents/skills/ (the
// canonical store) without any symlink or config-entry action from graft. The
// skills manager skips the symlink step and reports SkillNativeLinked.
func (skills) NativeCanonicalDiscovery() bool { return true }

// SkillDir returns "" because grok-cli uses native discovery of the canonical
// .agents/skills/ store — there is no separate provider-scoped skills directory
// to symlink into; symlinks would be redundant and must not be created.
func (skills) SkillDir(string) string { return "" }

// HomeSkillDirs returns the grok-native and vendor-neutral home-scope skill
// directories that grok-cli auto-scans. These are read-only sources for graft's
// home-scope install candidates; graft never writes symlinks into them.
//
// Directories returned (in scan order):
//   - ~/.grok/skills/    — grok-native user scope
//   - ~/.agents/skills/  — vendor-neutral user scope
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".grok", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// DetectSkills returns the skills currently present in the grok-native home
// scope (~/.grok/skills/). The vendor-neutral .agents/skills/ is the canonical
// store and is already enumerated by the Store; scanning it here would produce
// duplicate entries, so only the grok-native home dir is scanned.
func (skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	// grok-cli auto-discovers the canonical .agents/skills/ natively — no project-
	// scope provider dir exists to scan. Return no project-scope refs; the
	// skills manager treats this provider as linked (native) for every canonical
	// skill and does not attempt a symlink.
	_ = root
	return nil, nil
}
