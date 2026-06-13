package codex

import (
	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for OpenAI (the backend powering codex).
const modelsDevKey = "openai"

// Models returns the known codex model ids sourced from models.dev
// (the OpenAI provider entry), falling back to the embedded catalog
// baseline when offline with no cache.  It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	var baseline []string
	if cat, err := catalog.LoadOnce(); err == nil {
		baseline, _ = cat.ModelsFor("codex")
	}
	return models.ModelsForWithCatalog(modelsDevKey, baseline, models.Config{})
}
