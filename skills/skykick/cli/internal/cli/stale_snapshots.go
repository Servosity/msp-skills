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

type staleSnapshotsView struct {
	Meta             fleetEnvelopeMeta `json:"meta"`
	ThresholdHours   int               `json:"threshold_hours"`
	Stale            []fleet.StaleRow  `json:"stale"`
	ScannedMailboxes int               `json:"scanned_mailboxes"`
	Note             string            `json:"note,omitempty"`
}

func newNovelStaleSnapshotsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var hours int

	cmd := &cobra.Command{
		Use:   "stale-snapshots",
		Short: "Every mailbox not snapshotted within N hours, fleet-wide",
		Long: strings.Trim(`
Use this to find mailboxes whose last snapshot is older than --hours across
the whole fleet - the silently-failing backups. Mailboxes with no parseable
snapshot timestamp at all are listed first as never_snapshotted. Reads the
local fleet store - run 'fleet-sync' first. Do NOT use it for whole-tenant
on/off posture; use 'fleet-health'.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli stale-snapshots --hours 48 --agent
  skykick-cli stale-snapshots --hours 24 --agent --select stale.mailbox,stale.age_hours
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan the fleet store for mailboxes with stale snapshots")
				return nil
			}
			if hours <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--hours must be a positive number of hours"))
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
			rows := fleet.FindStale(state.Stats, companies, time.Duration(hours)*time.Hour, time.Now().UTC())
			view := staleSnapshotsView{
				Meta:             metaForRun(run),
				ThresholdHours:   hours,
				Stale:            rows,
				ScannedMailboxes: len(state.Stats),
			}
			if view.Stale == nil {
				view.Stale = []fleet.StaleRow{}
			}
			if len(rows) == 0 {
				view.Note = fmt.Sprintf("scanned %d mailbox snapshot rows; none older than %dh", len(state.Stats), hours)
			}

			humanRows := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				age := "never"
				if r.AgeHours != nil {
					age = fmt.Sprintf("%.0fh", *r.AgeHours)
				}
				last := ""
				if r.LastSnapshot != nil {
					last = r.LastSnapshot.Format(time.RFC3339)
				}
				humanRows = append(humanRows, map[string]any{
					"company": r.Company, "mailbox": r.Mailbox, "last_snapshot": last, "age": age,
				})
			}
			summary := fmt.Sprintf("%d stale of %d scanned mailboxes (threshold %dh, run %d)", len(rows), len(state.Stats), hours, run.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().IntVar(&hours, "hours", 48, "Staleness threshold in hours")
	return cmd
}
