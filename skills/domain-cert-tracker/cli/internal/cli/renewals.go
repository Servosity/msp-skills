// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source local

package cli

import (
	"time"

	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelRenewalsCmd(flags *rootFlags) *cobra.Command {
	var client, window string
	cmd := &cobra.Command{
		Use:   "renewals",
		Short: "Find hosts still serving an old certificate after a newer one was logged, or past their renewal point with no successor in CT.",
		Long: `Use this command to find hosts whose renewal automation failed: a newer
certificate exists in CT but the host still serves the old one, or the host is
past its renewal point with no successor logged. Do NOT use this command for a
plain expiring-within-N-days list; use 'report' instead.

Statuses: deploy_stalled, issuance_stalled, served_not_logged, ct_unverified,
tls_unverified, ok (problems first). Reads the local store only.

deploy_stalled is measured over every newer same-CA certificate in CT, from
the earliest one this tool saw (a newer issuance never restarts the one-day
grace). successor is the newest; on deploy_stalled, overdue_successor is the
one whose grace ran out first and overdue_since is when it ran out. A newer
certificate this tool cannot date is ct_unverified; one seen after the host's
last probe is tls_unverified. A newer certificate from the same CA whose names
sweep has not yet read from the certificate itself (crt.sh lists only the
names that matched the domain) may still renew the host, so while one exists
and no successor is visible the host is ct_unverified, never ok or
issuance_stalled.`,
		Example:     "  domain-cert-tracker-cli renewals --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			w, err := parseDaysFlag("window", window)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "renewals")
			}
			db, err := openTrackerRead(cmd.Context())
			if err != nil {
				return err
			}
			findings := []tracker.RenewalFinding{}
			if db != nil {
				defer db.Close()
				if findings, err = tracker.Renewals(cmd.Context(), db, client, w, time.Now().UTC()); err != nil {
					return trackerErr(err)
				}
			}
			counts := map[string]int{}
			for _, f := range findings {
				counts[f.Status]++
			}
			// --agent compaction drops keys present on few rows; the stalled
			// rows are exactly the rare ones that carry these.
			return printJSONFilteredKeep(cmd.OutOrStdout(), map[string]any{"hosts": findings, "counts": counts}, flags, "successor", "overdue_successor", "overdue_since", "other_covering_certs", "served_in_ct")
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "Only this inventoried client")
	cmd.Flags().StringVar(&window, "window", "14", "Days before expiry by which a successor certificate should exist")
	return cmd
}
