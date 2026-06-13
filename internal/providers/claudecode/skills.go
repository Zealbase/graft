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

// HomeSkillDirs returns Claude Code's personal skill dir ~/.claude/skills.
// Source: research skillResolution searchPaths (personal scope). Claude does NOT
// read .agents/skills, so the vendor-neutral home dir is not listed here.
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".claude", "skills")}
}

func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
