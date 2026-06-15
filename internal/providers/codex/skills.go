package codex

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SkillProvider returns the Codex skills plugin. Codex supports skills via
// native vendor-neutral discovery: it auto-scans .agents/skills/ at CWD, repo
// root, and $HOME at startup — no symlink or config-entry action is required.
// Because the canonical store (.agents/skills/) IS the directory codex reads,
// graft marks codex as "linked (native)" without creating any symlink.
//
// Discovery scopes (all auto-scanned by codex, no config required):
//   - Repository (CWD / repo root): .agents/skills/          (vendor-neutral)
//   - User (home):                  ~/.agents/skills/         (vendor-neutral)
//   - User (codex-native):          ~/.codex/skills/          (codex-native)
//   - Admin:                        /etc/codex/skills         (codex-native)
//
// Source: https://developers.openai.com/codex/skills (official OpenAI docs);
// openai/codex config.schema.json (90k+ stars repo).
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string { return name }

// SkillsSupported returns true: codex fully supports the Agent Skills standard.
func (skills) SkillsSupported() bool { return true }

// NativeCanonicalDiscovery returns true: codex auto-scans .agents/skills/ (the
// canonical store) without any symlink or config-entry action from graft. The
// skills manager skips the symlink step and reports SkillNativeLinked.
func (skills) NativeCanonicalDiscovery() bool { return true }

// SkillDir returns "" because codex uses native discovery of the canonical
// .agents/skills/ store — there is no separate provider-scoped skills directory
// to symlink into; symlinks would be redundant and must not be created.
func (skills) SkillDir(string) string { return "" }

// HomeSkillDirs returns the codex-native and vendor-neutral home-scope skill
// directories that codex auto-scans. These are read-only sources for graft's
// home-scope install candidates; graft never writes symlinks into them.
//
// Directories returned (in scan order):
//   - ~/.codex/skills/   — codex-native user scope
//   - ~/.agents/skills/  — vendor-neutral user scope
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// DetectSkills returns the skills currently present in the codex-native home
// scope (~/.codex/skills/). The vendor-neutral .agents/skills/ is the canonical
// store and is already enumerated by the Store; scanning it here would produce
// duplicate entries, so only the codex-native home dir is scanned.
//
// Note: the project-scope canonical .agents/skills/ is NOT scanned here because
// it is the canonical store itself — the manager handles that separately.
func (skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	// codex auto-discovers the canonical .agents/skills/ natively — no project-
	// scope provider dir exists to scan. Return no project-scope refs; the
	// skills manager treats this provider as linked (native) for every canonical
	// skill and does not attempt a symlink.
	_ = root
	return nil, nil
}

// codexNativeSkillDir is a helper used by tests to confirm codex home skill dirs.
// It returns the codex-native user-scope skill dir for the given home.
func codexNativeSkillDir(home string) string {
	return filepath.Join(home, ".codex", "skills")
}

// canonicalHomeSkillDir returns the vendor-neutral home-scope skill dir.
func canonicalHomeSkillDir(home string) string {
	return filepath.Join(home, ".agents", "skills")
}

