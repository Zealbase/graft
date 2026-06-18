package gateway

import (
	"fmt"
	"os"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// omniResolver returns the resolver the gate uses to turn an omni ref into
// sys-instructions. It defaults to canonical.DefaultOmniResolver (Supported()
// false) so omni refs are recorded but never applied until a real resolver is
// injected. The CLI/tests override it via SetOmniResolver.
func (g *gate) omniResolver() contract.OmniResolver {
	if g.omni == nil {
		return canonical.DefaultOmniResolver{}
	}
	return g.omni
}

// OmniResolverConfigurable is the optional capability the CLI/tests type-assert
// to inject an OmniResolver into the gateway. Implemented by *gate. It mirrors
// SkillHookConfigurable / EnabledProvidersConfigurable so Phase f/g can swap in a
// Supported()=true test resolver without touching the frozen EntryGate contract.
type OmniResolverConfigurable interface {
	SetOmniResolver(contract.OmniResolver)
}

// SetOmniResolver stores the omni resolver on the gate (test/override seam).
func (g *gate) SetOmniResolver(r contract.OmniResolver) { g.omni = r }

// OmniResult reports the outcome of recording/applying an omni ref on an agent.
// Applied is true only when the resolver supported the ref AND the omni block
// was prepended into the canonical Body. When Supported is false the ref is
// recorded in .meta.json and Body is left untouched (the unsupported path is a
// clean no-op, never an error).
type OmniResult struct {
	Ref       string `json:"ref"`
	Supported bool   `json:"supported"`
	Applied   bool   `json:"applied"`
	// Warning carries a human-readable message when the ref was recorded but not
	// applied (unsupported resolver). Empty when applied.
	Warning string `json:"warning,omitempty"`
}

// AgentOmniCapable is the optional capability the CLI type-asserts for the omni
// surface: scaffolding an agent with an omni ref (CreateAgentWithOmni) and
// re-running the resolver to replace the omni block in place (RefreshOmni). It
// is kept off the frozen EntryGate contract (additive capability, same pattern
// as the configurable seams).
type AgentOmniCapable interface {
	CreateAgentWithOmni(name, prompt, omniRef string) (contract.CanonicalAgent, OmniResult, error)
	RefreshOmni(name string) (OmniResult, error)
}

// CreateAgentWithOmni scaffolds a default canonical agent (like CreateAgent) and
// records/applies an omni ref. When omniRef == "" it is exactly CreateAgent with
// a zero OmniResult. Otherwise it runs the support-check:
//
//   - resolver.Supported(ref): Resolve(ref) -> PrependOmniBlock into Body,
//     meta.Omni{ref, applied=true, supported=true}.
//   - unsupported: meta.Omni{ref, applied=false, supported=false}, Body
//     UNCHANGED, OmniResult.Warning set. Never an error.
//
// A resolver that claims Supported but fails to Resolve is a hard error: the
// agent is NOT created (no partial Body write), keeping the store consistent.
func (g *gate) CreateAgentWithOmni(name, prompt, omniRef string) (contract.CanonicalAgent, OmniResult, error) {
	a, err := g.CreateAgent(name, prompt)
	if err != nil {
		return contract.CanonicalAgent{}, OmniResult{}, err
	}
	if omniRef == "" {
		return a, OmniResult{}, nil
	}

	res, aerr := g.applyOmni(&a, omniRef)
	if aerr != nil {
		// Resolution failed AFTER the bare agent was scaffolded. Roll the scaffold
		// back so init fails cleanly with no half-applied agent on disk.
		//
		// Rollback safety: CreateAgent above errors out if the agent dir already
		// exists (see stubs_v003.go), so reaching this point guarantees the dir was
		// freshly created by THIS call. The unconditional RemoveAll therefore only
		// ever deletes the scaffold we just wrote, never pre-existing agent data.
		_ = os.RemoveAll(canonical.AgentDir(g.root, a.Name))
		return contract.CanonicalAgent{}, OmniResult{}, aerr
	}
	return a, res, nil
}

// RefreshOmni re-runs the omni resolver for an existing agent and replaces the
// omni block in place. It reads the recorded ref from .meta.json; an agent with
// no recorded ref is an error (nothing to refresh). When the resolver is
// unsupported it is a clean no-op + warning (exit 0), leaving Body and meta as
// they were recorded.
func (g *gate) RefreshOmni(name string) (OmniResult, error) {
	dir := canonical.AgentDir(g.root, name)
	if _, err := os.Stat(dir); err != nil {
		return OmniResult{}, fmt.Errorf("gateway: agent %q not found", name)
	}
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		return OmniResult{}, fmt.Errorf("gateway: load meta %q: %w", name, err)
	}
	if meta.Omni == nil || meta.Omni.Ref == "" {
		return OmniResult{}, fmt.Errorf("gateway: agent %q has no omni ref to refresh (run `graft agent init <name> --omni-agent`)", name)
	}
	a, err := canonical.Load(dir)
	if err != nil {
		return OmniResult{}, fmt.Errorf("gateway: load agent %q: %w", name, err)
	}
	return g.applyOmni(&a, meta.Omni.Ref)
}

// applyOmni runs the support-check for ref against the current agent, persisting
// the result. On the supported path it prepends/replaces the omni block in Body
// and re-saves; on the unsupported path it records meta only (Body untouched).
// The agent's existing provider-hash meta is preserved across the save.
func (g *gate) applyOmni(a *contract.CanonicalAgent, ref string) (OmniResult, error) {
	dir := canonical.AgentDir(g.root, a.Name)
	meta, err := canonical.LoadMeta(dir)
	if err != nil {
		return OmniResult{}, fmt.Errorf("gateway: load meta %q: %w", a.Name, err)
	}

	resolver := g.omniResolver()
	if !resolver.Supported(ref) {
		// Unsupported: record the ref, leave Body untouched, warn (never error).
		meta.Omni = &contract.OmniRef{Ref: ref, Applied: false, Supported: false}
		if perr := g.persistAgent(*a, meta); perr != nil {
			return OmniResult{}, perr
		}
		return OmniResult{
			Ref:       ref,
			Supported: false,
			Applied:   false,
			Warning:   fmt.Sprintf("omni agent %q not yet supported — reference recorded, header skipped", ref),
		}, nil
	}

	sysInstr, rerr := resolver.Resolve(ref)
	if rerr != nil {
		return OmniResult{}, fmt.Errorf("gateway: resolve omni %q: %w", ref, rerr)
	}
	if sysInstr == "" {
		return OmniResult{}, fmt.Errorf("gateway: omni %q resolved to empty sys-instructions", ref)
	}
	// Refuse sys-instructions that would collide with graft's omni sentinel
	// markers: a stray close marker (or open-marker prefix) inside the block would
	// make it self-corrupting — the next refresh's stripLeadingOmniBlock would
	// match the embedded close line and truncate mid-content. Fail safe BEFORE any
	// Body write (CreateAgentWithOmni rolls back the scaffold on this error).
	if canonical.ContainsOmniMarker(sysInstr) {
		return OmniResult{}, fmt.Errorf("gateway: omni %q resolved to sys-instructions containing a graft sentinel marker (refusing to apply)", ref)
	}

	a.Body = canonical.ReplaceOmniBlock(a.Body, ref, sysInstr)
	meta.Omni = &contract.OmniRef{Ref: ref, Applied: true, Supported: true}
	if perr := g.persistAgent(*a, meta); perr != nil {
		return OmniResult{}, perr
	}
	return OmniResult{Ref: ref, Supported: true, Applied: true}, nil
}

// persistAgent re-renders the agent + meta to disk (preserving the supplied
// provider-hash meta). CanonicalHash is recomputed inside SaveWithMeta.
func (g *gate) persistAgent(a contract.CanonicalAgent, meta canonical.Meta) error {
	writes, err := canonical.SaveWithMeta(g.root, a, meta)
	if err != nil {
		return fmt.Errorf("gateway: save agent %q: %w", a.Name, err)
	}
	if err := writeFiles(writes); err != nil {
		return fmt.Errorf("gateway: write agent %q: %w", a.Name, err)
	}
	return nil
}
