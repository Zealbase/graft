package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// TestPrintAgentListTableSurfacesDrift: a drifted agent renders its out-of-sync
// providers distinctly (on an "out of sync:" line) plus a "<k>/<n> in sync"
// summary, rather than burying drift in the flat provider:ok/drift list
// (v0.0.4 verify). In-sync agents keep the flat coverage rendering.
func TestPrintAgentListTableSurfacesDrift(t *testing.T) {
	agents := []contract.AgentStatus{
		{
			Name:   "drifter",
			InSync: false,
			Providers: map[string]bool{
				"claude-code": true,
				"opencode":    false,
				"cursor":      false,
			},
		},
		{
			Name:   "clean",
			InSync: true,
			Providers: map[string]bool{
				"claude-code": true,
				"opencode":    true,
			},
		},
	}
	var buf bytes.Buffer
	if err := printAgentListTable(&buf, agents); err != nil {
		t.Fatalf("printAgentListTable: %v", err)
	}
	out := buf.String()

	// The drifted providers must appear on a distinct "out of sync:" line.
	if !strings.Contains(out, "out of sync:") {
		t.Fatalf("missing distinct out-of-sync line:\n%s", out)
	}
	if !strings.Contains(out, "cursor") || !strings.Contains(out, "opencode") {
		t.Fatalf("drifted providers not listed:\n%s", out)
	}
	// The summary must show the in-sync fraction (1 of 3 here).
	if !strings.Contains(out, "1/3 in sync") {
		t.Fatalf("missing '<k>/<n> in sync' summary:\n%s", out)
	}

	// Drift must NOT be buried as a flat "provider:drift" cell for the drifted
	// agent: the old comma-packed rendering used ":drift" markers.
	driftBlock := out[strings.Index(out, "drifter"):]
	if i := strings.Index(out, "clean"); i > strings.Index(out, "drifter") {
		driftBlock = out[strings.Index(out, "drifter"):i]
	}
	if strings.Contains(driftBlock, ":drift") {
		t.Fatalf("drift buried in flat provider:drift cell:\n%s", out)
	}

	// The clean agent keeps the flat coverage rendering (provider:ok), and must
	// not carry an out-of-sync line.
	cleanBlock := out[strings.Index(out, "clean"):]
	if strings.Contains(cleanBlock, "out of sync:") {
		t.Fatalf("in-sync agent should not show an out-of-sync line:\n%s", out)
	}
	if !strings.Contains(cleanBlock, "claude-code:ok") {
		t.Fatalf("in-sync agent should keep flat coverage rendering:\n%s", out)
	}
}
