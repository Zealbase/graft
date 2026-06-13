package contract

// Skill is a canonical skill stored under .agents/skills/<name>/ (SKILL.md + assets).
// Skills are reconciled by SYMLINK (not transform/merge): one canonical dir is
// symlinked into each supporting provider's skills dir.
type Skill struct {
	Name string `json:"name"`
	Dir  string `json:"dir"` // absolute path to the canonical skill dir
}

// SkillRef locates a skill instance — canonical (Provider == "") or found in a
// specific provider's skills dir.
type SkillRef struct {
	Name     string `json:"name"`
	Provider string `json:"provider,omitempty"`
	Path     string `json:"path"`
}

// SkillLinkState is the live filesystem state of one skill for one provider.
// It is always computed live (lstat/readlink), never persisted.
type SkillLinkState string

const (
	SkillLinked      SkillLinkState = "linked"      // symlink -> canonical skill dir (target exists)
	SkillMissing     SkillLinkState = "missing"     // no entry at the provider path
	SkillWrongLink   SkillLinkState = "wrong-link"  // symlink -> some other (existing) target
	SkillConflict    SkillLinkState = "conflict"    // a real dir/file is present (needs --override)
	SkillUnsupported SkillLinkState = "unsupported" // provider does not support skills
	// SkillDead is a broken/dangling symlink: the entry IS a symlink but its
	// target does not exist (e.g. the canonical skill was deleted, leaving the
	// provider symlink pointing at a now-missing .agents/skills/<name>). Such a
	// link is NOT "linked"; sync prunes the dangling symlink (and only that).
	SkillDead SkillLinkState = "dead"
)

// SkillStatus is the per-provider link state for one canonical skill.
type SkillStatus struct {
	Skill    string         `json:"skill"`
	Provider string         `json:"provider"`
	State    SkillLinkState `json:"state"`
	LinkPath string         `json:"link_path,omitempty"`
}

// SkillOpts parameterizes install/sync.
type SkillOpts struct {
	Override bool   // replace a non-symlink entry with a symlink
	Provider string // limit to a single provider (empty = all supporting)
	Yes      bool   // non-interactive: auto-install missing referenced skills
}

// SkillProvider is implemented (optionally) by providers that support skills.
// The central skills registry acts ONLY on providers returning SkillsSupported()
// == true; non-supporting providers are silently skipped. A provider that does
// not support skills need not implement this interface at all.
type SkillProvider interface {
	// Name matches the provider id (e.g. "claude-code").
	Name() string
	// SkillsSupported declares whether this provider participates in skills.
	SkillsSupported() bool
	// SkillDir returns the provider's skills directory under the workspace root.
	// This is the project-scope dir that receives the canonical symlinks.
	SkillDir(root string) string
	// HomeSkillDirs returns the provider's home/user-scope skill directories
	// (e.g. ~/.claude/skills). Personal skills found here are surfaced as
	// install candidates so they can be copied into the canonical store; the
	// home dirs are read-only sources and never receive symlinks. home is the
	// resolved user home directory. May return nil. Mirrors how antigravity
	// agents are home-scoped.
	HomeSkillDirs(home string) []string
	// DetectSkills returns the skills already present in this provider's
	// project-scope dir.
	DetectSkills(root string) ([]SkillRef, error)
}
