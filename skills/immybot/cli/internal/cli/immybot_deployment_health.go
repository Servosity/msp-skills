// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Absorbed feature (manifest row 58): historical deployment compliance.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type deploymentHealth struct {
	AssignmentID   string  `json:"assignment_id"`
	TargetName     string  `json:"target_name,omitempty"`
	MaintenanceID  string  `json:"maintenance_identifier,omitempty"`
	TenantID       string  `json:"tenant_id,omitempty"`
	Actions        int     `json:"actions"`
	Succeeded      int     `json:"succeeded"`
	Failed         int     `json:"failed"`
	SuccessRate    float64 `json:"success_rate"`
	NeverSucceeded bool    `json:"never_succeeded"`
}

type deploymentHealthView struct {
	ScannedActions int                `json:"scanned_actions"`
	Deployments    []deploymentHealth `json:"deployments"`
	NeverSucceeded int                `json:"never_succeeded_count"`
	Note           string             `json:"note,omitempty"`
}

func newNovelDeploymentHealthCmd(flags *rootFlags) *cobra.Command {
	var (
		flagOnlyFailing bool
		flagLimit       int
		dbPath          string
	)
	cmd := &cobra.Command{
		Use:   "deployment-health",
		Short: "Historical success rate per deployment, including ones that never worked",
		Long: "Roll up every recorded maintenance action per target assignment to show which " +
			"deployments actually land and which have never once succeeded.",
		Example: strings.Trim(`
  immybot-pp-cli deployment-health
  immybot-pp-cli deployment-health --only-failing --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=25",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "deployment-health")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := deploymentHealthView{Deployments: make([]deploymentHealth, 0)}
			if immyMirrorMissing(cmd, dbPath, "maintenance-actions,target-assignments") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "maintenance-actions") {
				hintIfStale(cmd, db, "maintenance-actions", flags.maxAge)
			}

			arows, err := db.DB().QueryContext(ctx, `
				SELECT json_extract(data,'$.assignmentId'),
				       COALESCE(json_extract(data,'$.statusName'), json_extract(data,'$.status'))
				FROM resources
				WHERE resource_type IN ('maintenance-actions','maintenance-actions-maintenance-item','maintenance-actions-version')`)
			if err != nil {
				return fmt.Errorf("querying maintenance actions: %w", err)
			}
			type tally struct{ actions, ok, bad int }
			tallies := map[string]*tally{}
			for arows.Next() {
				var assignment, status sql.NullString
				if err := arows.Scan(&assignment, &status); err != nil {
					_ = arows.Close()
					return fmt.Errorf("scanning maintenance action: %w", err)
				}
				view.ScannedActions++
				id := nullStr(assignment)
				if id == "" {
					continue
				}
				tl := tallies[id]
				if tl == nil {
					tl = &tally{}
					tallies[id] = tl
				}
				tl.actions++
				st := nullStr(status)
				if immyIsFailureStatus(st) {
					tl.bad++
				} else if strings.EqualFold(strings.TrimSpace(st), "success") || strings.EqualFold(strings.TrimSpace(st), "succeeded") {
					tl.ok++
				}
			}
			if err := arows.Err(); err != nil {
				_ = arows.Close()
				return fmt.Errorf("iterating maintenance actions: %w", err)
			}
			if err := arows.Close(); err != nil {
				return fmt.Errorf("closing maintenance actions: %w", err)
			}

			// Name the deployments now that the action rows are closed.
			names := map[string][2]string{}
			tenants := map[string]string{}
			trows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.targetName'), json_extract(data,'$.maintenanceIdentifier'), json_extract(data,'$.tenantId')
				FROM resources WHERE resource_type IN ('target-assignments','target-assignments-global')`)
			if err == nil {
				for trows.Next() {
					var id, name, mid, tenant sql.NullString
					if err := trows.Scan(&id, &name, &mid, &tenant); err != nil {
						break
					}
					names[nullStr(id)] = [2]string{nullStr(name), nullStr(mid)}
					tenants[nullStr(id)] = nullStr(tenant)
				}
				_ = trows.Err()
				_ = trows.Close()
			}

			for id, tl := range tallies {
				d := deploymentHealth{
					AssignmentID:  id,
					TargetName:    names[id][0],
					MaintenanceID: names[id][1],
					TenantID:      tenants[id],
					Actions:       tl.actions,
					Succeeded:     tl.ok,
					Failed:        tl.bad,
				}
				if tl.actions > 0 {
					d.SuccessRate = float64(tl.ok) / float64(tl.actions)
				}
				d.NeverSucceeded = tl.ok == 0 && tl.actions > 0
				if d.NeverSucceeded {
					view.NeverSucceeded++
				}
				if flagOnlyFailing && tl.bad == 0 && !d.NeverSucceeded {
					continue
				}
				view.Deployments = append(view.Deployments, d)
			}
			sort.Slice(view.Deployments, func(i, j int) bool {
				if view.Deployments[i].SuccessRate != view.Deployments[j].SuccessRate {
					return view.Deployments[i].SuccessRate < view.Deployments[j].SuccessRate
				}
				return view.Deployments[i].AssignmentID < view.Deployments[j].AssignmentID
			})
			if flagLimit > 0 && len(view.Deployments) > flagLimit {
				view.Deployments = view.Deployments[:flagLimit]
			}
			if view.ScannedActions == 0 {
				view.Note = "no maintenance actions in the local mirror; run 'immybot-pp-cli sync --resources maintenance-actions'"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Deployments) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), firstNonEmpty(view.Note, "No deployments with recorded actions."))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-36s %-7s %-7s %-7s %s\n", "DEPLOYMENT", "RUNS", "OK", "FAIL", "RATE")
			for _, d := range view.Deployments {
				fmt.Fprintf(cmd.OutOrStdout(), "%-36s %-7d %-7d %-7d %.0f%%\n",
					immyTruncate(firstNonEmpty(d.TargetName, d.AssignmentID), 36), d.Actions, d.Succeeded, d.Failed, d.SuccessRate*100)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d deployment(s) have never succeeded.\n", view.NeverSucceeded)
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagOnlyFailing, "only-failing", false, "Only show deployments with failures or that never succeeded")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum deployments to return (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelDeploymentHealthCmd(flags))
	})
}
