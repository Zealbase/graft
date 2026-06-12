// Package toolset provides a simple frozenset for provider-known tool names,
// used by contract.ToolSupporter implementations. The set is built once at
// package init and is safe for concurrent reads (never mutated).
package toolset

// Set is an immutable set of tool name strings, case-sensitive.
type Set map[string]struct{}

// New builds a Set from the supplied names.
func New(names ...string) Set {
	s := make(Set, len(names))
	for _, n := range names {
		s[n] = struct{}{}
	}
	return s
}

// Contains reports whether the tool name is in the set.
func (s Set) Contains(tool string) bool {
	_, ok := s[tool]
	return ok
}
