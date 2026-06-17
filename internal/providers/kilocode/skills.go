package kilocode

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns the Kilo Code skills plugin. Kilo Code supports skills
// under .kilo/skills/ (primary project scope) with additional compat dirs.
func SkillProvider() contract.SkillProvider { return skills{} }

type skills struct{}

func (skills) Name() string                   { return name }
func (skills) SkillsSupported() bool          { return true }
func (skills) NativeCanonicalDiscovery() bool { return false }

// SkillDir returns the primary project-scope skill directory for Kilo Code.
func (skills) SkillDir(root string) string {
	return filepath.Join(root, ".kilo", "skills")
}

// HomeSkillDirs returns Kilo Code's personal skill dirs under home.
func (skills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".kilo", "skills"),
		filepath.Join(home, ".config", "kilo", "skills"),
		filepath.Join(home, ".kilocode", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// DetectSkills scans all project skill dirs and returns deduped skill refs.
// Searches: .kilo/skills, .kilocode/skills, .agents/skills, .claude/skills
func (s skills) DetectSkills(root string) ([]contract.SkillRef, error) {
	dirs := []string{
		filepath.Join(root, ".kilo", "skills"),
		filepath.Join(root, ".kilocode", "skills"),
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".claude", "skills"),
	}

	seen := make(map[string]bool)
	var out []contract.SkillRef
	for _, dir := range dirs {
		refs, err := skl.Detect(name, dir)
		if err != nil {
			// Skip dirs that fail to read; missing dirs are already handled inside
			// skl.Detect (returns nil,nil for os.IsNotExist). Any other per-dir
			// error is non-fatal: log the skip and continue scanning remaining dirs.
			continue
		}
		for _, r := range refs {
			if !seen[r.Name] {
				seen[r.Name] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}
