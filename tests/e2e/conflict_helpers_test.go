package e2e

import (
	"strings"
	"testing"
)

// resolveSide rewrites a git-conflicted file keeping exactly one side of every
// conflict hunk and dropping the markers. side is "ours" (the first / HEAD side,
// i.e. the SOURCE provider) or "theirs" (the second / incoming side, the TARGET
// provider). Non-conflicting lines are preserved verbatim.
func resolveSide(body, side string) string {
	var out []string
	state := "" // "", "ours", "theirs"
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			state = "ours"
			continue
		case strings.HasPrefix(line, "======="):
			state = "theirs"
			continue
		case strings.HasPrefix(line, ">>>>>>>"):
			state = ""
			continue
		}
		if state == "" || state == side {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// resolveManual replaces a conflicted YAML `model:` block with a single chosen
// model line (a hand-merged value that is neither side verbatim is achieved by
// passing a novel model), dropping all markers and both candidate lines.
func resolveManualModel(body, model string) string {
	var out []string
	state := ""
	emitted := false
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			state = "ours"
			continue
		case strings.HasPrefix(line, "======="):
			state = "theirs"
			continue
		case strings.HasPrefix(line, ">>>>>>>"):
			state = ""
			if !emitted {
				out = append(out, "model: "+model)
				emitted = true
			}
			continue
		}
		if state == "" {
			out = append(out, line)
		}
		// drop both conflicting sides; the merged line is emitted at hunk end.
	}
	return strings.Join(out, "\n")
}

// hasMarkers reports whether s still contains git conflict markers.
func hasMarkers(s string) bool {
	return strings.Contains(s, "<<<<<<<") || strings.Contains(s, "=======") || strings.Contains(s, ">>>>>>>")
}

// syncResume runs the convergence step after a user has resolved a conflict.
//
// Per the in-flight core semantics, the PRIMARY path is a BARE `graft sync
// agents` re-run that auto-continues. Until that change lands, the binary
// refuses a bare re-run with "rerun with --continue"; this helper detects that
// transitional refusal and falls back to the explicit `--continue` alias so the
// resolution scenarios assert the OUTCOME (convergence + propagation) across the
// core change. It reports which path served via usedContinue.
func syncResume(t *testing.T, root string) (res runResult, usedContinue bool) {
	t.Helper()
	bare := graft(t, root, "sync", "agents", "-o", "json")
	if bare.exitCode == 0 {
		return bare, false
	}
	// Transitional: bare re-run refused pending core's auto-continue change.
	if contains(bare.stderr, "--continue") || contains(bare.stderr, "unresolved conflict run") {
		cont := graft(t, root, "sync", "agents", "--continue", "-o", "json")
		return cont, true
	}
	// Any other non-zero is a real failure to surface.
	return bare, false
}
