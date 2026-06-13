package e2e

import (
	"os"
	"strings"
	"testing"
)

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// mustRemoveAll removes an absolute path (file or dir) and fails the test on
// error. Used by deletion tests to simulate the user removing a canonical or a
// single provider file.
func mustRemoveAll(t *testing.T, abs string) {
	t.Helper()
	if err := os.RemoveAll(abs); err != nil {
		t.Fatalf("remove %s: %v", abs, err)
	}
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// splitLines splits a string into lines WITHOUT the trailing newline on each
// line. An empty string yields a nil slice. Used by tests that need to inject
// a line into a file without pulling in bufio.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// strings.Split on "\n" would include a trailing empty element when s ends
	// with "\n" (which YAML files always do); drop it.
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
