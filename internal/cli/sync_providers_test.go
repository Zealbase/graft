package cli_test

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli"
	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// captureGate is a stub EntryGate that records the SyncOpts it was called with.
type captureGate struct {
	lastSync   contract.SyncOpts
	abortCalls int
	abortRes   contract.AbortResult
}

func (g *captureGate) Init() (contract.InitResult, error)    { return contract.InitResult{}, nil }
func (g *captureGate) List() ([]contract.AgentStatus, error) { return nil, nil }
func (g *captureGate) Status(*string) (contract.StatusReport, error) {
	return contract.StatusReport{}, nil
}
func (g *captureGate) Sync(opts contract.SyncOpts) (contract.RunResult, error) {
	g.lastSync = opts
	return contract.RunResult{Status: contract.RunDone}, nil
}
func (g *captureGate) AbortSync() (contract.AbortResult, error) {
	g.abortCalls++
	return g.abortRes, nil
}
func (g *captureGate) Validate(string) ([]contract.Finding, error) { return nil, nil }
func (g *captureGate) SkillList() ([]contract.Skill, error)        { return nil, nil }
func (g *captureGate) SkillStatus(contract.SkillOpts) ([]contract.SkillStatus, error) {
	return nil, nil
}
func (g *captureGate) SkillInstall(string, contract.SkillOpts) ([]contract.SkillStatus, error) {
	return nil, nil
}
func (g *captureGate) SkillSync(contract.SkillOpts) ([]contract.SkillStatus, error) { return nil, nil }
func (g *captureGate) CreateAgent(string, string) (contract.CanonicalAgent, error) {
	return contract.CanonicalAgent{}, nil
}
func (g *captureGate) SetAgentModel(string, string, string) ([]contract.Finding, error) {
	return nil, nil
}
func (g *captureGate) Update(contract.UpdateOpts) (contract.UpdateResult, error) {
	return contract.UpdateResult{}, nil
}
func (g *captureGate) Destroy(contract.DestroyOpts) (contract.DestroyResult, error) {
	return contract.DestroyResult{}, nil
}
func (g *captureGate) Close() error { return nil }

func runSyncWith(t *testing.T, resolver config.Resolver, args ...string) contract.SyncOpts {
	t.Helper()
	gate := &captureGate{}
	c := cli.EntrypointWithVersion(gate, resolver, "test")
	var out, errb bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errb)
	r.SetArgs(args)
	if err := r.Execute(); err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, out.String())
	}
	return gate.lastSync
}

// runAbortWith drives a sync command with --abort through the CLI against the
// capture gate, returning the gate (for call assertions) and stdout.
func runAbortWith(t *testing.T, gate *captureGate, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	c := cli.EntrypointWithVersion(gate, resolver, "test")
	var out, errb bytes.Buffer
	r := c.Root()
	r.SetOut(&out)
	r.SetErr(&errb)
	r.SetArgs(args)
	if err := r.Execute(); err != nil {
		t.Fatalf("abort %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// TestSyncAbortRoutesToGateway confirms `graft sync agents --abort` calls
// AbortSync (not Sync) and renders the confirmation naming the run + pruned count.
func TestSyncAbortRoutesToGateway(t *testing.T) {
	gate := &captureGate{abortRes: contract.AbortResult{Aborted: true, RunID: "run-123", PrunedBranches: 3}}
	out := runAbortWith(t, gate, "sync", "agents", "--abort")
	if gate.abortCalls != 1 {
		t.Fatalf("AbortSync calls = %d, want 1", gate.abortCalls)
	}
	if gate.lastSync.Names != nil {
		t.Fatalf("--abort must not run a sync, but Sync recorded opts: %+v", gate.lastSync)
	}
	if !strings.Contains(out, "run-123") || !strings.Contains(out, "pruned 3 temp branches") {
		t.Fatalf("abort output missing confirmation: %q", out)
	}
}

// TestSyncAbortNoOpMessage confirms aborting with nothing in progress prints the
// friendly no-op message and exits 0.
func TestSyncAbortNoOpMessage(t *testing.T) {
	gate := &captureGate{abortRes: contract.AbortResult{Aborted: false}}
	out := runAbortWith(t, gate, "sync", "agent", "x", "--abort")
	if gate.abortCalls != 1 {
		t.Fatalf("AbortSync calls = %d, want 1", gate.abortCalls)
	}
	if !strings.Contains(out, "no in-progress sync to abort") {
		t.Fatalf("abort no-op output = %q, want friendly message", out)
	}
}

func TestSyncCarriesEffectiveProvidersAll(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	// mode=all (default) -> full supported set.
	opts := runSyncWith(t, resolver, "sync", "agents")
	if !reflect.DeepEqual(opts.Providers, config.SupportedProviders()) {
		t.Fatalf("opts.Providers = %v, want all supported %v", opts.Providers, config.SupportedProviders())
	}
}

func TestSyncCarriesEffectiveProvidersSpecific(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	if err := resolver.Save(config.ApplyDefaults(&config.Config{
		Providers: config.ProvidersConfig{
			Mode:    config.ProviderModeSpecific,
			Enabled: []string{"opencode", "claude-code"},
		},
	})); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	opts := runSyncWith(t, resolver, "sync", "agents")
	want := []string{"claude-code", "opencode"} // sorted by EffectiveProviders
	if !reflect.DeepEqual(opts.Providers, want) {
		t.Fatalf("opts.Providers = %v, want %v", opts.Providers, want)
	}
}

func TestSyncCarriesEffectiveProvidersDisabled(t *testing.T) {
	// antigravity is no longer in SupportedProviders (unregistered pending research);
	// use grok-cli as the representative disabled provider.
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}
	if err := resolver.Save(config.ApplyDefaults(&config.Config{
		Providers: config.ProvidersConfig{
			Mode:     config.ProviderModeAll,
			Disabled: []string{"grok-cli"},
		},
	})); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	opts := runSyncWith(t, resolver, "sync", "agent", "x")
	for _, p := range opts.Providers {
		if p == "grok-cli" {
			t.Fatalf("disabled provider leaked into sync opts: %v", opts.Providers)
		}
	}
	if len(opts.Providers) != len(config.SupportedProviders())-1 {
		t.Fatalf("opts.Providers = %d, want %d", len(opts.Providers), len(config.SupportedProviders())-1)
	}
	// `sync agent x` also carries the named target.
	if len(opts.Names) != 1 || opts.Names[0] != "x" {
		t.Fatalf("opts.Names = %v, want [x]", opts.Names)
	}
}
