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
