package opencode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// Models returns all model ids from the entire models.dev catalog.
// OpenCode is a multi-provider tool that can route to any of 75+ LLM
// backends; it uses models.dev internally as its model registry.  So the
// valid model set is the union of every provider entry in the catalog.
//
// It satisfies contract.ModelLister.  ErrUnavailable is returned when
// offline with no cache; callers must skip model validation in that case.
func (Provider) Models() ([]string, error) {
	return models.AllModels(models.Config{})
}
