// Package status computes in-sync / drift state for tracked agents. It bridges
// the canonical store (.graft/agents/<name>) and the live provider files on
// disk: for each agent it recomputes what every registered provider's file(s)
// SHOULD contain from the canonical form, compares that to what is actually on
// disk, and reports per-provider sync state. It also consults store.Drift for
// the recorded-hash view.
package status

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/canonical"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// Reporter answers status/list queries for a workspace root.
type Reporter struct {
	store contract.Store
	tr    contract.Transformer
	root  string
}

// New constructs a Reporter over the given dependencies and workspace root.
func New(store contract.Store, tr contract.Transformer, root string) *Reporter {
	return &Reporter{store: store, tr: tr, root: root}
}

// List returns the per-agent, per-provider sync state for every agent tracked
// under .graft/agents.
func (r *Reporter) List() ([]contract.AgentStatus, error) {
	names, err := r.agentNames()
	if err != nil {
		return nil, err
	}
	out := make([]contract.AgentStatus, 0, len(names))
	for _, name := range names {
		st, err := r.agentStatus(name)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// Status returns a StatusReport for one agent (when name != nil) or all agents,
// aggregating how many agents each provider is out of sync for.
func (r *Reporter) Status(name *string) (contract.StatusReport, error) {
	var statuses []contract.AgentStatus
	if name != nil {
		st, err := r.agentStatus(*name)
		if err != nil {
			return contract.StatusReport{}, err
		}
		statuses = []contract.AgentStatus{st}
	} else {
		var err error
		statuses, err = r.List()
		if err != nil {
			return contract.StatusReport{}, err
		}
	}

	report := contract.StatusReport{
		Agents:             statuses,
		OutOfSyncProviders: map[string]int{},
	}
	for _, st := range statuses {
		for prov, inSync := range st.Providers {
			if !inSync {
				report.OutOfSyncProviders[prov]++
			}
		}
	}
	return report, nil
}

// agentStatus recomputes one agent's per-provider sync state by comparing the
// canonical-derived expected file content against what is on disk.
func (r *Reporter) agentStatus(name string) (contract.AgentStatus, error) {
	st := contract.AgentStatus{Name: name, Providers: map[string]bool{}, InSync: true}

	can, err := canonical.Load(canonical.AgentDir(r.root, name))
	if err != nil {
		// No canonical form → nothing to compare against; treat as out of sync.
		st.InSync = false
		return st, nil
	}

	for _, provName := range r.tr.Providers() {
		prov, ok := r.tr.Provider(provName)
		if !ok {
			continue
		}
		// Only report providers that actually have a file for this agent on disk.
		refs, derr := prov.Detect(r.root)
		if derr != nil {
			continue
		}
		var onDisk *contract.AgentRef
		for i := range refs {
			if refs[i].Name == name {
				onDisk = &refs[i]
				break
			}
		}
		if onDisk == nil {
			continue
		}

		inSync := r.providerInSync(can, provName, *onDisk)
		st.Providers[provName] = inSync
		if !inSync {
			st.InSync = false
		}
	}
	return st, nil
}

// providerInSync renders the canonical agent for one provider and compares the
// resulting file content to the bytes currently on disk.
func (r *Reporter) providerInSync(can contract.CanonicalAgent, provName string, ref contract.AgentRef) bool {
	writes, err := r.tr.FromCanonical(can, provName)
	if err != nil {
		return false
	}
	for _, w := range writes {
		abs := w.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.root, w.Path)
		}
		actual, err := os.ReadFile(abs)
		if err != nil {
			return false
		}
		if hashBytes(actual) != hashBytes(w.Data) {
			return false
		}
	}
	return true
}

// agentNames lists agent directories under .graft/agents.
func (r *Reporter) agentNames() ([]string, error) {
	dir := filepath.Join(r.root, ".graft", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
