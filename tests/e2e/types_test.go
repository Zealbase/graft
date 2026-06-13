package e2e

// Local JSON-decode mirrors of the contract result types. Kept here (rather than
// importing internal/contract) so the e2e suite asserts the actual JSON wire
// shape emitted by the binary, exactly as an external consumer would see it.

type initResult struct {
	Root    string `json:"root"`
	GitMode string `json:"git_mode"`
	Created bool   `json:"created"`
}

type agentStatus struct {
	Name      string          `json:"name"`
	Providers map[string]bool `json:"providers"`
	InSync    bool            `json:"in_sync"`
}

type statusReport struct {
	Agents             []agentStatus  `json:"agents"`
	OutOfSyncProviders map[string]int `json:"out_of_sync_providers"`
}

type runResultJSON struct {
	RunID     string   `json:"run_id"`
	Status    string   `json:"status"`
	Changed   []string `json:"changed"`
	Deleted   []string `json:"deleted"`
	Conflicts []struct {
		Path  string `json:"path"`
		Agent string `json:"agent"`
	} `json:"conflicts"`
}

type finding struct {
	Agent    string `json:"agent"`
	Provider string `json:"provider"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type configJSON struct {
	Sync struct {
		GitAuto bool `json:"gitAuto"`
	} `json:"sync"`
	Scope     string `json:"scope"`
	Providers struct {
		Enabled []string `json:"enabled"`
	} `json:"providers"`
	Theme string `json:"theme"`
}

// allProviders is the sorted set of the nine active provider ids graft emits.
// NOTE(2026-06-13): antigravity (agy) is intentionally absent — unregistered
// pending research spike. See tasks/_draft/antigravity-deferred.yaml.
var allProviders = []string{
	"claude-code",
	"codex",
	"cursor",
	"gemini-cli",
	"github-copilot",
	"goose",
	"grok-cli",
	"opencode",
	"roo-code",
}
