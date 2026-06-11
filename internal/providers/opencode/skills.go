package opencode

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns the opencode skills plugin. opencode supports skills as
// <name>/SKILL.md directories under .opencode/skills/ (project scope). Source:
// research skill-schema.json skillFile.paths (.opencode/skills/<name>/SKILL.md).
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string          { return name }
func (skills) SkillsSupported() bool { return true }
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".opencode", "skills")
}
func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
