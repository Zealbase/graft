package contract

// OmniRef records the omni-agent linkage for one canonical agent. It is
// persisted alongside the agent (in .graft/agents/<name>/.meta.json); an empty
// Ref means the agent has no omni header. The fields are additive and optional:
// nothing here changes the on-wire shape of CanonicalAgent.
type OmniRef struct {
	Ref       string `json:"ref"`       // resolved omni reference (defaults to agent name)
	Applied   bool   `json:"applied"`   // true once sys-instructions were prepended
	Supported bool   `json:"supported"` // result of the last support-check
}

// HydrateView is the machine-readable resolution of one agent for a runner.
// It is the consumer contract a host reads to spin an agent runner with the
// hydrated model/tools/sandbox.
//
// Sandbox is per-requested-provider; it is empty when the view is not
// provider-scoped.
type HydrateView struct {
	Name    string            `json:"name"`
	Model   string            `json:"model"`
	Tools   []string          `json:"tools"`
	Sandbox map[string]string `json:"sandbox,omitempty"`
	Skills  []string          `json:"skills,omitempty"`
	MCP     []string          `json:"mcp,omitempty"`
}

// DetectReport answers "is this a graft workspace?" without mutating anything.
// It is the side-effect-free probe a host runs before consuming graft.
type DetectReport struct {
	IsWorkspace bool   `json:"isWorkspace"`    // .graft/ present
	Initialized bool   `json:"initialized"`    // store + git mode ready
	Root        string `json:"root"`           // workspace root
	Hint        string `json:"hint,omitempty"` // e.g. "run graft init first"
}

// OmniResolver turns an omni ref into sys-instructions text. The interface is
// frozen now; v0..0.7 ships it alongside a default implementation that reports
// Supported()=false, so omni refs are recorded but never applied until a
// resolver capability ships.
type OmniResolver interface {
	// Supported reports whether this resolver can resolve the given ref.
	Supported(ref string) bool
	// Resolve returns the sys-instructions text for the given ref.
	Resolve(ref string) (sysInstructions string, err error)
}
