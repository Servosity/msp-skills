// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type autodiscoverAuditView struct {
	Meta    fleetEnvelopeMeta       `json:"meta"`
	Tenants []fleet.AutodiscoverRow `json:"tenants"`
	Off     int                     `json:"off"`
	Partial int                     `json:"partial"`
	On      int                     `json:"on"`
	Unknown int                     `json:"unknown"`
	Note    string                  `json:"note,omitempty"`
}

func newNovelAutodiscoverAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var onlyOff bool

	cmd := &cobra.Command{
		Use:   "autodiscover-audit",
		Short: "Fleet table of autodiscover on/off state per tenant",
		Long: strings.Trim(`
Use this to find tenants where auto-discover is off or partial fleet-wide -
new mailboxes and sites in those tenants will silently never enroll in backup.
Reads the local fleet store - run 'fleet-sync' first. Distinct from
'coverage-gaps', which diffs already-discovered items against enablement.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli autodiscover-audit --agent
  skykick-cli autodiscover-audit --only-off --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate per-tenant autodiscover state from the fleet store")
				return nil
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
			rows := fleet.AutodiscoverAudit(state.Autodiscover, companies, onlyOff)
			view := autodiscoverAuditView{Meta: metaForRun(run), Tenants: rows}
			if view.Tenants == nil {
				view.Tenants = []fleet.AutodiscoverRow{}
			}
			all := fleet.AutodiscoverAudit(state.Autodiscover, companies, false)
			for _, r := range all {
				switch r.Status {
				case "off":
					view.Off++
				case "partial":
					view.Partial++
				case "on":
					view.On++
				default:
					view.Unknown++
				}
			}
			if onlyOff && len(rows) == 0 {
				view.Note = fmt.Sprintf("no tenants with autodiscover off or partial (%d on, %d unknown)", view.On, view.Unknown)
			}

			humanRows := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				humanRows = append(humanRows, map[string]any{
					"company": r.Company, "exchange": triStateWord(r.ExchangeOn), "sharepoint": triStateWord(r.SharePointOn), "status": r.Status,
				})
			}
			summary := fmt.Sprintf("%d off, %d partial, %d on, %d unknown (run %d)", view.Off, view.Partial, view.On, view.Unknown, run.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().BoolVar(&onlyOff, "only-off", false, "Show only tenants whose autodiscover is off or partial")
	return cmd
}
