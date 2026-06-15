// Package toolmapper provides a bidirectional, case-insensitive mapping between
// a provider's native tool names and the shared canonical vocabulary.
//
// Canonical names are lowercase_snake_case (e.g. "read_file", "bash").
// Native names follow provider conventions (e.g. "Read", "shell", "websearch").
//
// Providers embed one *Map per package and implement contract.ToolMapper by
// delegating to it. The Map is built once at init time and is safe for
// concurrent reads.
package toolmapper

import (
	"sort"
	"strings"
)

// Entry holds one native↔canonical pair.
type Entry struct {
	Native    string
	Canonical string
}

// Map is an immutable bidirectional tool-name mapping.
type Map struct {
	// nativeToCanonical is keyed by strings.ToLower(native).
	nativeToCanonical map[string]string
	// canonicalToNative maps canonical → first native that claimed it.
	canonicalToNative map[string]string
	// tools is the sorted slice of canonical names supported.
	tools []string
}

// New builds a Map from a slice of Entry values. If two entries share the same
// canonical name (e.g. gemini-cli "edit" and "replace" both → "file_edit"),
// the first entry wins for the canonical→native direction; all entries resolve
// in the native→canonical direction. Lookup on the native side is
// case-insensitive.
func New(entries []Entry) *Map {
	m := &Map{
		nativeToCanonical: make(map[string]string, len(entries)),
		canonicalToNative: make(map[string]string, len(entries)),
	}
	canonSet := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		key := strings.ToLower(e.Native)
		m.nativeToCanonical[key] = e.Canonical
		if _, exists := m.canonicalToNative[e.Canonical]; !exists {
			m.canonicalToNative[e.Canonical] = e.Native
		}
		canonSet[e.Canonical] = struct{}{}
	}
	m.tools = make([]string, 0, len(canonSet))
	for c := range canonSet {
		m.tools = append(m.tools, c)
	}
	sort.Strings(m.tools)
	return m
}

// CanonicalTool translates a native name (case-insensitive) to canonical.
// ok is false if the name is not in this provider's mapping.
func (m *Map) CanonicalTool(native string) (canonical string, ok bool) {
	canonical, ok = m.nativeToCanonical[strings.ToLower(native)]
	return
}

// NativeTool translates a canonical name to this provider's native name.
// ok is false if the provider does not support that canonical tool.
func (m *Map) NativeTool(canonical string) (native string, ok bool) {
	native, ok = m.canonicalToNative[canonical]
	return
}

// Tools returns the sorted canonical names supported by this provider.
func (m *Map) Tools() []string {
	out := make([]string, len(m.tools))
	copy(out, m.tools)
	return out
}

// MapToCanonical translates a slice of native tool names to canonical names.
// Names with no mapping are kept verbatim (pass-through).
func (m *Map) MapToCanonical(natives []string) []string {
	if len(natives) == 0 {
		return nil
	}
	out := make([]string, len(natives))
	for i, n := range natives {
		if c, ok := m.CanonicalTool(n); ok {
			out[i] = c
		} else {
			out[i] = n
		}
	}
	return out
}

// MapToNative translates a slice of canonical tool names to native names.
// Names with no mapping are kept verbatim (pass-through).
func (m *Map) MapToNative(canonicals []string) []string {
	if len(canonicals) == 0 {
		return nil
	}
	out := make([]string, len(canonicals))
	for i, c := range canonicals {
		if n, ok := m.NativeTool(c); ok {
			out[i] = n
		} else {
			out[i] = c
		}
	}
	return out
}
