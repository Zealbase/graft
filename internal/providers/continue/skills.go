package continueprov

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns a contract.SkillProvider for Continue.
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string                   { return name }
func (skills) SkillsSupported() bool          { return true }
func (skills) NativeCanonicalDiscovery() bool { return false }
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".continue", "skills")
}

func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".continue", "skills")}
}

func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	return skl.Detect(name, s.SkillDir(root))
}
