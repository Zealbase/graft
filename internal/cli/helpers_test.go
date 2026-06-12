package cli

import (
	"os"
	"strings"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

func countOccurrences(s, sub string) int {
	return strings.Count(s, sub)
}
