package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
		return printJSON(w, v)
	case "yaml", "yml":
		return printYAML(w, v)
	case "table":
		return printTable(w, kind, v)
	default:
		return fmt.Errorf("unsupported output format %q (use: json|yaml|table)", format)
	}
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
	case "status":
		return printStatusTable(w, v)
	case "sync":
		return printRunResultTable(w, v)
	case "validate":
		return printFindingsTable(w, v)
	case "config":
		return printConfigTable(w, v)
	default:
		return printJSON(w, v)
	}
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

func printAgentListTable(w io.Writer, v any) error {
	agents, ok := v.([]contract.AgentStatus)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tIN_SYNC\tPROVIDERS")
	for _, a := range agents {
		fmt.Fprintf(tw, "%s\t%t\t%s\n", a.Name, a.InSync, providerCoverage(a.Providers))
	}
	return tw.Flush()
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
	r, ok := v.(contract.RunResult)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "run_id\t%s\n", r.RunID)
	fmt.Fprintf(tw, "status\t%s\n", r.Status)
	if len(r.Changed) > 0 {
		fmt.Fprintf(tw, "changed\t%d\n", len(r.Changed))
	}
	if len(r.Conflicts) > 0 {
		fmt.Fprintf(tw, "conflicts\t%d\n", len(r.Conflicts))
	}
	if err := tw.Flush(); err != nil {
		return err
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

func printConfigTable(w io.Writer, v any) error {
	cfg, ok := v.(*config.Config)
	if !ok {
		return printJSON(w, v)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "sync.gitAuto\t%t\n", cfg.Sync.GitAuto)
	fmt.Fprintf(tw, "scope\t%s\n", cfg.Scope)
	fmt.Fprintf(tw, "providers.enabled\t%v\n", cfg.Providers.Enabled)
	fmt.Fprintf(tw, "theme\t%s\n", cfg.Theme)
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
