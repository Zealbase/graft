package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
	"gopkg.in/yaml.v3"
)

// printOutput renders v in the requested format to w. kind selects the table
// renderer. json/yaml payloads are always raw (no ANSI) for piped consumers.
func printOutput(w io.Writer, kind, format string, v any) error {
	switch format {
	case "json":
		return printJSON(w, unwrapForMachine(v))
	case "yaml", "yml":
		return printYAML(w, unwrapForMachine(v))
	case "table":
		return printTable(w, kind, v)
	default:
		return fmt.Errorf("unsupported output format %q (use: json|yaml|table)", format)
	}
}

// unwrapForMachine strips CLI-only presentation wrappers so json/yaml output
// stays the raw domain payload (e.g. syncView -> its RunResult).
func unwrapForMachine(v any) any {
	if sv, ok := v.(syncView); ok {
		return sv.Result
	}
	return v
}

// syncSummaryLine renders the plan-revise task-2 line:
// "{y} agents in sync with {x} providers".
func syncSummaryLine(agents, providers int) string {
	return fmt.Sprintf("%d %s in sync with %d %s",
		agents, plural(agents, "agent", "agents"),
		providers, plural(providers, "provider", "providers"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func printJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

func printYAML(w io.Writer, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, string(b))
	return err
}

// printTable dispatches by kind, falling back to JSON for unknown kinds.
func printTable(w io.Writer, kind string, v any) error {
	switch kind {
	case "init":
		return printInitTable(w, v)
	case "agent.list":
		return printAgentListTable(w, v)
	case "agent.create":
		return printAgentCreateTable(w, v)
	case "status":
		return printStatusTable(w, v)
	case "sync":
		return printRunResultTable(w, v)
	case "validate":
		return printFindingsTable(w, v)
	case "config":
		return printConfigTable(w, v)
	case "update":
		return printUpdateTable(w, v)
	case "destroy":
		return printDestroyTable(w, v)
	case "skill.list":
		return printSkillListTable(w, v)
	case "skill.status":
		return printSkillStatusTable(w, v)
	default:
		return printJSON(w, v)
	}
}

func printSkillListTable(w io.Writer, v any) error {
	skills, ok := v.([]contract.Skill)
	if !ok {
		return printJSON(w, v)
	}
	if len(skills) == 0 {
		_, err := fmt.Fprintln(w, "no canonical skills")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SKILL\tDIR")
	for _, s := range skills {
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, s.Dir)
	}
	return tw.Flush()
}

func printSkillStatusTable(w io.Writer, v any) error {
	states, ok := v.([]contract.SkillStatus)
	if !ok {
		return printJSON(w, v)
	}
	if len(states) == 0 {
		_, err := fmt.Fprintln(w, "no skill links")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SKILL\tPROVIDER\tSTATE\tLINK_PATH")
	for _, s := range states {
		lp := s.LinkPath
		if lp == "" {
			lp = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Skill, s.Provider, s.State, lp)
	}
	return tw.Flush()
}

func printInitTable(w io.Writer, v any) error {
	r, ok := v.(contract.InitResult)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "root\t%s\n", r.Root)
	fmt.Fprintf(tw, "git_mode\t%s\n", r.GitMode)
	fmt.Fprintf(tw, "created\t%t\n", r.Created)
	return tw.Flush()
}

func printAgentCreateTable(w io.Writer, v any) error {
	a, ok := v.(contract.CanonicalAgent)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "name\t%s\n", a.Name)
	if a.Description != "" {
		fmt.Fprintf(tw, "description\t%s\n", a.Description)
	}
	return tw.Flush()
}

func printAgentListTable(w io.Writer, v any) error {
	agents, ok := v.([]contract.AgentStatus)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tIN_SYNC\tPROVIDERS")
	for _, a := range agents {
		// In-sync agents render the flat coverage list. Drifted agents instead get
		// a "<k>/<n> in sync" summary so the count is legible, and the drifted
		// providers are surfaced on their own "out of sync: ..." cell instead of
		// being buried in the comma-packed coverage list (v0.0.4 verify).
		if a.InSync {
			fmt.Fprintf(tw, "%s\t%t\t%s\n", a.Name, a.InSync, providerCoverage(a.Providers))
			continue
		}
		inSync, drifted := splitCoverage(a.Providers)
		total := inSync + len(drifted)
		fmt.Fprintf(tw, "%s\t%t\t%d/%d in sync\n", a.Name, a.InSync, inSync, total)
		fmt.Fprintf(tw, "\t\tout of sync: %s\n", strings.Join(drifted, ","))
	}
	return tw.Flush()
}

// splitCoverage partitions a provider->inSync map into the count of in-sync
// providers and the sorted list of drifted (out-of-sync) provider ids.
func splitCoverage(m map[string]bool) (inSync int, drifted []string) {
	for p, ok := range m {
		if ok {
			inSync++
		} else {
			drifted = append(drifted, p)
		}
	}
	sort.Strings(drifted)
	return inSync, drifted
}

func printStatusTable(w io.Writer, v any) error {
	rep, ok := v.(contract.StatusReport)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tPROVIDER\tIN_SYNC")
	for _, a := range rep.Agents {
		provs := make([]string, 0, len(a.Providers))
		for p := range a.Providers {
			provs = append(provs, p)
		}
		sort.Strings(provs)
		if len(provs) == 0 {
			fmt.Fprintf(tw, "%s\t-\t%t\n", a.Name, a.InSync)
			continue
		}
		for _, p := range provs {
			fmt.Fprintf(tw, "%s\t%s\t%t\n", a.Name, p, a.Providers[p])
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(rep.OutOfSyncProviders) > 0 {
		fmt.Fprintln(w)
		sw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(sw, "OUT_OF_SYNC_PROVIDER\t#AGENTS")
		provs := make([]string, 0, len(rep.OutOfSyncProviders))
		for p := range rep.OutOfSyncProviders {
			provs = append(provs, p)
		}
		sort.Strings(provs)
		for _, p := range provs {
			fmt.Fprintf(sw, "%s\t%d\n", p, rep.OutOfSyncProviders[p])
		}
		return sw.Flush()
	}
	return nil
}

func printRunResultTable(w io.Writer, v any) error {
	var r contract.RunResult
	providerCount := -1
	skillCount := -1
	switch t := v.(type) {
	case syncView:
		r = t.Result
		providerCount = t.ProviderCount
		skillCount = t.SkillCount
	case contract.RunResult:
		r = t
	default:
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "run_id\t%s\n", r.RunID)
	fmt.Fprintf(tw, "status\t%s\n", r.Status)
	if len(r.Changed) > 0 {
		fmt.Fprintf(tw, "changed\t%d\n", len(r.Changed))
	}
	if len(r.Deleted) > 0 {
		fmt.Fprintf(tw, "deleted\t%d\n", len(r.Deleted))
	}
	if len(r.Conflicts) > 0 {
		fmt.Fprintf(tw, "conflicts\t%d\n", len(r.Conflicts))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	// plan-revise task 2 / v0.0.3 task 8: human summary line. A clean run (no
	// agents changed, no conflicts) reads "already in sync"; otherwise list the
	// reflected agents and the count line.
	if providerCount >= 0 {
		fmt.Fprintln(w)
		agentsClean := len(r.Changed) == 0 && len(r.Conflicts) == 0 && len(r.Deleted) == 0
		skillsClean := len(r.SkillsLinked) == 0 && len(r.SkillsConflicted) == 0 && len(r.SkillsPruned) == 0
		if agentsClean && skillsClean {
			// Fully in sync. Claim skills only when there are canonical skills and
			// skills are enabled (skillCount >= 0).
			if skillCount >= 0 {
				fmt.Fprintf(w, "already in sync (%d %s, %d %s)\n",
					providerCount, plural(providerCount, "provider", "providers"),
					skillCount, plural(skillCount, "skill", "skills"))
			} else {
				fmt.Fprintf(w, "already in sync (%d %s)\n",
					providerCount, plural(providerCount, "provider", "providers"))
			}
		} else {
			if len(r.Changed) > 0 {
				fmt.Fprintf(w, "synced: %s\n", strings.Join(r.Changed, ", "))
			}
			if len(r.Deleted) > 0 {
				fmt.Fprintf(w, "deleted: %s\n", strings.Join(r.Deleted, ", "))
			}
			if len(r.Changed) > 0 {
				fmt.Fprintf(w, "%s\n", syncSummaryLine(len(r.Changed), providerCount))
			}
			// Skill links created/repaired this run: report them so the user isn't
			// told "already in sync" when skill drift was actually healed.
			if n := len(r.SkillsLinked); n > 0 {
				fmt.Fprintf(w, "linked %d %s: %s\n",
					n, plural(n, "skill", "skills"), strings.Join(r.SkillsLinked, ", "))
			}
			// Dead/dangling skill links pruned this run (canonical skill deleted,
			// provider symlink left broken). Report so the cleanup isn't silent.
			if n := len(r.SkillsPruned); n > 0 {
				fmt.Fprintf(w, "pruned %d dead skill %s: %s\n",
					n, plural(n, "link", "links"), strings.Join(r.SkillsPruned, ", "))
			}
		}
		// Skill conflicts are surfaced as a warning regardless of agent state: a
		// real dir/file occupies the link path and the skill is NOT linked. Never
		// claim full in-sync while this is present (handled above: SkillsConflicted
		// makes skillsClean false).
		if n := len(r.SkillsConflicted); n > 0 {
			fmt.Fprintf(w, "warning: %d %s in conflict (real dir/file at link path; re-run `graft skill sync --override` to replace): %s\n",
				n, plural(n, "skill", "skills"), strings.Join(r.SkillsConflicted, ", "))
		}
	}
	if len(r.Conflicts) > 0 {
		fmt.Fprintln(w)
		cw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(cw, "CONFLICT_AGENT\tPATH")
		for _, c := range r.Conflicts {
			fmt.Fprintf(cw, "%s\t%s\n", c.Agent, c.Path)
		}
		return cw.Flush()
	}
	return nil
}

func printFindingsTable(w io.Writer, v any) error {
	findings, ok := v.([]contract.Finding)
	if !ok {
		return printJSON(w, v)
	}
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "ok: no findings")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SEVERITY\tAGENT\tPROVIDER\tPATH\tMESSAGE")
	for _, f := range findings {
		prov := f.Provider
		if prov == "" {
			prov = "-"
		}
		path := f.Path
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", f.Severity, f.Agent, prov, path, f.Message)
	}
	return tw.Flush()
}

func printDestroyTable(w io.Writer, v any) error {
	r, ok := v.(contract.DestroyResult)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "removed_dir\t%t\n", r.RemovedDir)
	fmt.Fprintf(tw, "removed_rows\t%d\n", r.RemovedRows)
	fmt.Fprintf(tw, "removed_lock\t%t\n", r.RemovedLock)
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(w, "\ngraft state removed. Provider agent files were kept.")
	return nil
}

func printUpdateTable(w io.Writer, v any) error {
	r, ok := v.(contract.UpdateResult)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "current\t%s\n", r.Current)
	fmt.Fprintf(tw, "latest\t%s\n", r.Latest)
	fmt.Fprintf(tw, "updated\t%t\n", r.Updated)
	if err := tw.Flush(); err != nil {
		return err
	}
	if r.Notes != "" {
		fmt.Fprintf(w, "\n%s\n", r.Notes)
	}
	return nil
}

func printConfigTable(w io.Writer, v any) error {
	cfg, ok := v.(*config.Config)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "sync.gitAuto\t%t\n", cfg.Sync.GitAuto)
	fmt.Fprintf(tw, "scope\t%s\n", cfg.Scope)
	fmt.Fprintf(tw, "providers.mode\t%s\n", cfg.Providers.Mode)
	fmt.Fprintf(tw, "providers.enabled\t%v\n", cfg.Providers.Enabled)
	fmt.Fprintf(tw, "providers.disabled\t%v\n", cfg.Providers.Disabled)
	fmt.Fprintf(tw, "providers.effective\t%v\n", cfg.EffectiveProviders())
	fmt.Fprintf(tw, "theme\t%s\n", cfg.Theme)
	fmt.Fprintf(tw, "skills.enabled\t%t\n", cfg.Skills.EnabledOrDefault())
	fmt.Fprintf(tw, "skills.autoInstall\t%t\n", cfg.Skills.AutoInstall)
	fmt.Fprintf(tw, "skills.providers\t%v\n", cfg.Skills.Providers)
	return tw.Flush()
}

// providerCoverage renders a provider->inSync map as a sorted compact string.
func providerCoverage(m map[string]bool) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]byte, 0, 32)
	for i, k := range keys {
		if i > 0 {
			out = append(out, ',')
		}
		mark := "ok"
		if !m[k] {
			mark = "drift"
		}
		out = append(out, []byte(k+":"+mark)...)
	}
	return string(out)
}
