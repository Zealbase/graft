package canonical

import (
	_ "embed"
	"fmt"
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

const schemaURL = "https://tfs.local/schemas/common-agent-definition.schema.json"

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
		xp := make(map[string]any, len(a.ProviderOverrides))
		for prov, fields := range a.ProviderOverrides {
			inner := make(map[string]any, len(fields))
			for k, v := range fields {
				inner[k] = v
			}
			xp[prov] = inner
		}
		inst["x-provider"] = xp
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
