package sync

import (
	"errors"
	"fmt"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// PostSyncValidationError is returned by the sync engine when the post-sync
// validation gate finds error-severity findings in the just-committed canonical
// store. It carries the blocking findings so callers (CLI) can render them.
//
// Unlike the gateway's pre-sync ValidationError, a PostSyncValidationError does
// NOT mean the sync was blocked or rolled back — by the time this surfaces the
// merge is already committed and the run is marked done. It exists purely to
// report loudly that the committed canonical store has drifted/corrupted, so the
// user knows the on-disk truth is suspect even though the operation "succeeded".
type PostSyncValidationError struct {
	Findings []contract.Finding
}

// Error renders the carried findings as a single line, mirroring the gateway
// ValidationError format (including the "/<provider>" location suffix).
func (e *PostSyncValidationError) Error() string {
	if len(e.Findings) == 0 {
		return "post-sync validation failed"
	}
	parts := make([]string, 0, len(e.Findings))
	for _, f := range e.Findings {
		loc := f.Agent
		if f.Provider != "" {
			loc += "/" + f.Provider
		}
		parts = append(parts, fmt.Sprintf("%s: %s", loc, f.Message))
	}
	out := "post-sync validation failed: "
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

// PostSyncFindings returns the findings carried by err if it is a
// *PostSyncValidationError, else nil.
func PostSyncFindings(err error) []contract.Finding {
	var pe *PostSyncValidationError
	if errors.As(err, &pe) {
		return pe.Findings
	}
	return nil
}

// validateCanonicalStore re-validates the named canonical agents AFTER a sync
// has written and committed them to the canonical .graft/ store.
//
// WHY this gate exists: the sync engine's pre-sync validation runs against the
// inputs, but the merge itself can introduce corruption or drift into the
// canonical store — three-way merges can interleave fields, a half-applied
// resolution can leave conflict markers behind, or a serialization round-trip
// can emit something that no longer loads or validates. Once the merge is
// committed, that bad state is the on-disk truth; without this gate it would go
// silently undetected and the next sync (or consumer) would inherit it.
//
// This method re-runs the canonical validation trio over the ACTUAL committed
// canonical agents and reports any error-severity findings loudly. It does NOT
// roll back: the merge is already committed and the run is already done. The
// contract here is "report, don't undo" — surface the problem so the user can
// fix it, while leaving the committed state intact (rolling back a successful,
// committed merge would itself risk losing user work).
//
// The validation trio matches the gateway's pre-sync validateAgents:
//  1. ScanConflictMarkers FIRST — a marker-laden file is unparseable, so report
//     it and skip Load/Validate (avoids a cryptic YAML error masking the real
//     cause).
//  2. canonical.Load — a load error is itself an error-severity finding.
//  3. canonical.Validate — its returned error is a HARNESS failure (the schema
//     could not compile), not a content violation, so it is propagated as a
//     hard error rather than collected as a finding.
//
// Only error-severity findings gate; warnings are intentionally dropped (they
// never block and would be noise in a post-commit report).
func (e *Engine) validateCanonicalStore(names []string) error {
	var findings []contract.Finding
	for _, name := range names {
		agentDir := canonical.AgentDir(e.root, name)

		// Conflict markers => file is unparseable; report and skip Load/Validate.
		if mf := canonical.ScanConflictMarkers(agentDir, name); len(mf) > 0 {
			findings = append(findings, mf...)
			continue
		}

		can, err := canonical.Load(agentDir)
		if err != nil {
			findings = append(findings, contract.Finding{
				Agent:    name,
				Severity: "error",
				Message:  fmt.Sprintf("load canonical agent: %v", err),
			})
			continue
		}

		fs, verr := canonical.Validate(can)
		if verr != nil {
			// Harness failure (schema would not compile), not a content
			// violation — propagate as a hard error.
			return fmt.Errorf("sync: validate %s: %w", name, verr)
		}
		findings = append(findings, fs...)
	}

	// Keep only error-severity findings; warnings never gate.
	var errs []contract.Finding
	for _, f := range findings {
		if f.Severity == "error" {
			errs = append(errs, f)
		}
	}
	if len(errs) > 0 {
		return &PostSyncValidationError{Findings: errs}
	}
	return nil
}
