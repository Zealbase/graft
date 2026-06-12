package geminicli

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for Google (the backend powering gemini-cli).
const modelsDevKey = "google"

// Models returns the known gemini-cli model ids sourced from models.dev
// (the Google provider entry).  It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	return models.ModelsFor(modelsDevKey, models.Config{})
}
