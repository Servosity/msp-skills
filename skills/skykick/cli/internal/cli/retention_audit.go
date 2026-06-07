// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type retentionAuditView struct {
	Meta       fleetEnvelopeMeta    `json:"meta"`
	FloorDays  int                  `json:"floor_days"`
	Tenants    []fleet.RetentionRow `json:"tenants"`
	Pass       int                  `json:"pass"`
	UnderFloor int                  `json:"under_floor"`
	Unknown    int                  `json:"unknown"`
}

func newNovelRetentionAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var floorDays int

	cmd := &cobra.Command{
		Use:   "retention-audit",
		Short: "Grade each tenant's backup retention period against a compliance floor",
		Long: strings.Trim(`
Use this to grade fleet retention against a compliance floor: every tenant
whose known Exchange or SharePoint retention is below --floor-days is flagged
under_floor; tenants with no extractable retention are unknown (never silently
passed). Reads the local fleet store - run 'fleet-sync' first. For the full
posture table, use 'fleet-health'.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli retention-audit --floor-days 365 --agent
  skykick-cli retention-audit --floor-days 180 --agent --select tenants.company,tenants.status
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would grade stored retention periods against the compliance floor")
				return nil
			}
			if floorDays <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--floor-days must be a positive number of days"))
			}
			db, err := openFleetStore(cmdContext(cmd), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			state, run, err := latestFleetState(cmdContext(cmd), db)
			if err != nil {
				return err
			}
			companies := fleet.CompanyIndex(state.Subscriptions, state.Settings)
			rows := fleet.RetentionAudit(state.Retention, companies, floorDays)
			view := retentionAuditView{Meta: metaForRun(run), FloorDays: floorDays, Tenants: rows}
			if view.Tenants == nil {
				view.Tenants = []fleet.RetentionRow{}
			}
			for _, r := range rows {
				switch r.Status {
				case "pass":
					view.Pass++
				case "under_floor":
					view.UnderFloor++
				default:
					view.Unknown++
				}
			}

			humanRows := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				exch, sp := "?", "?"
				if r.ExchangeDays != nil {
					exch = fmt.Sprintf("%d", *r.ExchangeDays)
				}
				if r.SharePointDays != nil {
					sp = fmt.Sprintf("%d", *r.SharePointDays)
				}
				humanRows = append(humanRows, map[string]any{
					"company": r.Company, "exchange_days": exch, "sharepoint_days": sp, "status": r.Status,
				})
			}
			summary := fmt.Sprintf("floor %dd: %d pass, %d under floor, %d unknown (run %d)", floorDays, view.Pass, view.UnderFloor, view.Unknown, run.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().IntVar(&floorDays, "floor-days", 365, "Minimum acceptable retention in days")
	return cmd
}
