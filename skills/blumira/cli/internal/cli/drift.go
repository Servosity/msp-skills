// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: finding drift since last sync.
// pp:data-source local

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "New, status-changed, and newly-resolved findings between the two most recent syncs",
		Long: "Compare the two most recent finding snapshots and report what changed: new\n" +
			"findings, status changes, and newly-resolved findings. Each run records the\n" +
			"current state as a snapshot, so drift becomes available after a second sync.\n" +
			"Reads (and appends to) the local store's snapshot history.",
		Example: "  blumira-cli drift\n" +
			"  blumira-cli drift --since 24h --json --select account,name,change",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			// --since is accepted for symmetry/agent ergonomics; drift always
			// compares the two most recent snapshot batches.
			if _, err := parseWindow(flagSince); err != nil {
				return usageErr(err)
			}
			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}
			findings, err := loadFindings(s)
			if err != nil {
				return apiErr(err)
			}
			if _, err := captureFindingHistory(s.DB(), s, findings, time.Now().UTC()); err != nil {
				return apiErr(err)
			}
			snaps, err := loadHistorySnapshots(s.DB())
			if err != nil {
				return apiErr(err)
			}
			if len(snaps) < 2 {
				if !flags.quiet {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"drift needs at least two distinct syncs; %d snapshot(s) recorded so far — run 'sync --full' again later\n", len(snaps))
				}
				return emitAnalyticsRows(cmd, flags, []driftRow{}, 0, "")
			}
			prev := snaps[len(snaps)-2].Findings
			cur := snaps[len(snaps)-1].Findings
			rows := computeDrift(prev, cur)
			return emitAnalyticsRows(cmd, flags, rows, len(rows), "no drift: nothing changed between the two most recent syncs")
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Hint window (e.g. 24h, 1d); drift compares the two most recent syncs regardless")
	return cmd
}
