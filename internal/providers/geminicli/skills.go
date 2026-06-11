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

func (skills) Name() string          { return name }
func (skills) SkillsSupported() bool { return true }
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".gemini", "skills")
}
func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
