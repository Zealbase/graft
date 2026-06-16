package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// This file implements the provider-granular branch+merge algorithm (bug 3).
//
// The merge SURFACE is the canonical file (.graft/agents/<name>/agent.yaml +
// instructions.md). For every changed agent we:
//
//  1. Build a common-ancestor canonical (the agent's last-synced canonical, or an
//     empty skeleton on first sync) and commit it on a shared `canonbase` branch
//     cut from the workspace base branch.
//  2. For EACH changed provider file, cut its own branch from `canonbase`, fold
//     ONLY that provider's parsed canonical onto the ancestor, canonical.Save +
//     commit. Divergent providers that touch the SAME canonical field now change
//     the SAME line relative to the ancestor.
//  3. Merge those per-provider branches SEQUENTIALLY into the global beta branch
//     (also cut from `canonbase`). git performs a real three-way merge against
//     the common ancestor:
//       - non-overlapping edits auto-merge -> proceed (no user),
//       - same-line divergence -> standard git conflict markers land in beta's
//         worktree canonical file; we surface (agent,path), set status=conflict,
//         and STOP. The half-finished merge state is left on disk so --continue
//         can complete it after the user edits the markers out.
//
// Capability variance is never a conflict: a provider that cannot express a
// field simply does not set it in ToCanonical, so its per-provider canonical
// keeps the ancestor's value for that line (no divergence).

// providerSource is one changed provider file for an agent, already parsed.
type providerSource struct {
	provider string
	ref      contract.AgentRef
	parsed   contract.ProviderAgent
	srcHash  string // content hash of the on-disk provider file (Raw)
}

// agentWork is the per-agent unit of merge work: the agent name, its
// last-synced (ancestor) canonical, and the provider files that changed.
//
// Note: whether an agent became work via a changed provider file, a
// canonical-as-source edit, or ingestion is NOT carried here — the downstream
// fan-out is uniform (applyProviders force-writes every enabled provider for
// each name in result.Changed regardless of why it drifted), so no per-reason
// branch exists or should be added.
type agentWork struct {
	name     string
	ancestor contract.CanonicalAgent // common base canonical for the 3-way merge
	changed  []providerSource        // changed provider files (sorted by provider)
}

// canonBaseBranch is the shared common-ancestor branch for a run.
func canonBaseBranch(runID string) string {
	return fmt.Sprintf("graft/%s/canonbase", runID)
}

// providerBranch is the per-agent, per-provider branch ref.
func providerBranch(runID, agent, provider string) string {
	return fmt.Sprintf("graft/%s/agent/%s/%s", runID, agent, provider)
}

// hashBytes returns the content hash used for provider source-hash bookkeeping.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// buildAgentWork turns the diffed agents into per-agent merge units, computing
// each agent's ancestor canonical (from the existing .graft on the base branch,
// if any) and the set of provider files whose content changed since last sync.
//
// An agent becomes work when ANY of the following drifted (the union of change
// sources, plan-sync task 1 §canonical-as-source):
//   - a detected provider file's content differs from its recorded SourceHash;
//   - the canonical itself differs from meta.CanonicalHash (a direct edit to
//     .graft/agents/<n>/* — propagates to every enabled provider);
//   - the agent has no .graft canonical yet (ingestion from provider-only files,
//     gated on opts.Ingest, plan-sync task 5).
//
// opts.Ingest gates the create-if-missing path: when false, an agent that exists
// ONLY in a provider directory (no .graft canonical) is skipped.
//
// dryRun makes the deletion path SIDE-EFFECT-FREE (v0.0.4 verify r2 HIGH 1): a
// `graft sync --dry-run` must mutate NOTHING. On dry-run the would-be-deleted
// agent names are collected and returned (so the caller can report them) instead
// of removing provider files or deleting db rows.
func (e *Engine) buildAgentWork(wsID string, changed []changedAgent, ingest, dryRun bool) (works []agentWork, deleted []string, err error) {
	for _, ca := range changed {
		ancestor, prevMeta := e.ancestorCanonical(ca.name)
		canonExists := e.canonicalExists(ca.name)

		// Deletion-respecting ingestion gate (v0.0.4 verify task 3). An agent that
		// exists ONLY as provider file(s) (no .graft canonical) is normally
		// INGESTED. But if a prior sync COMPLETED for this agent, its canonical was
		// DELETED after that sync — re-ingesting it would RESURRECT a deleted agent.
		// Distinguish the two via the prior-sync-completed signal (an agents row
		// AND ≥1 provider_links row, see agentPriorSyncCompleted):
		//   - never completed -> genuinely new, provider-authored -> ingest.
		//   - completed before -> canonical deleted -> propagate the DELETE: remove
		//     the agent's file from every (enabled+detected) provider and delete its
		//     db rows; do NOT ingest.
		// Gated strictly on the completed-sync signal so an agent that was never
		// fully synced (incl. an orphan agents row from a prior aborted run) is
		// never deleted (v0.0.4 verify r2 HIGH 2). Deleting from a SINGLE provider
		// while the canonical still exists is unaffected: canonExists is true here,
		// so this branch is skipped and the canonical restores it as before.
		//
		// DRY-RUN (v0.0.4 verify r2 HIGH 1): on --dry-run we mutate NOTHING — the
		// would-be-deleted name is collected and returned instead of removing files
		// or db rows.
		if !canonExists && len(ca.sources) > 0 {
			completed, perr := e.agentPriorSyncCompleted(wsID, ca.name)
			if perr != nil {
				return nil, nil, perr
			}
			if completed {
				if dryRun {
					deleted = append(deleted, ca.name)
					continue // dry-run: report only, no mutation
				}
				if derr := e.deleteAgentEverywhere(wsID, ca.name, ca.sources); derr != nil {
					return nil, nil, derr
				}
				deleted = append(deleted, ca.name)
				continue // deletion handled: not work, not ingested
			}
		}

		var srcs []providerSource
		for _, pa := range ca.sources {
			provName := pa.Provider
			srcHash := hashBytes(pa.Raw)
			// Changed if there is no recorded source hash for this provider, or it
			// differs from the recorded one.
			prev, ok := prevMeta.Providers[provName]
			if ok && prev.SourceHash == srcHash {
				continue // unchanged provider file
			}
			srcs = append(srcs, providerSource{
				provider: provName,
				ref:      pa.Ref,
				parsed:   pa,
				srcHash:  srcHash,
			})
		}
		sort.Slice(srcs, func(i, j int) bool { return srcs[i].provider < srcs[j].provider })

		// Canonical-as-source: the canonical drifted from its last-recorded hash
		// (a direct edit), or meta is missing while a canonical exists. Either way
		// the canonical edit must fan out to every enabled provider.
		canonChanged := canonExists &&
			(prevMeta.CanonicalHash == "" || canonical.Hash(ancestor) != prevMeta.CanonicalHash)

		// Subset-sync stale detection (review r2 HIGH). A prior `sync --providers=P`
		// may have advanced the canonical (and stamped Meta.CanonicalHash) WITHOUT
		// rewriting providers outside the subset, leaving those providers' on-disk
		// files written from an OLDER canonical. Their SourceHash still matches their
		// (unchanged) bytes and canonChanged is false (the canonical itself did not
		// drift since last sync), so neither check above catches them. Detect it via
		// the per-provider CanonicalHash: any ENABLED provider whose file was last
		// written from a canonical other than the current ancestor is stale and the
		// agent must become work so applyProviders force-rewrites it.
		canonStale := false
		if canonExists {
			ancHash := canonical.Hash(ancestor)
			for p, pm := range prevMeta.Providers {
				if !e.providerEnabled(p) {
					continue // out-of-subset providers are healed on a sync that includes them
				}
				if pm.CanonicalHash != ancHash {
					canonStale = true
					break
				}
			}
		}

		// Ingestion: a provider-only agent (detected providers but no .graft
		// canonical) is created from its provider file(s). Gated on opts.Ingest.
		ingested := !canonExists && len(ca.sources) > 0
		if ingested && !ingest {
			continue // ingestion disabled: skip provider-only agent
		}

		// Never-synced fan-out (v0.0.4 verify task 2). A freshly-scaffolded
		// `graft agent init` writes the canonical via SaveWithMeta(root, a, Meta{}),
		// which ALWAYS stamps Meta.CanonicalHash = Hash(a) but leaves Providers
		// empty. That makes canonChanged false (hash matches), canonStale false
		// (no provider entries to compare), and srcs==0 (no provider files yet) —
		// so the agent would be SKIPPED and never written to any provider. Detect
		// the brand-new scaffold by: a canonical exists but NO provider has ever
		// been written from it (Providers empty). Force it into the work set so
		// applyProviders fans it out to every enabled provider and stamps the
		// per-provider meta. After the first sync Providers is populated, so this
		// only ever triggers for a never-propagated canonical (no spurious
		// re-fan-out of already-synced agents).
		//
		// FRESH-CLONE canonical-only fan-out (v0.0.5). A repo cloned/copied with
		// ONLY .graft/ (its .meta.json already lists Providers from the source
		// machine's sync) but WITHOUT the provider files on disk (gitignored / not
		// committed) makes the Providers-empty test FALSE — yet nothing has been
		// fanned out on THIS machine, so the agent would be wrongly skipped and the
		// provider files never created. Discriminate on the FILESYSTEM, not on what
		// .meta.json claims: the agent needs fan-out when ANY enabled provider whose
		// file SHOULD exist (it is recorded in prevMeta.Providers) is actually ABSENT
		// on disk. When every recorded provider file IS present (the genuine fresh-
		// clone-with-providers no-op case, FC-a) this stays false and the agent is a
		// no-op as before.
		neverSynced := canonExists &&
			(len(prevMeta.Providers) == 0 || e.recordedProviderFileMissing(ancestor, prevMeta))

		// Nothing actually drifted for this agent: no changed provider file, the
		// canonical is unchanged, no provider is stale against the current canonical,
		// it is not a new ingestion, and it has been propagated before. Skip it.
		if len(srcs) == 0 && !canonChanged && !canonStale && !ingested && !neverSynced {
			continue
		}

		// The set of providers whose override bucket this run REBUILDS from their
		// current fold: exactly the providers whose file CHANGED this run (srcs).
		// Rebuilding from the current parse makes a removed key disappear (the
		// deletion-aware fix). A provider that did NOT change this run — whether
		// detected-but-identical or not synced at all — keeps its PRIOR bucket
		// verbatim (its current content equals prior, so there is nothing to
		// rebuild, and we must never drop a non-changing provider's data).
		owned := map[string]bool{}
		for _, s := range srcs {
			owned[s.provider] = true
		}

		enriched, eerr := e.enrichAncestor(ancestor, srcs, owned)
		if eerr != nil {
			return nil, nil, eerr
		}
		works = append(works, agentWork{
			name:     ca.name,
			ancestor: enriched,
			changed:  srcs,
		})
	}
	return works, deleted, nil
}

// enrichAncestor builds the three-way merge common ancestor. Starting from the
// last-synced canonical, for each field it pre-seeds the value the changed
// providers AGREE on (or that only one provider expresses). This keeps the
// ancestor close to both sides so non-conflicting / capability-variance edits do
// not show up as spurious add/add insertions at the same anchor. Where the
// changed providers DISAGREE on a field, the ancestor keeps the prior value (or
// stays empty on first sync), so each per-provider branch changes that one line
// differently -> a genuine git conflict on exactly that field.
func (e *Engine) enrichAncestor(prior contract.CanonicalAgent, srcs []providerSource, owned map[string]bool) (contract.CanonicalAgent, error) {
	folds := make([]contract.CanonicalAgent, len(srcs))
	for i, src := range srcs {
		f, err := e.foldProvider(prior, src)
		if err != nil {
			return contract.CanonicalAgent{}, err
		}
		folds[i] = f
	}

	anc := prior
	if anc.Name == "" && len(folds) > 0 {
		anc.Name = folds[0].Name
	}

	// description / model: agreed scalar -> seed; disagreement -> keep prior.
	anc.Description = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Description }), prior.Description)
	anc.Model = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Model }), prior.Model)
	anc.Body = agreedScalar(foldsField(folds, func(c contract.CanonicalAgent) string { return c.Body }), prior.Body)

	// tools / mcp: agreed slice (order-insensitive) -> seed; else keep prior.
	anc.Tools = agreedSlice(folds, func(c contract.CanonicalAgent) []string { return c.Tools }, prior.Tools)
	anc.MCP = agreedSlice(folds, func(c contract.CanonicalAgent) []string { return c.MCP }, prior.MCP)
	anc.Skills = agreedSlice(folds, func(c contract.CanonicalAgent) []string { return c.Skills }, prior.Skills)

	// permissions: agreed map -> seed; disagreement -> keep prior. Without this
	// an agreed permissions change lags the ancestor and shows up as a spurious
	// add/add in every per-provider branch (noisy 3-way merge).
	anc.Permissions = agreedMap(folds, func(c contract.CanonicalAgent) map[string]string { return c.Permissions }, prior.Permissions)

	// providerOverrides: DELETION-AWARE (v0.0.3 task 2). A provider's override
	// bucket is owned SOLELY by that provider, so the ancestor's bucket for a
	// provider being synced this run is REBUILT from that provider's CURRENT
	// branch fold — never carried (unioned) from the prior canonical. A key that
	// is absent in the current provider file but was present in prior is therefore
	// DROPPED (the user deleted it) rather than resurrected.
	//
	// GUARD: only buckets for providers in this run's enabled+detected set
	// (`owned`) are rebuilt. A provider not synced this run keeps its prior bucket
	// verbatim, so we never drop data for a disabled/undetected provider.
	merged := map[string]map[string]any{}
	for k, v := range prior.ProviderOverrides {
		if owned[k] {
			continue // owner is syncing this run -> rebuilt from its fold below
		}
		merged[k] = v
	}
	for i, src := range srcs {
		// folds[i] already holds this provider's fold (foldProvider REPLACES, not
		// unions, the provider's own bucket), so its bucket for src.provider is
		// exactly what the current parse expressed. Absent -> the bucket is not set,
		// i.e. the user deleted it. Reuse folds[i] rather than re-parsing.
		if b, ok := folds[i].ProviderOverrides[src.provider]; ok && len(b) > 0 {
			merged[src.provider] = b
		}
		// else: provider expressed no overrides this run -> bucket deleted.
	}
	if len(merged) > 0 {
		anc.ProviderOverrides = merged
	} else {
		anc.ProviderOverrides = nil
	}
	return anc, nil
}

// foldsField extracts one string field across all folds.
func foldsField(folds []contract.CanonicalAgent, get func(contract.CanonicalAgent) string) []string {
	out := make([]string, 0, len(folds))
	for _, f := range folds {
		out = append(out, get(f))
	}
	return out
}

// agreedScalar returns the common non-empty value if every fold that sets the
// field sets the SAME value; otherwise it returns the prior value (forcing a
// genuine conflict between the diverging per-provider branches).
func agreedScalar(vals []string, prior string) string {
	seen := ""
	for _, v := range vals {
		if v == "" {
			continue
		}
		if seen == "" {
			seen = v
		} else if seen != v {
			return prior // disagreement -> keep prior so the branches conflict
		}
	}
	if seen == "" {
		return prior
	}
	return seen
}

// agreedSlice is agreedScalar for string slices. Agreement is ORDER-INSENSITIVE:
// two providers listing the same tools in a different order agree (the canonical
// value is a set), so the agreed value seeds the ancestor instead of falling back
// to prior. The first provider's ORIGINAL ordering is preserved in the result.
func agreedSlice(folds []contract.CanonicalAgent, get func(contract.CanonicalAgent) []string, prior []string) []string {
	var seen []string
	var seenKey string
	have := false
	for _, f := range folds {
		v := get(f)
		if len(v) == 0 {
			continue
		}
		key := sliceKey(v)
		if !have {
			seen = v
			seenKey = key
			have = true
		} else if seenKey != key {
			return prior
		}
	}
	if !have {
		return prior
	}
	return seen
}

// sliceKey is an order-insensitive identity for a string slice (sorted+joined).
func sliceKey(v []string) string {
	cp := append([]string(nil), v...)
	sort.Strings(cp)
	return strings.Join(cp, "\x00")
}

// agreedMap is agreedScalar for string maps: it returns the common map if every
// fold that sets it agrees key-for-key (compared as a sorted canonical form);
// otherwise it returns prior (forcing a genuine conflict between the diverging
// per-provider branches).
func agreedMap(folds []contract.CanonicalAgent, get func(contract.CanonicalAgent) map[string]string, prior map[string]string) map[string]string {
	var seen map[string]string
	var seenKey string
	have := false
	for _, f := range folds {
		m := get(f)
		if len(m) == 0 {
			continue
		}
		key := mapKey(m)
		if !have {
			seen = m
			seenKey = key
			have = true
		} else if seenKey != key {
			return prior
		}
	}
	if !have {
		return prior
	}
	return seen
}

// mapKey is a deterministic identity for a string map (sorted key=value pairs).
func mapKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte('\x00')
	}
	return b.String()
}

// ancestorCanonical loads the agent's last-synced canonical (the 3-way merge
// ancestor) and its recorded per-provider source hashes. On first sync there is
// no canonical on disk yet; we return an empty skeleton carrying just the name.
func (e *Engine) ancestorCanonical(name string) (contract.CanonicalAgent, canonical.Meta) {
	dir := canonical.AgentDir(e.root, name)
	can, err := canonical.Load(dir)
	if err != nil {
		return contract.CanonicalAgent{Name: name}, canonical.Meta{}
	}
	meta, _ := canonical.LoadMeta(dir)
	if can.Name == "" {
		can.Name = name
	}
	return can, meta
}

// overridePermissionsField is the override-key name for the permissions field.
const overridePermissionsField = "permissions"

// foldProvider folds one provider's parsed canonical onto the ancestor: the
// provider's expressed fields override, and its ProviderOverrides are merged in.
// Fields the provider does not express keep the ancestor's value (so capability
// variance never shows up as a change).
//
// CANONICAL-FIELD OVERRIDE RESURRECTION GUARD (v0.0.4 conformance r1 HIGH 2).
// A per-provider override of a CANONICAL field (e.g.
// ProviderOverrides["claude-code"]["description"]) is written to the provider
// file by RestoreOverrides and so reappears on the NEXT parse as a plain
// canonical field (pc.Description), NOT inside pc.ProviderOverrides (Extras
// strips known keys). If we promoted that value into the SHARED canonical it
// would overwrite the real shared value for every other provider — silent data
// loss. The discriminator is the PRIOR override bucket: if the ancestor already
// recorded this field as an override for THIS provider, the parsed value is that
// provider's override — keep it in the provider's bucket and do NOT let it touch
// the shared canonical field. If there was NO prior override for the field, the
// field is genuinely shared and propagates exactly as before.
func (e *Engine) foldProvider(ancestor contract.CanonicalAgent, src providerSource) (contract.CanonicalAgent, error) {
	pc, err := e.tr.ToCanonical(src.parsed)
	if err != nil {
		return contract.CanonicalAgent{}, fmt.Errorf("sync: tocanonical %s/%s: %w", src.ref.Name, src.provider, err)
	}
	out := ancestor
	out.Name = firstNonEmpty(ancestor.Name, pc.Name, src.ref.Name)

	// prevBucket is the ancestor's override bucket for this provider — the
	// discriminator for the resurrection guard. reclaimed collects the canonical
	// fields whose parsed value is actually this provider's override (so they are
	// re-stashed in the bucket below instead of folded into the shared canonical).
	prevBucket := ancestor.ProviderOverrides[src.provider]
	reclaimed := map[string]any{}
	wasOverride := func(field string) bool {
		if prevBucket == nil {
			return false
		}
		_, ok := prevBucket[field]
		return ok
	}

	if pc.Description != "" {
		if wasOverride("description") {
			reclaimed["description"] = pc.Description
		} else {
			out.Description = pc.Description
		}
	}
	if pc.Model != "" {
		if wasOverride("model") {
			reclaimed["model"] = pc.Model
		} else {
			out.Model = pc.Model
		}
	}
	if len(pc.Tools) > 0 {
		if wasOverride("tools") {
			reclaimed["tools"] = pc.Tools
		} else {
			out.Tools = pc.Tools
		}
	}
	if len(pc.MCP) > 0 {
		if wasOverride("mcp") {
			reclaimed["mcp"] = pc.MCP
		} else {
			out.MCP = pc.MCP
		}
	}
	if len(pc.Skills) > 0 {
		if wasOverride("skills") {
			reclaimed["skills"] = pc.Skills
		} else {
			out.Skills = pc.Skills
		}
	}
	if len(pc.Permissions) > 0 {
		if wasOverride(overridePermissionsField) {
			reclaimed[overridePermissionsField] = pc.Permissions
		} else {
			out.Permissions = pc.Permissions
		}
	}
	if pc.Body != "" {
		out.Body = pc.Body
	}
	// Rebuild the provider-overrides map DELETION-AWARELY (v0.0.3 task 2). This
	// provider OWNS its own bucket: the fold's bucket for src.provider is set to
	// EXACTLY what the current parse expressed (which may be absent -> the bucket
	// is dropped, i.e. the user deleted those keys). Every OTHER provider's bucket
	// is carried from the ancestor unchanged (this fold does not own them).
	merged := map[string]map[string]any{}
	for k, v := range ancestor.ProviderOverrides {
		if k == src.provider {
			continue // owned by this fold -> replaced below (or deleted if absent)
		}
		merged[k] = v
	}
	// The provider's own bucket = the non-canonical extras it expressed this run,
	// plus any reclaimed canonical-field overrides (resurrection guard). A
	// canonical field that WAS a prior override but is now ABSENT in the parse is
	// simply not reclaimed -> the override is dropped (deletion-aware, consistent
	// with the extras-bucket rule).
	bucket := map[string]any{}
	if b, ok := pc.ProviderOverrides[src.provider]; ok {
		for k, v := range b {
			bucket[k] = v
		}
	}
	for k, v := range reclaimed {
		bucket[k] = v
	}
	if len(bucket) > 0 {
		merged[src.provider] = bucket
	}
	if len(merged) > 0 {
		out.ProviderOverrides = merged
	} else {
		out.ProviderOverrides = nil
	}
	return out, nil
}

// recordedProviderFileMissing reports whether ANY enabled provider that
// prevMeta records as having been written for this agent has its serialized file
// ABSENT on disk. It powers the fresh-clone canonical-only fan-out (v0.0.5): a
// repo copied with only .graft/ (whose .meta.json lists providers from the source
// machine) but without the provider files needs a fan-out even though
// prevMeta.Providers is non-empty.
//
// The provider's expected path is derived the SAME way applyProviders writes it:
// FromCanonical(ancestor, provider)[0].Path resolved under the provider's scope
// base. A provider that cannot express this agent (zero writes) is skipped — it
// never had a file, so its absence is not a missing file. Providers outside the
// enabled subset are ignored (they are healed by a sync that includes them).
//
// Any error resolving the base or rendering is treated as "not missing" so this
// purely-additive probe never turns a healthy agent into spurious work.
func (e *Engine) recordedProviderFileMissing(ancestor contract.CanonicalAgent, prevMeta canonical.Meta) bool {
	for provName := range prevMeta.Providers {
		if !e.providerEnabled(provName) {
			continue
		}
		base, err := e.providerBase(provName)
		if err != nil {
			continue
		}
		writes, err := e.tr.FromCanonical(ancestor, provName)
		if err != nil || len(writes) == 0 {
			continue // provider cannot express this agent -> no file expected
		}
		abs := writes[0].Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(base, abs)
		}
		if _, err := os.Stat(abs); err != nil {
			// Fail SAFE into fan-out on ANY stat error, not just IsNotExist: an
			// inaccessible-but-maybe-present provider file (EACCES, stale NFS
			// handle, symlink to an unreadable target) should re-write rather than
			// silently skip the agent. The downside is at most one spurious
			// re-write — applyProviders guards its own writes — versus the much
			// worse failure of leaving a fresh clone unfanned (v0.0.5 review).
			return true // a recorded provider file is gone/unreadable -> needs fan-out
		}
	}
	return false
}

// canonicalExists reports whether a .graft/agents/<name>/agent.yaml is present
// on disk (i.e. the agent already has a canonical store entry). Used to tell a
// provider-only agent (needs ingestion) from one with an existing canonical.
func (e *Engine) canonicalExists(name string) bool {
	dir := canonical.AgentDir(e.root, name)
	if _, err := os.Stat(filepath.Join(dir, "agent.yaml")); err == nil {
		return true
	}
	return false
}

// agentDeleter is an OPTIONAL store capability: deleting one agent's rows
// (agents + provider_links + agent_states) for a workspace. The sync engine
// uses it to propagate a canonical deletion as a real delete (v0.0.4 verify
// task 3). It is an optional interface (type-asserted off contract.Store) so
// the deletion path degrades gracefully on a store that has not implemented it
// yet (the files are still removed; the db rows are left for the store owner).
type agentDeleter interface {
	DeleteAgent(wsID, name string) error
}

// agentPriorSyncCompleted reports whether a prior sync COMPLETED for this agent
// — the deletion signal. It uses store.AgentSynced, which is true only when an
// agents row AND ≥1 provider_links row exist. The provider_links requirement is
// the robust discriminator (v0.0.4 verify r2 HIGH 2): the previous probe used
// store.Drift, whose reason "no provider links" (an agents row with ZERO links,
// e.g. a prior ABORTED run that called UpsertAgent in prepareBranches but never
// reached applyProviders) is != "agent not tracked" and so was mis-read as
// "known" — DELETING a legitimately-new provider-authored agent. A provider link
// is only ever recorded AFTER the resolved canonical lands (applyProviders), so
// AgentSynced == true means a full sync genuinely completed for this agent.
func (e *Engine) agentPriorSyncCompleted(wsID, name string) (bool, error) {
	synced, err := e.store.AgentSynced(wsID, name)
	if err != nil {
		return false, fmt.Errorf("sync: prior-sync probe %s: %w", name, err)
	}
	return synced, nil
}

// deleteAgentEverywhere propagates a canonical deletion: it removes the agent's
// provider file from every (enabled+detected) provider source and deletes its
// db rows. The provider sources are the files Detect found this run for the
// enabled subset — exactly the locations the agent still lives in. Provider
// files outside the enabled subset are intentionally left untouched (subset
// semantics); a later full sync removes them too.
func (e *Engine) deleteAgentEverywhere(wsID, name string, sources []contract.ProviderAgent) error {
	// DB-FIRST ordering (v0.0.4 verify r2 MED 3). Delete the db rows BEFORE the
	// provider files. A DB failure then leaves EVERYTHING intact (files + rows),
	// so the next sync re-attempts the deletion cleanly. The opposite order risks
	// removing the files, then failing the db delete — leaving orphan rows that
	// would mis-classify the (now file-less) agent on the next run. With db-first,
	// a db success + later file-removal failure is RECOVERABLE: the leftover
	// provider files simply re-ingest cleanly on a later sync (no db rows remain,
	// so they read as a genuinely-new provider-authored agent).
	if d, ok := e.store.(agentDeleter); ok {
		if err := d.DeleteAgent(wsID, name); err != nil {
			return fmt.Errorf("sync: delete db rows for %s: %w", name, err)
		}
	}
	for _, pa := range sources {
		p := pa.Ref.Path
		if p == "" {
			continue
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("sync: delete provider file %s for %s: %w", p, name, err)
		}
		// Per-agent-dir providers (e.g. antigravity's home-scoped
		// <home>/.gemini/antigravity-cli/agents/<name>/agent.json) keep the agent's
		// file in its OWN <name>/ subdirectory. Removing just the file leaves an
		// empty dir behind, and a stale empty dir would make Detect skip cleanly but
		// litter $HOME. When the file's parent dir is named exactly <name>, RemoveAll
		// the whole per-agent subdir (v0.0.4 verify r2 LOW 4). Best-effort: a failure
		// here is not fatal (the agent.json — the only thing Detect keys on — is
		// already gone).
		if filepath.Base(filepath.Dir(p)) == name {
			_ = os.RemoveAll(filepath.Dir(p))
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// prepareBranches builds the common-ancestor branch and the per-provider
// branches for every agent. It returns the canonbase branch name and, per agent,
// the ordered list of per-provider branch refs to merge.
func (e *Engine) prepareBranches(ws contract.Workspace, run contract.SyncRun, works []agentWork) (string, map[string][]string, error) {
	base := run.BaseBranch
	canonbase := canonBaseBranch(run.RunID)

	// 1. Common-ancestor branch with every agent's ancestor canonical committed.
	if err := e.git.Branch(canonbase, base); err != nil {
		return "", nil, fmt.Errorf("sync: canonbase branch: %w", err)
	}
	cbWT, err := e.git.Worktree(canonbase, canonbase)
	if err != nil {
		return "", nil, fmt.Errorf("sync: canonbase worktree: %w", err)
	}
	for _, w := range works {
		anc := w.ancestor
		if anc.Name == "" {
			anc.Name = w.name
		}
		if err := writeCanonical(cbWT, anc); err != nil {
			return "", nil, err
		}
	}
	if _, err := commitWorktree(cbWT, "graft: common ancestor canonical"); err != nil {
		return "", nil, fmt.Errorf("sync: commit canonbase: %w", err)
	}

	// 2. Per-provider branches folding one provider's changes onto the ancestor.
	order := map[string][]string{}
	for _, w := range works {
		for _, src := range w.changed {
			folded, err := e.foldProvider(w.ancestor, src)
			if err != nil {
				return "", nil, err
			}
			branch := providerBranch(run.RunID, w.name, src.provider)
			if err := e.git.Branch(branch, canonbase); err != nil {
				return "", nil, fmt.Errorf("sync: branch %s: %w", branch, err)
			}
			wt, err := e.git.Worktree(branch, branch)
			if err != nil {
				return "", nil, fmt.Errorf("sync: worktree %s: %w", branch, err)
			}
			if err := writeCanonical(wt, folded); err != nil {
				return "", nil, err
			}
			head, err := commitWorktree(wt, fmt.Sprintf("graft: %s via %s", w.name, src.provider))
			if err != nil {
				return "", nil, fmt.Errorf("sync: commit %s: %w", branch, err)
			}
			_ = e.store.SaveBranch(contract.Branch{
				RunID: run.RunID, Name: branch, Kind: contract.BranchAgent,
				HeadHash: head, State: branchPending,
			})
			order[w.name] = append(order[w.name], branch)
		}
		// Record the agent identity (canonical hash filled after merge resolves).
		if _, err := e.store.UpsertAgent(contract.Agent{
			WsID: ws.ID, Name: w.name, CanonicalHash: canonical.Hash(w.ancestor),
		}); err != nil {
			return "", nil, fmt.Errorf("sync: upsert agent %s: %w", w.name, err)
		}
	}
	return canonbase, order, nil
}

// branch state values recorded in the branches table (State column) so a
// --continue can resume the merge loop exactly where it stopped.
const (
	branchPending  = "pending"  // per-provider branch not yet merged into beta
	branchMerged   = "merged"   // already merged into beta
	branchConflict = "conflict" // the branch whose merge conflicted (in progress)
)

// mergeOrder returns the deterministic global order in which per-provider
// branches are merged into beta: agents sorted by name, providers sorted within.
func mergeOrder(order map[string][]string) []mergeStep {
	var agents []string
	for a := range order {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	var steps []mergeStep
	for _, a := range agents {
		for _, b := range order[a] {
			steps = append(steps, mergeStep{agent: a, branch: b})
		}
	}
	return steps
}

type mergeStep struct {
	agent  string
	branch string
}

// mergeInto runs the sequential three-way merge of every per-provider branch in
// `steps` into the beta branch (via the GitX seam, so a test fake can intercept
// the merge OUTCOME). It begins after `startIdx` (so --continue can skip
// already-merged branches). On a conflict it records branch state and returns
// the conflict; the conflicted (marker-bearing) state is left in beta's worktree
// for the user to resolve. On success it returns the final step index.
func (e *Engine) mergeInto(run contract.SyncRun, betaName, betaWT string, steps []mergeStep, startIdx int) ([]contract.Conflict, int, error) {
	for i := startIdx; i < len(steps); i++ {
		step := steps[i]
		mr, err := e.git.Merge(betaName, step.branch)
		if err != nil {
			return nil, i, fmt.Errorf("sync: merge %s: %w", step.branch, err)
		}
		if mr.Clean {
			_ = e.store.SaveBranch(contract.Branch{
				RunID: run.RunID, Name: step.branch, Kind: contract.BranchAgent, State: branchMerged,
			})
			continue
		}

		// Real conflict. Prefer the actual unmerged paths in beta's worktree (they
		// carry the markers the user will edit); fall back to what the GitX impl
		// reported (e.g. a test fake that does not touch the worktree).
		_ = e.store.SaveBranch(contract.Branch{
			RunID: run.RunID, Name: step.branch, Kind: contract.BranchAgent, State: branchConflict,
		})
		var conflicts []contract.Conflict
		if paths, perr := conflictPathsIn(betaWT); perr == nil && len(paths) > 0 {
			for _, p := range paths {
				conflicts = append(conflicts, contract.Conflict{Path: p, Agent: step.agent})
			}
		} else {
			for _, c := range mr.Conflicts {
				c.Agent = step.agent
				conflicts = append(conflicts, c)
			}
		}
		return conflicts, i, nil
	}
	return nil, len(steps), nil
}

// conflictPathsIn lists unmerged (conflicted) paths in a worktree.
func conflictPathsIn(dir string) ([]string, error) {
	out, err := gitInDir(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// surfaceConflictToWorkspace copies the conflicted (marker-bearing) canonical
// files from the beta worktree into the user's working tree under root so the
// user edits the SAME .graft/agents/<name>/agent.yaml they see in `graft status`.
func (e *Engine) surfaceConflictToWorkspace(betaWT string, conflicts []contract.Conflict) error {
	for _, c := range conflicts {
		from := filepath.Join(betaWT, c.Path)
		to := filepath.Join(e.root, c.Path)
		data, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("sync: read conflict %s: %w", from, err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return fmt.Errorf("sync: write conflict %s: %w", to, err)
		}
	}
	return nil
}

// betaWorktree returns the path of the beta branch's linked worktree, creating
// it if needed.
func (e *Engine) betaWorktree(betaBranch string) (string, error) {
	return e.git.Worktree(betaBranch, betaBranch)
}

// worktreeHasStagedOrMerge reports whether the worktree has an in-progress merge
// (MERGE_HEAD) or any staged changes — i.e. whether `git commit` would produce a
// commit. Used to tell a real conflict resolution from a phantom (test-fake) one.
func (e *Engine) worktreeHasStagedOrMerge(dir string) bool {
	if gitDir, err := gitInDir(dir, "rev-parse", "--git-dir"); err == nil {
		gd := strings.TrimSpace(gitDir)
		if !filepath.IsAbs(gd) {
			gd = filepath.Join(dir, gd)
		}
		if _, err := os.Stat(filepath.Join(gd, "MERGE_HEAD")); err == nil {
			return true
		}
	}
	// Staged changes? `git diff --cached --quiet` exits non-zero when staged.
	if _, err := gitInDir(dir, "diff", "--cached", "--quiet"); err != nil {
		return true
	}
	return false
}
