// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleJob struct {
	Company   string  `json:"company"`
	Job       string  `json:"job"`
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	LastRun   string  `json:"last_run,omitempty"`
	DaysStale float64 `json:"days_stale"`
	Enabled   bool    `json:"enabled"`
}

type staleView struct {
	Jobs      []staleJob `json:"jobs"`
	Threshold string     `json:"threshold"`
	Count     int        `json:"count"`
}

func newNovelStaleBackupsCmd(flags *rootFlags) *cobra.Command {
	var flagDays, dbPath string
	var includeDisabled bool
	var limit int

	cmd := &cobra.Command{
		Use:   "stale-backups",
		Short: "Every backup job and agent job whose last successful run is older than N days, across all tenants, sorted by staleness.",
		Long: strings.Trim(`
List backup-server jobs and agent jobs whose last run is older than the
threshold (default 3 days) — or that have never run — across every tenant,
sorted with the stalest first. A job that last ran inside the window but did
not succeed is also reported, since its last good restore point is stale.

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first.
Disabled jobs are hidden unless --include-disabled is set.`, "\n"),
		Example: strings.Trim(`
  veeam-cli stale-backups --days 3
  veeam-cli stale-backups --days 7 --include-disabled
  veeam-cli stale-backups --days 1 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window, err := veeamParseDays(flagDays, 3)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --days: %w", err))
			}
			ctx := cmd.Context()
			db, ok, err := veeamOpenStoreRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			if ok {
				defer db.Close()
			}

			names := veeamCompanyNames(ctx, db)
			now := time.Now()
			cutoff := now.Add(-window)

			sources := []struct {
				types []string
				kind  string
			}{
				{[]string{"infrastructure-backup-servers-jobs"}, "backup-server-job"},
				{[]string{
					"infrastructure-backup-agents-jobs",
					"infrastructure-backup-agents-windows-jobs",
					"infrastructure-backup-agents-linux-jobs",
					"infrastructure-backup-agents-mac-jobs",
				}, "agent-job"},
			}

			out := make([]staleJob, 0)
			for _, src := range sources {
				rows, _ := veeamLoad(ctx, db, src.types...)
				for _, j := range rows {
					enabled := true
					if v, ok := vbool(j, "isEnabled"); ok {
						enabled = v
					}
					if !enabled && !includeDisabled {
						continue
					}
					health := veeamJobHealth(vstr(j, "status"))
					last, hasLast := vtime(j, "lastRun")
					stale := false
					daysStale := 0.0
					switch {
					case !hasLast:
						// Never ran: maximally stale.
						stale = true
						daysStale = -1 // sentinel -> sorted first
					case last.Before(cutoff):
						stale = true
						daysStale = now.Sub(last).Hours() / 24
					case health == "failed" || health == "warning":
						// Ran recently but not cleanly: last good point is stale.
						stale = true
						daysStale = now.Sub(last).Hours() / 24
					}
					if !stale {
						continue
					}
					org := vstr(j, "organizationUid")
					if org == "" {
						org = vstr(j, "mappedOrganizationUid")
					}
					lastStr := ""
					if hasLast {
						lastStr = last.UTC().Format(time.RFC3339)
					}
					out = append(out, staleJob{
						Company:   veeamCompanyLabel(names, org),
						Job:       vstr(j, "name"),
						Kind:      src.kind,
						Status:    vstr(j, "status"),
						LastRun:   lastStr,
						DaysStale: roundTo(daysStale, 1),
						Enabled:   enabled,
					})
				}
			}

			// Stalest first; never-ran (sentinel -1) sorts ahead of everything.
			sort.SliceStable(out, func(i, j int) bool {
				ai, aj := out[i].DaysStale, out[j].DaysStale
				rank := func(v float64) float64 {
					if v < 0 {
						return 1e18 // never-ran ranks most stale
					}
					return v
				}
				return rank(ai) > rank(aj)
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			view := staleView{Jobs: out, Threshold: window.String(), Count: len(out)}
			table := make([]map[string]any, 0, len(out))
			for _, j := range out {
				days := fmt.Sprintf("%.1f", j.DaysStale)
				if j.DaysStale < 0 {
					days = "never run"
				}
				table = append(table, map[string]any{
					"company":    j.Company,
					"job":        j.Job,
					"kind":       j.Kind,
					"status":     j.Status,
					"days_stale": days,
					"last_run":   j.LastRun,
				})
			}
			return veeamEmit(cmd, flags, view, table, "No stale jobs in the local mirror. Run `veeam-cli sync` first, then re-check.")
		},
	}
	cmd.Flags().StringVar(&flagDays, "days", "", "Staleness threshold in days (default 3); also accepts a duration like 36h")
	cmd.Flags().BoolVar(&includeDisabled, "include-disabled", false, "Include disabled jobs in the report")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum jobs to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}

// roundTo rounds f to n decimal places.
func roundTo(f float64, n int) float64 {
	if f < 0 {
		return f
	}
	p := 1.0
	for i := 0; i < n; i++ {
		p *= 10
	}
	return float64(int64(f*p+0.5)) / p
}
