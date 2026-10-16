// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
	"sonicwall-pp-cli/internal/store"
)

type postureChangesResult struct {
	Scope            string           `json:"scope,omitempty"`
	From             string           `json:"from,omitempty"`
	To               string           `json:"to,omitempty"`
	BaselineComplete *bool            `json:"baseline_complete,omitempty"`
	LatestComplete   *bool            `json:"latest_complete,omitempty"`
	Note             string           `json:"note,omitempty"`
	Changes          []posture.Change `json:"changes"`
}

func newNovelPostureChangesCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath, tenant, serial string
	var fleet bool
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List per-firewall posture check changes between the newest saved posture run and the latest same-scope run at least --since (default 7d) older, falling back to the oldest same-scope run when history is shorter; plus firewalls added or removed; reads local runs only",
		Long: "Use this command to see what changed in fleet posture since an earlier 'posture' run (newly vulnerable,\n" +
			"MFA turned off, licence lapsed). Do NOT use this command for the current posture state; use 'posture' instead.\n\n" +
			"Reads only posture runs saved on this machine (run 'posture' first, and again later). Compares the newest run\n" +
			"with the latest run over the same scope (whole fleet, one --tenant, one --serial, or one --tenant plus --serial)\n" +
			"taken at or before --since ago, or the oldest same-scope run when none is that old. With no scope flag it uses\n" +
			"the scope of the newest saved run; pass --fleet, --tenant or --serial (the same filters you gave 'posture') to\n" +
			"pick one. Runs marked incomplete had firewalls that were never checked; their rows show 'unknown'.",
		Example: strings.Trim(`
  sonicwall-cli posture changes --fleet --since 7d --agent
  sonicwall-cli posture changes --since 24h`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture changes")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive duration like 7d, 1w or 24h, got %q", since))
			}
			if fleet && (tenant != "" || serial != "") {
				return usageErr(fmt.Errorf("--fleet cannot be combined with --tenant or --serial"))
			}
			wantScope := ""
			if fleet || tenant != "" || serial != "" {
				wantScope = postureScope(tenant, serial)
			}
			ctx := cmd.Context()
			if dbPath == "" {
				dbPath = defaultDBPath("sonicwall-cli")
			}
			result := postureChangesResult{Changes: make([]posture.Change, 0)}
			emit := func() error {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), result, flags)
				}
				if result.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), result.Note)
				}
				if len(result.Changes) == 0 {
					if result.From != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "No posture changes (%s) between %s and %s.\n", result.Scope, result.From, result.To)
					}
					return nil
				}
				rows := make([]map[string]any, 0, len(result.Changes))
				for _, ch := range result.Changes {
					rows = append(rows, map[string]any{"serial": ch.Serial, "name": ch.Name, "check": ch.Check, "before": ch.Before, "after": ch.After})
				}
				return printOrderedTable(cmd.OutOrStdout(), []string{"serial", "name", "check", "before", "after"}, rows)
			}
			if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local posture history at %s\nrun: sonicwall-cli posture\n", dbPath)
				result.Note = "no posture runs saved yet; run 'sonicwall-cli posture' now and again later"
				return emit()
			}
			db, err := store.OpenReadOnlyContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			runs, err := loadPostureRuns(ctx, db.DB())
			if err != nil {
				return fmt.Errorf("reading posture runs: %w", err)
			}
			if len(runs) == 0 {
				result.Note = "no posture runs saved yet; run 'sonicwall-cli posture' now and again later"
				return emit()
			}
			if wantScope != "" {
				scoped := make([]postureRun, 0, len(runs))
				for _, r := range runs {
					if r.Scope == wantScope {
						scoped = append(scoped, r)
					}
				}
				if len(scoped) == 0 {
					result.Scope = wantScope
					result.Note = fmt.Sprintf("no posture run saved for scope %q; run 'sonicwall-cli posture' with the same filters now and again later", wantScope)
					return emit()
				}
				runs = scoped
			}
			newest := runs[0]
			result.Scope = newest.Scope
			same := make([]postureRun, 0)
			for _, r := range runs[1:] {
				if r.Scope == newest.Scope {
					same = append(same, r)
				}
			}
			if len(same) == 0 {
				result.Note = fmt.Sprintf("only one posture run saved for scope %q; run 'sonicwall-cli posture' again later to compare", newest.Scope)
				return emit()
			}
			baseline := same[len(same)-1] // oldest same-scope run, unless one is old enough
			cutoff := newest.TakenAt.Add(-window)
			for _, r := range same {
				if !r.TakenAt.After(cutoff) {
					baseline = r
					break
				}
			}
			before, err := loadPostureSnapshot(ctx, db.DB(), baseline)
			if err != nil {
				return err
			}
			after, err := loadPostureSnapshot(ctx, db.DB(), newest)
			if err != nil {
				return err
			}
			names := after.Names
			for s, n := range before.Names {
				if names[s] == "" {
					names[s] = n
				}
			}
			bc, lc := baseline.Complete, newest.Complete
			result.From, result.To = baseline.TakenAt.Format(time.RFC3339), newest.TakenAt.Format(time.RFC3339)
			result.BaselineComplete, result.LatestComplete = &bc, &lc
			result.Changes = posture.DiffSnapshots(before.Checks, after.Checks, names)
			return emit()
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Compare against the latest same-scope run at least this old (e.g. 7d, 1w, 24h)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path holding posture runs (default: the CLI's local store)")
	cmd.Flags().BoolVar(&fleet, "fleet", false, "Compare whole-fleet posture runs (posture with no --tenant or --serial)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Compare posture runs taken with this --tenant filter")
	cmd.Flags().StringVar(&serial, "serial", "", "Compare posture runs taken with this --serial filter")
	return cmd
}
