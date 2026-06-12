// Package models provides an offline-tolerant helper that fetches
// https://models.dev/api.json once per day and caches it under
// ~/.cache/graft/models/models-dev.json.  The public API is:
//
//	ModelsFor(providerKey string) ([]string, error)
//
// The function returns ErrUnavailable when there is no cache AND the network is
// unreachable.  Callers MUST treat ErrUnavailable as "skip model validation"
// rather than hard-blocking the operation.
//
// All external dependencies (HTTP client, clock, cache directory) are injected
// via a Config so unit tests never hit the network.
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// ModelsDevURL is the canonical models.dev JSON endpoint.
	ModelsDevURL = "https://models.dev/api.json"
	// DefaultTTL is how long a cached response is considered fresh.
	DefaultTTL = 24 * time.Hour
	// cacheFile is the file name inside the cache directory.
	cacheFile = "models-dev.json"
)

// ErrUnavailable is returned by ModelsFor when there is no cached data and the
// network fetch failed.  Callers must skip validation when they receive this.
var ErrUnavailable = errors.New("models: unavailable (offline and no cache)")

// HTTPClient is the interface used to make HTTP GET requests.
// *http.Client satisfies it.
type HTTPClient interface {
	Get(url string) (*http.Response, error)
}

// Config controls the behaviour of ModelsFor.  The zero value uses sensible
// production defaults (real http.Client, real clock, XDG cache dir).
type Config struct {
	// URL to fetch; defaults to ModelsDevURL.
	URL string
	// CacheDir overrides the directory where the JSON is cached.
	// When empty the real XDG cache dir (~/.cache/graft/models) is used.
	CacheDir string
	// Client is the HTTP client.  Defaults to http.DefaultClient.
	Client HTTPClient
	// Now returns the current time.  Defaults to time.Now.
	Now func() time.Time
	// TTL overrides DefaultTTL.
	TTL time.Duration
}

func (c *Config) url() string {
	if c.URL != "" {
		return c.URL
	}
	return ModelsDevURL
}

func (c *Config) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultTTL
}

func (c *Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Config) client() HTTPClient {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

func (c *Config) cacheDir() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	// XDG-compatible default: ~/.cache/graft/models
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "graft", "models")
}

// modelsDevResponse is the top-level shape of models.dev/api.json.
// The API returns an object keyed by provider id whose value is an object with
// a "models" key containing a list of model objects.
//
// Minimal shape (only what we use):
//
//	{
//	  "anthropic": {
//	    "models": [{ "id": "claude-opus-4-8", ... }, ...]
//	  },
//	  "openai": { "models": [ ... ] },
//	  ...
//	}
type modelsDevResponse map[string]providerEntry

type providerEntry struct {
	Models []modelEntry `json:"models"`
}

type modelEntry struct {
	ID string `json:"id"`
}

// cachedFile wraps the raw JSON bytes together with a fetch timestamp so we
// can check freshness without a separate metadata file.
type cachedFile struct {
	FetchedAt time.Time       `json:"fetched_at"`
	Data      modelsDevResponse `json:"data"`
}

// ModelsFor returns the list of known model ids for the given models.dev
// provider key (e.g. "anthropic", "openai", "google").
//
// Cache-hit (file present and within TTL): returns ids, no network call.
// Cache-stale (file present but old): attempts a refresh; if the refresh fails
// (offline) returns the stale ids WITHOUT an error (offline tolerance).
// Cache-miss AND offline: returns nil, ErrUnavailable.
func ModelsFor(providerKey string, cfg Config) ([]string, error) {
	cacheDir := cfg.cacheDir()
	cachePath := ""
	if cacheDir != "" {
		cachePath = filepath.Join(cacheDir, cacheFile)
	}

	now := cfg.now()

	// 1. Try to load the cache.
	var cached *cachedFile
	if cachePath != "" {
		if c, err := loadCache(cachePath); err == nil {
			cached = c
		}
	}

	// 2. If fresh, return immediately.
	if cached != nil && now.Sub(cached.FetchedAt) < cfg.ttl() {
		return extractIDs(cached.Data, providerKey), nil
	}

	// 3. Attempt a network refresh.
	fresh, fetchErr := fetchRemote(cfg.client(), cfg.url())
	if fetchErr == nil {
		// Persist to cache.
		cf := &cachedFile{FetchedAt: now, Data: fresh}
		if cachePath != "" {
			_ = saveCache(cachePath, cf) // best-effort; ignore write errors
		}
		return extractIDs(fresh, providerKey), nil
	}

	// 4. Fetch failed.  Fall back to stale cache if available.
	if cached != nil {
		return extractIDs(cached.Data, providerKey), nil
	}

	// 5. No cache at all and network is down.
	return nil, ErrUnavailable
}

// loadCache reads and deserializes the cache file.
func loadCache(path string) (*cachedFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf cachedFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("models: corrupt cache: %w", err)
	}
	return &cf, nil
}

// saveCache serializes and writes the cache file, creating parent dirs.
func saveCache(path string, cf *cachedFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(cf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// fetchRemote downloads and parses the models.dev JSON.
func fetchRemote(client HTTPClient, url string) (modelsDevResponse, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("models: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models: fetch %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("models: read body: %w", err)
	}
	var result modelsDevResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("models: parse JSON: %w", err)
	}
	return result, nil
}

// extractIDs returns the model ids for the given provider key.
// Returns nil (not an error) when the provider key is absent in the data.
func extractIDs(data modelsDevResponse, providerKey string) []string {
	entry, ok := data[providerKey]
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(entry.Models))
	for _, m := range entry.Models {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// AllModels returns the flattened list of every model id from every provider
// entry in the models.dev catalog.  Used by providers (e.g. opencode) that
// support any backend and therefore accept any model id from the catalog.
//
// The same offline-tolerance rules as ModelsFor apply.
func AllModels(cfg Config) ([]string, error) {
	cacheDir := cfg.cacheDir()
	cachePath := ""
	if cacheDir != "" {
		cachePath = filepath.Join(cacheDir, cacheFile)
	}
	now := cfg.now()

	var cached *cachedFile
	if cachePath != "" {
		if c, err := loadCache(cachePath); err == nil {
			cached = c
		}
	}
	if cached != nil && now.Sub(cached.FetchedAt) < cfg.ttl() {
		return flattenAll(cached.Data), nil
	}
	fresh, fetchErr := fetchRemote(cfg.client(), cfg.url())
	if fetchErr == nil {
		cf := &cachedFile{FetchedAt: now, Data: fresh}
		if cachePath != "" {
			_ = saveCache(cachePath, cf)
		}
		return flattenAll(fresh), nil
	}
	if cached != nil {
		return flattenAll(cached.Data), nil
	}
	return nil, ErrUnavailable
}

// flattenAll collects every model id from every provider entry.
func flattenAll(data modelsDevResponse) []string {
	var ids []string
	for _, entry := range data {
		for _, m := range entry.Models {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
	}
	return ids
}
