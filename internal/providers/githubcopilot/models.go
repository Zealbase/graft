package githubcopilot

import (
	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

var _ contract.ModelLister = Provider{}

// models.dev provider key for GitHub Copilot models.
// models.dev uses the key "github" for GitHub-hosted models.
const modelsDevKey = "github"

// Models returns the known github-copilot model ids sourced from models.dev
// (the GitHub provider entry), falling back to the embedded catalog
// baseline when offline with no cache.  It satisfies contract.ModelLister.
//
// Note: the authoritative source is https://models.github.ai/catalog/models
// (requires a GitHub token).  models.dev mirrors the public catalog under the
// "github" key and is used here to avoid requiring user credentials.
// ErrUnavailable is returned when offline with no cache and no baseline;
// callers skip validation in that case.
func (Provider) Models() ([]string, error) {
	var baseline []string
	if cat, err := catalog.Load(); err == nil {
		baseline, _ = cat.ModelsFor("github-copilot")
	}
	return models.ModelsForWithCatalog(modelsDevKey, baseline, models.Config{})
}
