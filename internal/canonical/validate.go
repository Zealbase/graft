//go:generate go run ./schema/gen/main.go

package canonical

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"
)

// msgPrinter localizes jsonschema ErrorKind strings. LocalizedString panics on a
// nil printer, so we keep a real one.
var msgPrinter = message.NewPrinter(message.MatchLanguage("en"))

//go:embed schema/common-agent-definition.schema.json
var schemaJSON []byte

const schemaURL = "https://raw.githubusercontent.com/Shaik-Sirajuddin/graft/main/internal/canonical/schema/common-agent-definition.schema.json"

var (
	compiledOnce sync.Once
	compiled     *jsonschema.Schema
	compileErr   error
)

func schema() (*jsonschema.Schema, error) {
	compiledOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(schemaJSON)))
		if err != nil {
			compileErr = fmt.Errorf("canonical: unmarshal schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaURL, doc); err != nil {
			compileErr = fmt.Errorf("canonical: add schema resource: %w", err)
			return
		}
		compiled, compileErr = c.Compile(schemaURL)
	})
	return compiled, compileErr
}

// toSchemaInstance projects a contract.CanonicalAgent onto the research
// common-agent-definition schema's vocabulary. The frozen contract is narrower
// than the schema, so only the overlapping fields are emitted:
//
//	name        -> name
//	description -> description
//	Body        -> systemPrompt (the schema-required body field)
//	Model       -> model
//	Tools       -> tools (allowlist form)
//	Permissions -> permissionMode is NOT set; permissions are provider-scoped
//	              and have no single schema field, so they are validated only
//	              structurally below.
//
// Fields absent from the contract (temperature, maxTurns, mcpServers, etc.) are
// simply omitted — they are optional in the schema.
func toSchemaInstance(a contract.CanonicalAgent) map[string]any {
	inst := map[string]any{
		"name":         a.Name,
		"description":  a.Description,
		"systemPrompt": a.Body,
	}
	if a.Model != "" {
		inst["model"] = a.Model
	}
	if len(a.Tools) > 0 {
		tools := make([]any, len(a.Tools))
		for i, t := range a.Tools {
			tools[i] = t
		}
		inst["tools"] = tools
	}
	if len(a.ProviderOverrides) > 0 {
		po := make(map[string]any, len(a.ProviderOverrides))
		for prov, fields := range a.ProviderOverrides {
			inner := make(map[string]any, len(fields))
			for k, v := range fields {
				inner[k] = v
			}
			po[prov] = inner
		}
		inst["providerOverrides"] = po
	}
	return inst
}

// Validate checks a canonical agent against the embedded common schema and
// returns structured findings (one per schema violation). A returned error is
// reserved for harness failures (schema won't compile); schema violations are
// reported as Findings, not errors, so callers can surface them per-agent.
func Validate(a contract.CanonicalAgent) ([]contract.Finding, error) {
	sch, err := schema()
	if err != nil {
		return nil, err
	}

	// Explicit guard: description must be non-empty (after trimming whitespace).
	// The schema enforces minLength:1 on the raw string, but a whitespace-only
	// description would pass that check while still being useless for delegation.
	// Claude Code (and other providers) require a non-empty description to
	// auto-detect and delegate to a subagent; a blank description makes the agent
	// undetectable. This guard surfaces a clear, actionable error message.
	if strings.TrimSpace(a.Description) == "" {
		return []contract.Finding{{
			Severity: severityError,
			Agent:    a.Name,
			Path:     "/description",
			Message: fmt.Sprintf(
				"agent %q: description is required and must be non-empty"+
					" (Claude and other providers need it to detect the agent)",
				a.Name,
			),
		}}, nil
	}

	inst := toSchemaInstance(a)
	verr := sch.Validate(inst)
	if verr == nil {
		return nil, nil
	}

	ve, ok := verr.(*jsonschema.ValidationError)
	if !ok {
		return []contract.Finding{{
			Severity: severityError,
			Agent:    a.Name,
			Message:  verr.Error(),
		}}, nil
	}

	var findings []contract.Finding
	collectFindings(ve, a.Name, &findings)
	if len(findings) == 0 {
		// Defensive: always surface at least the top-level error.
		findings = append(findings, contract.Finding{
			Severity: severityError,
			Agent:    a.Name,
			Message:  ve.Error(),
		})
	}
	return findings, nil
}

// severityError matches contract.Finding.Severity's "error" value.
const severityError = "error"

// collectFindings flattens the leaf causes of a jsonschema ValidationError into
// contract.Finding values. Leaf nodes carry the most specific messages.
func collectFindings(ve *jsonschema.ValidationError, agent string, out *[]contract.Finding) {
	if len(ve.Causes) == 0 {
		loc := instanceLocation(ve)
		*out = append(*out, contract.Finding{
			Severity: severityError,
			Agent:    agent,
			Path:     loc,
			Message:  fmt.Sprintf("%s: %s", loc, ve.ErrorKind.LocalizedString(msgPrinter)),
		})
		return
	}
	for _, c := range ve.Causes {
		collectFindings(c, agent, out)
	}
}

func instanceLocation(ve *jsonschema.ValidationError) string {
	if len(ve.InstanceLocation) == 0 {
		return "(root)"
	}
	return "/" + strings.Join(ve.InstanceLocation, "/")
}

// ScanConflictMarkers inspects the raw bytes of agent.yaml and instructions.md
// under dir (an agent directory: .../.graft/agents/<name>/) for git conflict
// markers. A conflict marker is detected when a line starts with "<<<<<<< " or
// ">>>>>>> " (seven chevrons followed by a space — the unambiguous git forms).
// The "=======" separator is only flagged when it appears as a standalone line
// (exactly seven equals signs, no leading/trailing text) AND either a "<<<<<<< "
// or ">>>>>>> " marker is also present in the same file, to avoid false positives
// on legitimate Markdown heading underlines.
//
// Returns error-severity findings (one per affected file). Missing files are
// silently skipped — a missing agent.yaml is caught by Load/Validate downstream.
// The name argument is used to populate Finding.Agent.
func ScanConflictMarkers(dir, name string) []contract.Finding {
	var findings []contract.Finding
	for _, filename := range []string{agentFile, bodyFile} {
		path := filepath.Join(dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			// Missing file — not our concern here; Load/Validate handles it.
			continue
		}
		if fs := scanMarkersInContent(string(data), path, name); len(fs) > 0 {
			findings = append(findings, fs...)
		}
	}
	return findings
}

// scanMarkersInContent checks whether content contains git conflict markers and
// returns a single error finding for the file if any are found. The check uses
// conservative rules to avoid false positives:
//
//  1. "<<<<<<< " (7 '<' + space) on a line start → always a conflict marker
//  2. ">>>>>>> " (7 '>' + space) on a line start → always a conflict marker
//  3. "=======" (exactly 7 '=') as the entire line → only flagged when rule 1
//     or 2 also matches (i.e., the trio is required for "=======" to count)
func scanMarkersInContent(content, path, name string) []contract.Finding {
	hasOpen := false
	hasClose := false
	hasSep := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<< "):
			hasOpen = true
		case strings.HasPrefix(line, ">>>>>>> "):
			hasClose = true
		case line == "=======":
			hasSep = true
		}
	}
	// A conflict block requires at least the open OR close marker.  The separator
	// alone (e.g. a Markdown underline "=======") is NOT flagged unless a chevron
	// marker is also present.
	detected := hasOpen || hasClose || (hasSep && (hasOpen || hasClose))
	if !detected {
		return nil
	}
	return []contract.Finding{{
		Severity: severityError,
		Agent:    name,
		Path:     path,
		Message: fmt.Sprintf(
			"unresolved git conflict markers in %s — resolve the conflict and remove the markers before syncing",
			path,
		),
	}}
}
