// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type fleetHealthView struct {
	Meta    fleetEnvelopeMeta     `json:"meta"`
	Tenants []fleet.TenantPosture `json:"tenants"`
	Gappy   int                   `json:"tenants_with_gaps"`
	Note    string                `json:"note,omitempty"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var flagGaps bool

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "One cross-tenant protection posture table: enablement, retention, autodiscover, and last-backup age with gap flags",
		Long: strings.Trim(`
Use this for the cross-tenant protection posture table (one row per
subscription: Exchange/SharePoint on-off, retention days, autodiscover,
last-backup age, gap flags). Reads the local fleet store - run 'fleet-sync'
first. Do NOT use it for per-mailbox staleness (use 'stale-snapshots') or for
retention-floor compliance alone (use 'retention-audit').

Gap flags: exchange_backup_off, sharepoint_backup_off, autodiscover_off,
unprotected_mailboxes, unprotected_sites, stale_backup (newest snapshot older
than 48h). Fields the API response did not expose parse as unknown and never
fabricate a gap.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli fleet-health --agent
  skykick-cli fleet-health --flag-gaps
  skykick-cli fleet-health --agent --select tenants.company,tenants.gaps
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would build the fleet protection posture table from the local store")
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
			postures := fleet.BuildPostures(state, time.Now().UTC())
			gappy := 0
			for _, p := range postures {
				if len(p.Gaps) > 0 {
					gappy++
				}
			}
			view := fleetHealthView{Meta: metaForRun(run), Gappy: gappy}
			if flagGaps {
				for _, p := range postures {
					if len(p.Gaps) > 0 {
						view.Tenants = append(view.Tenants, p)
					}
				}
				if len(view.Tenants) == 0 {
					view.Tenants = []fleet.TenantPosture{}
					view.Note = fmt.Sprintf("no protection gaps across %d tenants in run %d", len(postures), run.ID)
				}
			} else {
				view.Tenants = postures
			}

			rows := make([]map[string]any, 0, len(view.Tenants))
			for _, p := range view.Tenants {
				age := ""
				if p.NewestSnapshot != nil {
					age = fmt.Sprintf("%.0fh", time.Since(*p.NewestSnapshot).Hours())
				}
				rows = append(rows, map[string]any{
					"company":    p.Company,
					"id":         p.SubscriptionID,
					"exchange":   triStateWord(p.ExchangeEnabled),
					"sharepoint": triStateWord(p.SharePointEnabled),
					"autodisc":   triStateWord(p.AutodiscoverOn),
					"mailboxes":  fmt.Sprintf("%d/%d", p.MailboxesEnabled, p.MailboxesTotal),
					"sites":      fmt.Sprintf("%d/%d", p.SitesEnabled, p.SitesTotal),
					"backup_age": age,
					"gaps":       strings.Join(p.Gaps, ","),
				})
			}
			summary := fmt.Sprintf("%d tenants, %d with gaps (run %d, synced %s)", len(postures), gappy, run.ID, run.FinishedAt)
			return fleetPrint(cmd, flags, view, rows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().BoolVar(&flagGaps, "flag-gaps", false, "Show only tenants that have at least one protection gap")
	return cmd
}
