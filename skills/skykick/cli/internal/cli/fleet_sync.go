// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/syncer"
)

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit, workers, topAlerts int
	var skipCSV string

	cmd := &cobra.Command{
		Use:   "fleet-sync",
		Short: "Pull every subscription plus per-tenant settings, retention, autodiscover, snapshot stats, mailboxes, sites, and alerts into the local fleet store",
		Long: strings.Trim(`
Run this before any fleet-posture or backup-integrity command; it builds the
run-versioned store they read (kept for the last 5 runs so 'drift' can diff).
The SkyKick API is strictly per-subscription; this command fans out across
every subscription with bounded concurrency and records every failed fetch in
the fetch_failures list instead of silently dropping data.

Framework 'sync' covers only the flat subscription/datacenter lists -
'fleet-sync' adds the per-tenant facet fan-out.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli fleet-sync
  skykick-cli fleet-sync --limit 10 --workers 8
  skykick-cli fleet-sync --skip alerts --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would sync all backup subscriptions and their facets (settings, retention, autodiscover, snapshot stats, mailboxes, sites, alerts) into the local fleet store")
				return nil
			}
			limit = syncer.DogfoodCurtailed(limit)

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			db, err := openFleetStore(cmdContext(cmd), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			skip := map[string]bool{}
			for _, facet := range strings.Split(skipCSV, ",") {
				if f := strings.TrimSpace(strings.ToLower(facet)); f != "" {
					skip[f] = true
				}
			}

			res, err := syncer.RunFleetSync(cmdContext(cmd), c, db, syncer.Options{
				Limit:     limit,
				Workers:   workers,
				TopAlerts: topAlerts,
				Skip:      skip,
			})
			if err != nil {
				return classifyAPIError(err, flags)
			}

			if len(res.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d facet fetches failed across %d subscriptions; counts cover successful fetches only (see fetch_failures)\n",
					len(res.FetchFailures), res.Subscriptions)
			}

			summary := fmt.Sprintf("Synced %d/%d subscriptions (run %d): %d snapshot rows, %d mailboxes, %d sites, %d alerts in %.1fs",
				res.Subscriptions, res.SubscriptionsSeen, res.RunID, res.SnapshotRows, res.Mailboxes, res.Sites, res.Alerts, res.DurationSeconds)
			return fleetPrint(cmd, flags, res, nil, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum subscriptions to sync (0 = all)")
	cmd.Flags().IntVar(&workers, "workers", 4, "Concurrent subscription workers")
	cmd.Flags().IntVar(&topAlerts, "top-alerts", 500, "Alerts to pull per subscription (max 500; the API has no skip paging)")
	cmd.Flags().StringVar(&skipCSV, "skip", "", "Comma-separated facets to skip: settings,retention,autodiscover,snapshotstats,mailboxes,sites,alerts")
	return cmd
}
