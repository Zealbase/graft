package contract

// Skill is a canonical skill stored under .agent/skills/<name>/ (SKILL.md + assets).
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
	SkillLinked      SkillLinkState = "linked"      // symlink -> canonical skill dir
	SkillMissing     SkillLinkState = "missing"     // no entry at the provider path
	SkillWrongLink   SkillLinkState = "wrong-link"  // symlink -> some other target
	SkillConflict    SkillLinkState = "conflict"    // a real dir/file is present (needs --override)
	SkillUnsupported SkillLinkState = "unsupported" // provider does not support skills
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
	SkillDir(root string) string
	// DetectSkills returns the skills already present in this provider's dir.
	DetectSkills(root string) ([]SkillRef, error)
}
