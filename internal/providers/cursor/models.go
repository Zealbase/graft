package cursor

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/catalog"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

var _ contract.ModelLister = Provider{}

// Models returns the known cursor model ids from the embedded catalog baseline.
// Cursor has no stable public model-list API and is not listed on models.dev,
// so catalog-only resolution is used without a models.dev lookup.
// It satisfies contract.ModelLister.
func (Provider) Models() ([]string, error) {
	cat, err := catalog.LoadOnce()
	if err != nil {
		return nil, fmt.Errorf("cursor: load catalog: %w", err)
	}
	return cat.ModelsFor("cursor")
}
