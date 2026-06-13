// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type atRiskWorkload struct {
	Company            string `json:"company"`
	Workload           string `json:"workload"`
	ManagedBy          string `json:"managed_by"`
	LatestRestorePoint string `json:"latest_restore_point,omitempty"`
	AgeHours           int    `json:"age_hours"`
	Reason             string `json:"reason"`
}

type atRiskView struct {
	Workloads []atRiskWorkload `json:"workloads"`
	RPO       string           `json:"rpo"`
	Count     int              `json:"count"`
}

func newNovelAtRiskCmd(flags *rootFlags) *cobra.Command {
	var flagRpo, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "at-risk",
		Short: "Protected workloads whose latest restore point is older than the RPO threshold (or missing entirely).",
		Long: strings.Trim(`
List protected workloads (computers managed by a backup server or by the
console) whose newest restore point is older than the RPO window (default 24h)
— or that have no restore point at all. These are the workloads that would lose
data on a failure right now, sorted worst-first.

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first.`, "\n"),
		Example: strings.Trim(`
  veeam-cli at-risk
  veeam-cli at-risk --rpo 24h
  veeam-cli at-risk --rpo 4h --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			rpo, err := veeamParseWindow(flagRpo, 24*time.Hour)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --rpo: %w", err))
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
			cutoff := now.Add(-rpo)

			sources := []struct {
				rtype     string
				managedBy string
			}{
				{"protected-workloads-computers-managed-by-backup-server", "backup-server"},
				{"protected-workloads-computers-managed-by-console", "console"},
			}

			out := make([]atRiskWorkload, 0)
			for _, src := range sources {
				rows, _ := veeamLoad(ctx, db, src.rtype)
				for _, w := range rows {
					last, has := vtime(w, "latestRestorePointDate")
					var reason string
					var ageHours int
					var lastStr string
					switch {
					case !has:
						reason = "no restore point"
						ageHours = -1
					case last.Before(cutoff):
						reason = "restore point older than RPO"
						ageHours = veeamHoursSince(now, last)
						lastStr = last.UTC().Format(time.RFC3339)
					default:
						continue // within RPO -> not at risk
					}
					out = append(out, atRiskWorkload{
						Company:            veeamCompanyLabel(names, vstr(w, "organizationUid")),
						Workload:           vstr(w, "name"),
						ManagedBy:          src.managedBy,
						LatestRestorePoint: lastStr,
						AgeHours:           ageHours,
						Reason:             reason,
					})
				}
			}

			// Most at risk first: missing (-1) ranks ahead of oldest age.
			sort.SliceStable(out, func(i, j int) bool {
				rank := func(h int) int {
					if h < 0 {
						return 1 << 62
					}
					return h
				}
				return rank(out[i].AgeHours) > rank(out[j].AgeHours)
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			view := atRiskView{Workloads: out, RPO: rpo.String(), Count: len(out)}
			table := make([]map[string]any, 0, len(out))
			for _, w := range out {
				age := fmt.Sprintf("%dh", w.AgeHours)
				if w.AgeHours < 0 {
					age = "none"
				}
				table = append(table, map[string]any{
					"company":    w.Company,
					"workload":   w.Workload,
					"managed_by": w.ManagedBy,
					"age":        age,
					"reason":     w.Reason,
				})
			}
			return veeamEmit(cmd, flags, view, table, "No at-risk workloads in the local mirror. Run `veeam-cli sync` first, then re-check.")
		},
	}
	cmd.Flags().StringVar(&flagRpo, "rpo", "", "RPO window as a duration (default 24h); workloads with no newer restore point are at risk")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum workloads to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}
