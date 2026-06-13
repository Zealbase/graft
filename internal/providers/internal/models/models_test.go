package models_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/models"
)

// fixtureData is a minimal models.dev-shaped payload for tests.
var fixtureData = map[string]any{
	"anthropic": map[string]any{
		"models": []any{
			map[string]any{"id": "claude-opus-4-8"},
			map[string]any{"id": "claude-sonnet-4-6"},
			map[string]any{"id": "claude-haiku-4-5"},
		},
	},
	"openai": map[string]any{
		"models": []any{
			map[string]any{"id": "gpt-5.5"},
			map[string]any{"id": "gpt-5.4"},
			map[string]any{"id": "gpt-5.3-codex"},
		},
	},
	"google": map[string]any{
		"models": []any{
			map[string]any{"id": "gemini-3.5-flash"},
			map[string]any{"id": "gemini-3.1-pro"},
			map[string]any{"id": "gemini-2.5-flash"},
		},
	},
	"xai": map[string]any{
		"models": []any{
			map[string]any{"id": "grok-4.3"},
			map[string]any{"id": "grok-4.20-0309-reasoning"},
		},
	},
}

func fixtureJSON(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(fixtureData)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mockHTTPClient returns the given body / status for every GET.
type mockHTTPClient struct {
	body   []byte
	status int
	err    error
}

func (m *mockHTTPClient) Get(_ string) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.status,
		Body:       io.NopCloser(bytes.NewReader(m.body)),
	}, nil
}

func okClient(t *testing.T) *mockHTTPClient {
	return &mockHTTPClient{body: fixtureJSON(t), status: http.StatusOK}
}

func offlineClient() *mockHTTPClient {
	return &mockHTTPClient{err: errors.New("dial tcp: connection refused")}
}

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

var epoch = time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

// TestCacheMissOnline: no cache file, network reachable — should fetch,
// persist the cache, and return the model ids.
func TestCacheMissOnline(t *testing.T) {
	dir := t.TempDir()
	cfg := models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch),
	}
	ids, err := models.ModelsFor("anthropic", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(ids, "claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5") {
		t.Errorf("missing expected model ids; got %v", ids)
	}
	// Cache file should now exist.
	cacheFile := filepath.Join(dir, "models-dev.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("cache file not written: %v", err)
	}
}

// TestCacheFresh: cache file present and within TTL — no network call.
func TestCacheFresh(t *testing.T) {
	dir := t.TempDir()
	cfg := models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch),
		TTL:      24 * time.Hour,
	}
	// Pre-warm cache as if fetched 1 hour ago.
	if _, err := models.ModelsFor("anthropic", models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch.Add(-1 * time.Hour)),
	}); err != nil {
		t.Fatal(err)
	}

	// Replace client with an offline one — should NOT be called.
	cfg.Client = offlineClient()
	ids, err := models.ModelsFor("anthropic", cfg)
	if err != nil {
		t.Fatalf("unexpected error with fresh cache: %v", err)
	}
	if !containsAll(ids, "claude-opus-4-8") {
		t.Errorf("unexpected ids from fresh cache: %v", ids)
	}
}

// TestCacheStale: cache exists but is older than TTL; network fails — should
// return the stale data without error (offline tolerance).
func TestCacheStale(t *testing.T) {
	dir := t.TempDir()
	// Pre-warm cache as if fetched 48 hours ago.
	if _, err := models.ModelsFor("openai", models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch.Add(-48 * time.Hour)),
		TTL:      24 * time.Hour,
	}); err != nil {
		t.Fatal(err)
	}

	// Now use an offline client with a "now" that makes the cache stale.
	ids, err := models.ModelsFor("openai", models.Config{
		CacheDir: dir,
		Client:   offlineClient(),
		Now:      fixedNow(epoch),
		TTL:      24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("stale-cache offline should not error: %v", err)
	}
	if !containsAll(ids, "gpt-5.5", "gpt-5.4") {
		t.Errorf("expected stale data to contain openai models; got %v", ids)
	}
}

// TestCacheMissOffline: no cache AND network down — must return ErrUnavailable.
func TestCacheMissOffline(t *testing.T) {
	dir := t.TempDir()
	ids, err := models.ModelsFor("anthropic", models.Config{
		CacheDir: dir,
		Client:   offlineClient(),
		Now:      fixedNow(epoch),
	})
	if !errors.Is(err, models.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable; got err=%v ids=%v", err, ids)
	}
}

// TestUnknownProvider: key not present in the response — nil ids, no error.
func TestUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	ids, err := models.ModelsFor("no-such-provider", models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch),
	})
	if err != nil {
		t.Fatalf("unexpected error for unknown key: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected nil ids for unknown provider; got %v", ids)
	}
}

// TestMultipleProviders: verify distinct keys return correct slices.
func TestMultipleProviders(t *testing.T) {
	dir := t.TempDir()
	cfg := func() models.Config {
		return models.Config{
			CacheDir: dir,
			Client:   okClient(t),
			Now:      fixedNow(epoch),
		}
	}
	googleIDs, err := models.ModelsFor("google", cfg())
	if err != nil {
		t.Fatal(err)
	}
	xaiIDs, err := models.ModelsFor("xai", cfg())
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(googleIDs, "gemini-3.5-flash", "gemini-2.5-flash") {
		t.Errorf("google ids wrong: %v", googleIDs)
	}
	if !containsAll(xaiIDs, "grok-4.3") {
		t.Errorf("xai ids wrong: %v", xaiIDs)
	}
}

// TestTTLRefresh: cache exists but is expired AND network is up — should
// refresh the cache with new data.
func TestTTLRefresh(t *testing.T) {
	dir := t.TempDir()
	// Pre-warm with stale "old" data (we'll notice the new models are missing).
	if _, err := models.ModelsFor("openai", models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch.Add(-48 * time.Hour)),
		TTL:      24 * time.Hour,
	}); err != nil {
		t.Fatal(err)
	}

	// Fetch again with a live client at "now" (cache is stale) — should refresh.
	ids, err := models.ModelsFor("openai", models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch),
		TTL:      24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error on TTL refresh: %v", err)
	}
	if !containsAll(ids, "gpt-5.5") {
		t.Errorf("expected refreshed data; got %v", ids)
	}
}

// TestAllModels: AllModels returns the union of all provider model ids.
func TestAllModels(t *testing.T) {
	dir := t.TempDir()
	ids, err := models.AllModels(models.Config{
		CacheDir: dir,
		Client:   okClient(t),
		Now:      fixedNow(epoch),
	})
	if err != nil {
		t.Fatalf("AllModels error: %v", err)
	}
	// Fixture has anthropic + openai + google + xai models.
	if !containsAll(ids, "claude-opus-4-8", "gpt-5.5", "gemini-3.5-flash", "grok-4.3") {
		t.Errorf("AllModels missing expected ids; got %v", ids)
	}
}

// TestAllModelsOfflineNoCache: no cache AND offline -> ErrUnavailable.
func TestAllModelsOfflineNoCache(t *testing.T) {
	dir := t.TempDir()
	_, err := models.AllModels(models.Config{
		CacheDir: dir,
		Client:   offlineClient(),
		Now:      fixedNow(epoch),
	})
	if !errors.Is(err, models.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable; got %v", err)
	}
}

// TestModelsForWithCatalog_EmptyFetch verifies that ModelsForWithCatalog falls
// back to the catalog baseline when models.dev responds 200 but the provider
// key is absent (empty ids). Without this fix the baseline was skipped,
// triggering spurious "unknown model" warnings for cursor/antigravity etc.
func TestModelsForWithCatalog_EmptyFetch(t *testing.T) {
	dir := t.TempDir()
	cfg := models.Config{
		CacheDir: dir,
		Client:   okClient(t), // fixture has no "cursor" key
		Now:      fixedNow(epoch),
	}
	baseline := []string{"composer-2.5", "claude-fable-5"}
	ids, err := models.ModelsForWithCatalog("cursor", baseline, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(ids, "composer-2.5", "claude-fable-5") {
		t.Errorf("expected catalog baseline fallback; got %v", ids)
	}
}

// TestAllModelsWithCatalog_EmptyFetch verifies that AllModelsWithCatalog falls
// back to catalogBaseline when models.dev responds 200 but returns an empty
// union (degenerate case; defensive).
func TestAllModelsWithCatalog_EmptyFetch(t *testing.T) {
	dir := t.TempDir()
	// Use a client that returns a valid JSON object with zero provider entries.
	emptyJSON := []byte(`{}`)
	cfg := models.Config{
		CacheDir: dir,
		Client:   &mockHTTPClient{body: emptyJSON, status: 200},
		Now:      fixedNow(epoch),
	}
	baseline := []string{"fallback-model"}
	ids, err := models.AllModelsWithCatalog(baseline, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(ids, "fallback-model") {
		t.Errorf("expected catalog baseline fallback; got %v", ids)
	}
}

// containsAll checks that all want strings appear in got.
func containsAll(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
