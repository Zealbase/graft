package grokcli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/skl"
)

// SkillProvider returns a non-supporting skills plugin: this provider does not
// participate in skills (no project-scoped <provider>/skills/ convention in the
// research capabilities), so the central registry silently skips it.
func SkillProvider() contract.SkillProvider { return skl.Unsupported{ProviderName: name} }
