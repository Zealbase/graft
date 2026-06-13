// Package povr holds small shared helpers for the lossless ProviderOverrides
// rule that every provider package follows: extract the frontmatter/field keys
// that have no canonical home into an overrides map, and restore them back onto
// an ordered map when serializing. It also has a few generic coercion helpers.
package povr

import (
	"sort"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/omap"
)

// Extras returns a copy of all map entries whose key is NOT in known. The
// result is the lossless ProviderOverrides payload for a provider. A nil or
// empty result means there were no extra keys.
func Extras(all map[string]any, known []string) map[string]any {
	if len(all) == 0 {
		return nil
	}
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}
	out := map[string]any{}
	for k, v := range all {
		if knownSet[k] {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Restore writes the override entries onto the ordered map in sorted key order
// (deterministic output). Keys already present on the map are not overwritten.
// This is used for the stashed-extras pattern (lossless round-trip of unknown
// keys); it does NOT let overrides win over canonical fields.
func Restore(m *omap.OMap, overrides map[string]any) {
	for _, k := range SortedKeys(overrides) {
		if m.Has(k) {
			continue
		}
		m.Set(k, overrides[k])
	}
}

// RestoreOverrides writes the override entries onto the ordered map in sorted
// key order. Unlike Restore, override values WIN: if the key is already present
// its value is replaced by the override value. The protect set lists keys that
// must never be overwritten (typically {"name"} to guard agent identity).
func RestoreOverrides(m *omap.OMap, overrides map[string]any, protect map[string]bool) {
	for _, k := range SortedKeys(overrides) {
		if protect[k] {
			continue // protected key — identity must not change
		}
		m.Set(k, overrides[k])
	}
}

// SortedKeys returns the map's keys sorted lexicographically.
func SortedKeys(m map[string]any) []string {
	if m == nil {
		return nil
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// String coerces a decoded YAML/JSON value to a string (empty if not a string).
func String(v any) string {
	s, _ := v.(string)
	return s
}

// StringSlice coerces a decoded value to []string. Accepts []string, []any of
// strings, or a single string. Returns nil otherwise.
func StringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

// NormalizeBody collapses CRLF/CR to LF and ensures a non-empty body ends with
// exactly one trailing newline; an empty body stays empty. (Mirrors
// canonical.normalizeBody so provider output never carries embedded CR.)
func NormalizeBody(body string) string {
	if body == "" {
		return ""
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.TrimRight(body, "\n") + "\n"
}
