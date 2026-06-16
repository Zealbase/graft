package continueprov

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

const modelsDevKey = "anthropic"

func (Provider) Models() ([]string, error) {
	baseline := []string{
		"anthropic/claude-sonnet-4", "anthropic/claude-opus-4", "anthropic/claude-haiku-4",
		"openai/gpt-4o", "openai/gpt-4o-mini", "mistral/mistral-large",
	}
	return models.ModelsForWithCatalog(modelsDevKey, baseline, models.Config{})
}
