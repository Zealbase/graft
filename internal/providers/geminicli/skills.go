package geminicli

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns the Gemini CLI skills plugin. Gemini CLI supports
// skills as <skill>/SKILL.md directories under .gemini/skills/ (project scope).
// Source: research skill-schema.json skillFile.paths (.gemini/skills/<skill>/SKILL.md).
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string                   { return name }
func (skills) SkillsSupported() bool          { return true }
func (skills) NativeCanonicalDiscovery() bool { return false }
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".gemini", "skills")
}

// HomeSkillDirs returns Gemini CLI's user-scope skill dirs: the gemini-native
// ~/.gemini/skills and the vendor-neutral ~/.agents/skills it reads natively.
// Source: research skillResolution searchPaths (user scope).
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
