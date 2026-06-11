package gateway

import (
	"fmt"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// ValidationError is returned by Sync when the pre-sync validate gate finds
// error-severity findings. It carries the blocking findings so the CLI can
// render them and exit non-zero. The CLI may type-assert to surface the
// structured findings.
type ValidationError struct {
	Findings []contract.Finding
}

// Findings exposed for callers that want the structured list.
func (e *ValidationError) Error() string {
	if len(e.Findings) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Findings))
	for _, f := range e.Findings {
		loc := f.Agent
		if f.Provider != "" {
			loc += "/" + f.Provider
		}
		parts = append(parts, fmt.Sprintf("%s: %s", loc, f.Message))
	}
	return "sync blocked by validation: " + strings.Join(parts, "; ")
}

// FindingsOf returns the findings carried by err if it is a *ValidationError,
// else nil.
func FindingsOf(err error) []contract.Finding {
	if ve, ok := err.(*ValidationError); ok {
		return ve.Findings
	}
	return nil
}
