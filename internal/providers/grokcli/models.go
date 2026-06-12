package grokcli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for xAI (the backend powering grok-cli).
const modelsDevKey = "xai"

// Models returns the known grok-cli model ids sourced from models.dev
// (the xAI provider entry).  It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	return models.ModelsFor(modelsDevKey, models.Config{})
}
