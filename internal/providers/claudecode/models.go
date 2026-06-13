package claudecode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for Anthropic (the backend powering claude-code).
const modelsDevKey = "anthropic"

// Models returns the known claude-code model ids sourced from models.dev
// (the Anthropic provider entry), falling back to the embedded catalog
// baseline when offline with no cache.  It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	var baseline []string
	if cat, err := catalog.Load(); err == nil {
		baseline, _ = cat.ModelsFor("claude-code")
	}
	return models.ModelsForWithCatalog(modelsDevKey, baseline, models.Config{})
}
