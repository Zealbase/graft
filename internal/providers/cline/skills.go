package clineprov

import (
	"path/filepath"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns the Cline skills plugin.
func SkillProvider() contract.SkillProvider { return clineSkills{} }

type clineSkills struct{}

func (clineSkills) Name() string                   { return name }
func (clineSkills) SkillsSupported() bool          { return true }
func (clineSkills) NativeCanonicalDiscovery() bool { return true }

// SkillDir returns the primary project-scope skill directory for Cline.
func (clineSkills) SkillDir(root string) string {
	return filepath.Join(root, ".cline", "skills")
}

// HomeSkillDirs returns Cline's personal skill dirs under home.
func (clineSkills) HomeSkillDirs(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".cline", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// DetectSkills scans all project skill dirs and returns deduped skill refs.
func (s clineSkills) DetectSkills(root string) ([]contract.SkillRef, error) {
	dirs := []string{
		filepath.Join(root, ".cline", "skills"),
		filepath.Join(root, ".agents", "skills"),
	}

	seen := make(map[string]bool)
	var out []contract.SkillRef
	for _, dir := range dirs {
		refs, err := skl.Detect(name, dir)
		if err != nil {
			return nil, err
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
