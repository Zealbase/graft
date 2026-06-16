package roocode

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// SkillProvider returns the roo-code skills plugin. roo-code supports skills via
// native vendor-neutral discovery: it auto-scans .roo/skills/ and .agents/skills/
// at startup. No symlink or config-entry action is required; graft marks roo-code
// as "linked (native)" without creating any symlink.
//
// Discovery scopes (all auto-scanned by roo-code, no config required):
//   - Repository (CWD / repo root): .agents/skills/        (vendor-neutral)
//   - User (home, roo-native):      ~/.roo/skills/          (roo-native)
//   - User (home, vendor-neutral):  ~/.agents/skills/       (vendor-neutral)
//
// Source: roo-code docs + catalog skillResolution.supported: true.
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string { return name }

// SkillsSupported returns true: roo-code fully supports the Agent Skills standard.
func (skills) SkillsSupported() bool { return true }

// NativeCanonicalDiscovery returns true: roo-code auto-scans .agents/skills/ (the
// canonical store) without any symlink or config-entry action from graft. The
// skills manager skips the symlink step and reports SkillNativeLinked.
func (skills) NativeCanonicalDiscovery() bool { return true }

// SkillDir returns "" because roo-code uses native discovery of the canonical
// .agents/skills/ store — there is no separate provider-scoped skills directory
// to symlink into; symlinks would be redundant and must not be created.
func (skills) SkillDir(string) string { return "" }

// HomeSkillDirs returns the roo-native and vendor-neutral home-scope skill
// directories that roo-code auto-scans. These are read-only sources for graft's
// home-scope install candidates; graft never writes symlinks into them.
//
// Directories returned (in scan order):
//   - ~/.roo/skills/     — roo-native user scope
//   - ~/.agents/skills/  — vendor-neutral user scope
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".roo", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// DetectSkills returns the skills currently present in the project scope.
// roo-code auto-discovers the canonical .agents/skills/ natively — no project-
// scope provider dir exists to scan. Return no project-scope refs; the
// skills manager treats this provider as linked (native) for every canonical
// skill and does not attempt a symlink.
func (skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	_ = root
	return nil, nil
}
