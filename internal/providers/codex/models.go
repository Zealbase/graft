package codex

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for OpenAI (the backend powering codex).
const modelsDevKey = "openai"

// Models returns the known codex model ids sourced from models.dev
// (the OpenAI provider entry).  It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	return models.ModelsFor(modelsDevKey, models.Config{})
}
