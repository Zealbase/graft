package antigravity

import (
	"testing"
)

// TestModelsOffline verifies that antigravity.Models() returns a non-empty
// slice from the embedded catalog baseline even without network access
// (catalog-only resolution — there is no models.dev key for antigravity).
func TestModelsOffline(t *testing.T) {
	ids, err := Provider{}.Models()
	if err != nil {
		t.Fatalf("Models() error: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("Models() returned empty slice; expected catalog baseline models")
	}
}
