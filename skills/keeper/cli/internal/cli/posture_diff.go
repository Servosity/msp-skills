// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"keeper-pp-cli/internal/keeper"
	"keeper-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func newNovelPostureDiffCmd(flags *rootFlags) *cobra.Command {
	var from, to int64
	var mc string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "See what changed between two posture runs: per-company deltas and per-user transitions",
		Long: `Use this command to compare two stored posture snapshots.
Do NOT use this command to get current numbers; use 'posture report' instead.
Do NOT use it to consume changes incrementally from a cursor; use 'events list' instead.

Reads two runs from the local store (no network call): by default the newest run and the one
before it; --from and --to take run ids from 'posture runs'. A company is compared only when it
was read successfully in both runs; otherwise it is listed under not_compared with the reason.`,
		Example: strings.Trim(`
  keeper-cli posture diff --agent
  keeper-cli posture diff --from 3 --to 9 --mc 4421 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if from < 0 || to < 0 {
				return usageErr(fmt.Errorf("--from and --to are run ids from 'posture runs'"))
			}
			var err error
			if mc, err = validateMC(mc); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture diff")
			}
			db, err := openStoreForRead(cmd.Context(), "keeper-cli")
			if err != nil {
				return err
			}
			if db == nil {
				return notFoundErr(fmt.Errorf("no posture run stored yet: run 'keeper-cli posture report' first"))
			}
			defer db.Close()
			runs, err := db.PostureRuns(cmd.Context(), 100000)
			if err != nil {
				return err
			}
			if to == 0 && len(runs) > 0 {
				to = runs[0].ID
			}
			if from == 0 {
				for _, r := range runs {
					if r.ID < to {
						from = r.ID
						break
					}
				}
			}
			if from == 0 || to == 0 || from >= to {
				return notFoundErr(fmt.Errorf("need two posture runs to compare (from %d, to %d): run 'keeper-cli posture report' again, or pass --from and --to", from, to))
			}
			before, err := db.CompanyPostures(cmd.Context(), from)
			if err != nil {
				return err
			}
			after, err := db.CompanyPostures(cmd.Context(), to)
			if err != nil {
				return err
			}
			prev := map[string]store.CompanyPosture{}
			for _, p := range before {
				prev[p.CompanyID] = p
			}
			companies := []map[string]any{}
			notCompared := []map[string]any{}
			for _, cur := range after {
				if mc != "" && cur.CompanyID != mc {
					continue
				}
				old, ok := prev[cur.CompanyID]
				reason := ""
				switch {
				case !ok:
					reason = "not in the earlier run"
				case !old.OK:
					reason = "earlier run failed: " + old.Error
				case !cur.OK:
					reason = "later run failed: " + cur.Error
				}
				if reason != "" {
					notCompared = append(notCompared, map[string]any{"company_id": cur.CompanyID, "company_name": cur.CompanyName, "reason": reason})
					continue
				}
				var a, b postureRecord
				if err := json.Unmarshal(old.Metrics, &a); err != nil {
					return err
				}
				if err := json.Unmarshal(cur.Metrics, &b); err != nil {
					return err
				}
				pu, err := loadUserFacts(cmd.Context(), db, from, cur.CompanyID)
				if err != nil {
					return err
				}
				cu, err := loadUserFacts(cmd.Context(), db, to, cur.CompanyID)
				if err != nil {
					return err
				}
				changes := keeper.Diff(cur.CompanyID, a.Metrics, b.Metrics, pu, cu)
				companies = append(companies, map[string]any{"company_id": cur.CompanyID, "company_name": cur.CompanyName,
					"deltas": metricDeltas(a, b), "changes": changes})
			}
			return flags.printJSON(cmd, map[string]any{"from_run": from, "to_run": to, "companies": companies, "not_compared": notCompared})
		},
	}
	cmd.Flags().Int64Var(&from, "from", 0, "Earlier run id (default: the run before --to)")
	cmd.Flags().Int64Var(&to, "to", 0, "Later run id (default: the newest run)")
	cmd.Flags().StringVar(&mc, "mc", "", "Only this managed company id")
	return cmd
}

// metricDeltas lists the company metrics that differ, with before and after.
func metricDeltas(a, b postureRecord) map[string]any {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	var ma, mb map[string]any
	_ = json.Unmarshal(ja, &ma)
	_ = json.Unmarshal(jb, &mb)
	out := map[string]any{}
	for k, vb := range mb {
		va := ma[k]
		if fmt.Sprint(va) != fmt.Sprint(vb) {
			out[k] = map[string]any{"before": va, "after": vb}
		}
	}
	for k, va := range ma {
		if _, ok := mb[k]; !ok {
			out[k] = map[string]any{"before": va, "after": nil}
		}
	}
	return out
}
