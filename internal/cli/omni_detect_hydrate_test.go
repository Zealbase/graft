package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"github.com/Shaik-Sirajuddin/graft/internal/gateway"
)

// testResolver supports every ref and returns a fixed header (stands in for the
// future omni capability so the applied path is reachable from the CLI).
type testResolver struct{ header string }

func (r testResolver) Supported(string) bool          { return true }
func (r testResolver) Resolve(string) (string, error) { return r.header, nil }

// execCLIWithResolver runs the cobra tree against a gate opened at root with an
// injected OmniResolver, returning stdout and stderr separately. It mirrors
// execCLIStreams but lets a test exercise the supported omni path.
func execCLIWithResolver(t *testing.T, root string, resolver contract.OmniResolver, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	g, gerr := gateway.Open(root)
	if gerr != nil {
		t.Fatalf("gateway.Open: %v", gerr)
	}
	defer g.Close()
	if resolver != nil {
		g.(gateway.OmniResolverConfigurable).SetOmniResolver(resolver)
	}
	c := cli.EntrypointWithVersion(g, nil, "test")
	c.SetProjectResolver(&config.DefaultProjectResolver{WorkspaceRoot: root})
	var out, errBuf bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errBuf)
	r.SetArgs(args)
	err = c.Install()
	return out.String(), errBuf.String(), err
}

// execDetect runs `graft detect` WITHOUT a gateway (gate=nil), proving the
// command is side-effect-free: it never calls gateway.Open (which would create
// .graft/). cwd is set to root for the duration of the call.
func execDetect(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	prev, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(prev)

	c := cli.EntrypointWithVersion(nil, nil, "test")
	c.SetProjectResolver(&config.DefaultProjectResolver{WorkspaceRoot: root})
	var out, errBuf bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errBuf)
	r.SetArgs(append([]string{"detect"}, args...))
	err := r.Execute()
	return out.String(), err
}

// --- omni init -----------------------------------------------------------

// TestCLIOmniInitBareDefaultsRefToName: bare --omni-agent defaults the ref to
// the positional <name>; unsupported (default resolver) ⇒ recorded + warned,
// Body unchanged.
func TestCLIOmniInitBareDefaultsRefToName(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, errOut, err := execCLIStreams(t, root, nil, "agent", "init", "fixer", "Body.", "--omni-agent")
	if err != nil {
		t.Fatalf("agent init --omni-agent: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, "not yet supported") {
		t.Fatalf("expected unsupported warning on stderr:\n%s", errOut)
	}
	// meta records ref=name, applied=false.
	meta := readMeta(t, root, "fixer")
	if meta.Omni == nil || meta.Omni.Ref != "fixer" || meta.Omni.Applied {
		t.Fatalf("meta.Omni not recorded as unsupported ref=name: %+v", meta.Omni)
	}
	// Body unchanged (no omni sentinel).
	if body := readBody(t, root, "fixer"); strings.Contains(body, "<!-- graft:omni") {
		t.Fatalf("Body should not contain an omni block on the unsupported path:\n%s", body)
	}
}

// TestCLIOmniInitExplicitRef: --omni-agent=<ref> records that ref.
func TestCLIOmniInitExplicitRef(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := execCLIStreams(t, root, nil, "agent", "init", "fixer", "--omni-agent=shared-omni"); err != nil {
		t.Fatalf("agent init --omni-agent=ref: %v", err)
	}
	meta := readMeta(t, root, "fixer")
	if meta.Omni == nil || meta.Omni.Ref != "shared-omni" {
		t.Fatalf("explicit ref not recorded: %+v", meta.Omni)
	}
}

// TestCLIOmniInitSupportedApplies: with an injected supported resolver, bare
// --omni-agent prepends the header into Body and marks meta applied.
func TestCLIOmniInitSupportedApplies(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, errOut, err := execCLIWithResolver(t, root, testResolver{header: "SHARED"},
		"agent", "init", "fixer", "Body.", "--omni-agent")
	if err != nil {
		t.Fatalf("agent init supported: %v\n%s", err, out)
	}
	if strings.Contains(errOut, "not yet supported") {
		t.Fatalf("supported path must not warn unsupported:\n%s", errOut)
	}
	body := readBody(t, root, "fixer")
	if !strings.Contains(body, "<!-- graft:omni fixer -->") || !strings.Contains(body, "SHARED") {
		t.Fatalf("omni block not applied to Body:\n%s", body)
	}
	meta := readMeta(t, root, "fixer")
	if meta.Omni == nil || !meta.Omni.Applied || !meta.Omni.Supported {
		t.Fatalf("meta.Omni not applied: %+v", meta.Omni)
	}
}

// TestCLIOmniRefreshUnsupportedNoOp: `agent <name> omni --refresh` against the
// default resolver is a clean no-op + warning, exit 0, Body unchanged.
func TestCLIOmniRefreshUnsupportedNoOp(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := execCLIStreams(t, root, nil, "agent", "init", "fixer", "Body.", "--omni-agent"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	before := readBody(t, root, "fixer")
	out, errOut, err := execCLIStreams(t, root, nil, "agent", "fixer", "omni", "--refresh")
	if err != nil {
		t.Fatalf("omni --refresh unsupported should exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, "not yet supported") {
		t.Fatalf("expected unsupported warning:\n%s", errOut)
	}
	if after := readBody(t, root, "fixer"); after != before {
		t.Fatalf("Body changed on unsupported refresh:\n%s", after)
	}
}

// TestCLIOmniRefreshRequiresFlag: bare `omni` without --refresh errors.
func TestCLIOmniRefreshRequiresFlag(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "agent", "init", "fixer"); err != nil {
		t.Fatalf("agent init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "agent", "fixer", "omni"); err == nil {
		t.Fatalf("bare omni without --refresh should error")
	}
}

// --- detect --------------------------------------------------------------

// TestCLIDetectNonGraftDir: a non-graft dir reports isWorkspace=false,
// initialized=false, with the friendly hint, and writes NOTHING (no .graft/).
func TestCLIDetectNonGraftDir(t *testing.T) {
	root := t.TempDir()
	out, err := execDetect(t, root, "-o", "json")
	if err != nil {
		t.Fatalf("detect: %v\n%s", err, out)
	}
	var rep contract.DetectReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse detect json: %v\n%s", err, out)
	}
	if rep.IsWorkspace || rep.Initialized {
		t.Fatalf("non-graft dir should be {false,false}: %+v", rep)
	}
	if rep.Hint == "" {
		t.Fatalf("expected friendly hint, got none: %+v", rep)
	}
	// Side-effect-free: detect must not have created .graft/.
	if _, err := os.Stat(filepath.Join(root, ".graft")); err == nil {
		t.Fatalf("detect created .graft/ — must be side-effect-free")
	}
}

// TestCLIDetectUninitialized: a bare .graft/ (no agents store) reports
// isWorkspace=true, initialized=false.
func TestCLIDetectUninitialized(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".graft"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := execDetect(t, root, "-o", "json")
	if err != nil {
		t.Fatalf("detect: %v\n%s", err, out)
	}
	var rep contract.DetectReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !rep.IsWorkspace || rep.Initialized {
		t.Fatalf("bare .graft/ should be {true,false}: %+v", rep)
	}
}

// TestCLIDetectInitializedWorkspace: an initialized workspace reports
// isWorkspace=true, initialized=true.
func TestCLIDetectInitializedWorkspace(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execDetect(t, root, "-o", "json")
	if err != nil {
		t.Fatalf("detect: %v\n%s", err, out)
	}
	var rep contract.DetectReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if !rep.IsWorkspace || !rep.Initialized {
		t.Fatalf("initialized workspace should be {true,true}: %+v", rep)
	}
}

// --- hydrate -------------------------------------------------------------

// TestCLIStatusHydrateBlock: `agent <name> status -o json` carries an additive
// hydrate block exposing model/tools, AND the StatusReport keys remain parseable
// (back-compat).
func TestCLIStatusHydrateBlock(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "sync", "agents"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	out, err := execCLI(t, root, nil, "agent", "code-reviewer", "status", "-o", "json")
	if err != nil {
		t.Fatalf("agent status: %v\n%s", err, out)
	}
	// Back-compat: still a StatusReport.
	var rep contract.StatusReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("status not a StatusReport: %v\n%s", err, out)
	}
	// Additive hydrate key with stable shape.
	var wrap struct {
		Hydrate *contract.HydrateView `json:"hydrate"`
	}
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		t.Fatalf("parse hydrate: %v\n%s", err, out)
	}
	if wrap.Hydrate == nil || wrap.Hydrate.Name != "code-reviewer" {
		t.Fatalf("missing/incorrect hydrate block: %+v", wrap.Hydrate)
	}
	if wrap.Hydrate.Model != "sonnet" {
		t.Fatalf("hydrate model: %+v", wrap.Hydrate)
	}
	if len(wrap.Hydrate.Tools) == 0 {
		t.Fatalf("hydrate tools should expose the agent's tools: %+v", wrap.Hydrate)
	}
}

// TestCLISyncAgentHydrateProviderSandbox: `sync agent <name> --provider codex`
// JSON gains a hydrate block whose sandbox is provider-scoped (sandbox_mode),
// and the RunResult keys remain parseable (back-compat).
func TestCLISyncAgentHydrateProviderSandbox(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := execCLI(t, root, nil, "sync", "agents"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Give code-reviewer a codex sandbox_mode override.
	if _, err := execCLI(t, root, nil, "agent", "model", "code-reviewer",
		"--provider", "codex", "--model", "gpt-5-codex"); err != nil {
		t.Fatalf("agent model: %v", err)
	}
	writeSandboxMode(t, root, "code-reviewer", "workspace-write")

	out, err := execCLI(t, root, nil, "sync", "agent", "code-reviewer", "--provider", "codex", "-o", "json")
	if err != nil {
		t.Fatalf("sync agent: %v\n%s", err, out)
	}
	var res contract.RunResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("sync json not a RunResult: %v\n%s", err, out)
	}
	var wrap struct {
		Hydrate *contract.HydrateView `json:"hydrate"`
	}
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		t.Fatalf("parse hydrate: %v\n%s", err, out)
	}
	if wrap.Hydrate == nil {
		t.Fatalf("missing hydrate block on single-agent sync:\n%s", out)
	}
	if wrap.Hydrate.Sandbox["sandbox_mode"] != "workspace-write" {
		t.Fatalf("expected provider-scoped sandbox_mode: %+v", wrap.Hydrate.Sandbox)
	}
	if wrap.Hydrate.Model != "gpt-5-codex" {
		t.Fatalf("expected codex-scoped model: %+v", wrap.Hydrate)
	}
}
