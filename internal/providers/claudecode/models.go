package claudecode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for Anthropic (the backend powering claude-code).
const modelsDevKey = "anthropic"

// Models returns the known claude-code model ids sourced from models.dev
// (the Anthropic provider entry).  It satisfies contract.ModelLister.
//
// Uses the default Config (real HTTP client, real clock, XDG cache dir
// ~/.cache/graft/models).  ErrUnavailable is returned when the cache is
// absent and the network is unreachable; callers must skip model validation
// in that case.
func (Provider) Models() ([]string, error) {
	return models.ModelsFor(modelsDevKey, models.Config{})
}
