package claudecode

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns the Claude Code skills plugin. Claude Code supports
// skills as self-contained <skill>/SKILL.md directories under .claude/skills/
// (project scope). Source: research skill-schema.json skillFile.paths.
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string          { return name }
func (skills) SkillsSupported() bool { return true }
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".claude", "skills")
}
func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
